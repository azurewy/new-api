package controller

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupWZConEntitlementTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	common.OptionMap = map[string]string{}

	originalGroupRatio := ratio_setting.GroupRatio2JSONString()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalSpecialUsableGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.MarshalJSONString()
	previousDB := model.DB
	previousLogDB := model.LOG_DB
	t.Cleanup(func() {
		_ = ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatio)
		_ = setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups)
		_ = types.LoadFromJsonString(ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup, originalSpecialUsableGroups)
		common.OptionMap = map[string]string{}
		model.DB = previousDB
		model.LOG_DB = previousLogDB
	})

	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	model.DB = db
	model.LOG_DB = db
	if err := db.AutoMigrate(&model.Option{}, &model.Channel{}, &model.Ability{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	return db
}

func TestSyncWZConGroupEntitlementCreatesGroupAndExactAbilities(t *testing.T) {
	db := setupWZConEntitlementTestDB(t)
	weight := uint(3)
	priority := int64(5)
	tag := "primary"
	if err := db.Create(&[]model.Channel{
		{
			Id:       1,
			Name:     "primary",
			Key:      "sk-test",
			Status:   common.ChannelStatusEnabled,
			Models:   "gpt-5.4-mini,claude-sonnet-4-6",
			Group:    "default",
			Weight:   &weight,
			Priority: &priority,
			Tag:      &tag,
		},
		{
			Id:     2,
			Name:   "pro",
			Key:    "sk-test-2",
			Status: common.ChannelStatusEnabled,
			Models: "gpt-5.4-pro",
			Group:  "default",
		},
	}).Error; err != nil {
		t.Fatalf("seed channels: %v", err)
	}
	if err := db.Create(&model.Ability{Group: "wzcon_free", Model: "old-model", ChannelId: 2, Enabled: true}).Error; err != nil {
		t.Fatalf("seed old ability: %v", err)
	}

	err := syncWZConGroupEntitlement(WZConGroupEntitlementSyncRequest{
		Group:         "wzcon_free",
		Name:          "Free",
		Models:        []string{"gpt-5.4-mini"},
		Ratio:         1,
		UsableGroups:  []WZConUsableGroup{{Group: "wzcon_free", Name: "Free"}},
		ManagedGroups: []string{"wzcon_free", "wzcon_pro"},
	})
	if err != nil {
		t.Fatalf("sync entitlement: %v", err)
	}
	if !ratio_setting.ContainsGroupRatio("wzcon_free") {
		t.Fatal("expected group ratio to include wzcon_free")
	}
	usableGroups := service.GetUserUsableGroups("wzcon_free")
	if usableGroups["wzcon_free"] != "Free" {
		t.Fatalf("expected own group to be usable, got %#v", usableGroups)
	}
	if _, ok := usableGroups["wzcon_pro"]; ok {
		t.Fatalf("expected lower plan group to remove higher plan group, got %#v", usableGroups)
	}
	if _, ok := usableGroups["default"]; ok {
		t.Fatalf("expected wzcon group hierarchy to remove global default group, got %#v", usableGroups)
	}

	var abilities []model.Ability
	if err := db.Where(&model.Ability{Group: "wzcon_free"}).Find(&abilities).Error; err != nil {
		t.Fatalf("query abilities: %v", err)
	}
	if len(abilities) != 1 {
		t.Fatalf("expected exactly one synced ability, got %#v", abilities)
	}
	if abilities[0].Model != "gpt-5.4-mini" || abilities[0].ChannelId != 1 || abilities[0].Weight != 3 {
		t.Fatalf("unexpected synced ability: %#v", abilities[0])
	}
}

func TestSyncWZConGroupEntitlementAllowsLowerGroupsForHigherPlan(t *testing.T) {
	_ = setupWZConEntitlementTestDB(t)

	err := syncWZConGroupEntitlement(WZConGroupEntitlementSyncRequest{
		Group: "wzcon_pro",
		Name:  "Pro",
		UsableGroups: []WZConUsableGroup{
			{Group: "wzcon_free", Name: "Free"},
			{Group: "wzcon_pro", Name: "Pro"},
		},
		ManagedGroups: []string{"wzcon_free", "wzcon_pro"},
	})
	if err != nil {
		t.Fatalf("sync entitlement: %v", err)
	}
	usableGroups := service.GetUserUsableGroups("wzcon_pro")
	if usableGroups["wzcon_free"] != "Free" || usableGroups["wzcon_pro"] != "Pro" {
		t.Fatalf("expected higher plan to use lower and self groups, got %#v", usableGroups)
	}
}

func TestSyncWZConGroupEntitlementRejectsMissingChannelModels(t *testing.T) {
	db := setupWZConEntitlementTestDB(t)
	if err := db.Create(&model.Channel{
		Id:     1,
		Name:   "primary",
		Key:    "sk-test",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4-mini",
		Group:  "default",
	}).Error; err != nil {
		t.Fatalf("seed channel: %v", err)
	}

	err := syncWZConGroupEntitlement(WZConGroupEntitlementSyncRequest{
		Group:  "wzcon_pro",
		Name:   "Pro",
		Models: []string{"not-enabled-model"},
	})
	if err == nil || !strings.Contains(err.Error(), "没有启用渠道") {
		t.Fatalf("expected missing channel model error, got %v", err)
	}
}
