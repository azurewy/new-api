package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"
)

type WZConGroupEntitlementSyncRequest struct {
	Group         string             `json:"group"`
	Name          string             `json:"name"`
	Models        []string           `json:"models"`
	Ratio         float64            `json:"ratio"`
	UsableGroups  []WZConUsableGroup `json:"usable_groups"`
	ManagedGroups []string           `json:"managed_groups"`
}

type WZConUsableGroup struct {
	Group string `json:"group"`
	Name  string `json:"name"`
}

func SyncWZConGroupEntitlement(c *gin.Context) {
	var req WZConGroupEntitlementSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}

	if err := syncWZConGroupEntitlement(req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	common.ApiSuccess(c, nil)
}

func syncWZConGroupEntitlement(req WZConGroupEntitlementSyncRequest) error {
	group := strings.TrimSpace(req.Group)
	if group == "" {
		return fmt.Errorf("分组不能为空")
	}
	if strings.Contains(group, ",") {
		return fmt.Errorf("分组名称不能包含逗号")
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = group
	}
	ratio := req.Ratio
	if ratio <= 0 {
		ratio = 1
	}
	models := compactWZConModelNames(req.Models)

	if err := ensureWZConGroupOptions(group, name, ratio, req.UsableGroups, req.ManagedGroups); err != nil {
		return err
	}
	if err := syncWZConGroupAbilities(group, models); err != nil {
		return err
	}
	model.InitChannelCache()
	return nil
}

func ensureWZConGroupOptions(group, name string, ratio float64, usableGroups []WZConUsableGroup, managedGroups []string) error {
	groupRatios := ratio_setting.GetGroupRatioCopy()
	groupRatios[group] = ratio
	groupRatioJSON, err := json.Marshal(groupRatios)
	if err != nil {
		return fmt.Errorf("分组倍率编码失败: %w", err)
	}
	if err := model.UpdateOption("GroupRatio", string(groupRatioJSON)); err != nil {
		return fmt.Errorf("更新分组倍率失败: %w", err)
	}

	specialUsableGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.ReadAll()
	currentSpecial := buildWZConSpecialUsableGroup(group, name, usableGroups, managedGroups)
	specialUsableGroups[group] = currentSpecial
	specialUsableGroupsJSON, err := json.Marshal(specialUsableGroups)
	if err != nil {
		return fmt.Errorf("特殊可用分组编码失败: %w", err)
	}
	if err := model.UpdateOption("group_ratio_setting.group_special_usable_group", string(specialUsableGroupsJSON)); err != nil {
		return fmt.Errorf("更新特殊可用分组失败: %w", err)
	}
	return nil
}

func buildWZConSpecialUsableGroup(group, name string, usableGroups []WZConUsableGroup, managedGroups []string) map[string]string {
	allowed := make(map[string]string)
	for _, item := range usableGroups {
		itemGroup := strings.TrimSpace(item.Group)
		if itemGroup == "" {
			continue
		}
		allowed[itemGroup] = firstNonEmptyWZCon(strings.TrimSpace(item.Name), itemGroup)
	}
	if len(allowed) == 0 {
		allowed[group] = name
	}

	special := make(map[string]string)
	for globalGroup := range setting.GetUserUsableGroupsCopy() {
		if _, ok := allowed[globalGroup]; !ok {
			special["-:"+globalGroup] = globalGroup
		}
	}
	for _, item := range compactWZConModelNames(managedGroups) {
		if _, ok := allowed[item]; !ok {
			special["-:"+item] = item
		}
	}
	for itemGroup, itemName := range allowed {
		special[itemGroup] = itemName
	}
	return special
}

func syncWZConGroupAbilities(group string, selectedModels []string) error {
	var channels []model.Channel
	if err := model.DB.Where("status = ?", common.ChannelStatusEnabled).Find(&channels).Error; err != nil {
		return fmt.Errorf("查询启用渠道失败: %w", err)
	}

	selected := make(map[string]bool, len(selectedModels))
	for _, item := range selectedModels {
		selected[item] = true
	}
	covered := make(map[string]bool, len(selectedModels))
	seen := make(map[string]bool)
	abilities := make([]model.Ability, 0)
	for _, channel := range channels {
		channelModels := splitWZConCSV(channel.Models)
		for _, channelModel := range channelModels {
			if !selected[channelModel] {
				continue
			}
			key := fmt.Sprintf("%s|%s|%d", group, channelModel, channel.Id)
			if seen[key] {
				continue
			}
			seen[key] = true
			covered[channelModel] = true
			abilities = append(abilities, model.Ability{
				Group:     group,
				Model:     channelModel,
				ChannelId: channel.Id,
				Enabled:   true,
				Priority:  channel.Priority,
				Weight:    uint(channel.GetWeight()),
				Tag:       channel.Tag,
			})
		}
	}

	if missing := missingWZConModels(selectedModels, covered); len(missing) > 0 {
		return fmt.Errorf("以下模型没有启用渠道，无法同步到分组 %s: %s", group, strings.Join(missing, ", "))
	}

	tx := model.DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	if err := tx.Where(&model.Ability{Group: group}).Delete(&model.Ability{}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("清理分组能力失败: %w", err)
	}
	if len(abilities) > 0 {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&abilities).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("写入分组能力失败: %w", err)
		}
	}
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("提交分组能力失败: %w", err)
	}
	return nil
}

func compactWZConModelNames(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		result = append(result, trimmed)
	}
	return result
}

func splitWZConCSV(value string) []string {
	return compactWZConModelNames(strings.Split(value, ","))
}

func missingWZConModels(selected []string, covered map[string]bool) []string {
	missing := make([]string, 0)
	for _, item := range selected {
		if !covered[item] {
			missing = append(missing, item)
		}
	}
	sort.Strings(missing)
	return missing
}

func firstNonEmptyWZCon(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
