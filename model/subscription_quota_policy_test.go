package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func insertQuotaPolicySubscriptionPlan(t *testing.T, id int, quotaPolicy string) *SubscriptionPlan {
	t.Helper()
	plan := &SubscriptionPlan{
		Id:              id,
		Title:           fmt.Sprintf("Policy Plan %d", id),
		PriceAmount:     9.99,
		Currency:        "USD",
		DurationUnit:    SubscriptionDurationMonth,
		DurationValue:   1,
		Enabled:         true,
		UpgradeGroup:    "wzcon_policy",
		TotalAmount:     1000,
		QuotaPolicyJSON: quotaPolicy,
	}
	require.NoError(t, DB.Create(plan).Error)
	return plan
}

func insertActiveUserSubscriptionForPolicyTest(t *testing.T, userId int, plan *SubscriptionPlan, amountUsed int64) *UserSubscription {
	t.Helper()
	now := time.Now().Unix()
	sub := &UserSubscription{
		UserId:       userId,
		PlanId:       plan.Id,
		AmountTotal:  plan.TotalAmount,
		AmountUsed:   amountUsed,
		StartTime:    now - 3600,
		EndTime:      now + 30*24*3600,
		Status:       "active",
		UpgradeGroup: plan.UpgradeGroup,
	}
	require.NoError(t, DB.Create(sub).Error)
	return sub
}

func insertSubscriptionConsumeLogForPolicyTest(t *testing.T, userId int, subId int, consumed int64, createdAt int64) {
	insertSubscriptionConsumeLogWithRequestForPolicyTest(t, userId, subId, consumed, createdAt, "")
}

func insertSubscriptionConsumeLogWithRequestForPolicyTest(t *testing.T, userId int, subId int, consumed int64, createdAt int64, requestId string) {
	t.Helper()
	other := common.MapToJsonStr(map[string]interface{}{
		"billing_source":        "subscription",
		"subscription_id":       subId,
		"subscription_consumed": consumed,
	})
	require.NoError(t, LOG_DB.Create(&Log{
		UserId:    userId,
		Type:      LogTypeConsume,
		CreatedAt: createdAt,
		Quota:     int(consumed),
		RequestId: requestId,
		Other:     other,
	}).Error)
}

func TestPreConsumeUserSubscriptionRejectsQuotaPolicyAnchoredFiveHourLimit(t *testing.T) {
	truncateTables(t)

	policy := `{"mode":"all_limits_required","unit":"quota","limits":[{"key":"5h","name":"5 小时额度","amount":100,"window_seconds":18000,"reset":"rolling"},{"key":"monthly","name":"月总额度","amount":1000,"window_seconds":2592000,"reset":"subscription_cycle"}]}`
	plan := insertQuotaPolicySubscriptionPlan(t, 9101, policy)
	sub := insertActiveUserSubscriptionForPolicyTest(t, 7001, plan, 0)
	firstUse := time.Now().Add(-6 * time.Hour).Unix()
	insertSubscriptionConsumeLogForPolicyTest(t, 7001, sub.Id, 90, firstUse)
	insertSubscriptionConsumeLogForPolicyTest(t, 7001, sub.Id, 90, time.Now().Add(-30*time.Minute).Unix())

	_, err := PreConsumeUserSubscription("quota-policy-rolling", 7001, "gpt-5.5", 0, 20)
	require.Error(t, err)
	require.Contains(t, err.Error(), "subscription quota insufficient")
}

func TestPreConsumeUserSubscriptionStartsNewFiveHourBucketAfterFirstUseWindow(t *testing.T) {
	truncateTables(t)

	policy := `{"mode":"all_limits_required","unit":"quota","limits":[{"key":"5h","name":"5 小时额度","amount":100,"window_seconds":18000,"reset":"rolling"}]}`
	plan := insertQuotaPolicySubscriptionPlan(t, 9105, policy)
	sub := insertActiveUserSubscriptionForPolicyTest(t, 7005, plan, 0)
	now := time.Now().Unix()
	sub.StartTime = now - 8*3600
	require.NoError(t, DB.Save(sub).Error)
	insertSubscriptionConsumeLogForPolicyTest(t, 7005, sub.Id, 1, time.Now().Add(-6*time.Hour).Unix())
	insertSubscriptionConsumeLogForPolicyTest(t, 7005, sub.Id, 90, time.Now().Add(-4*time.Hour).Unix())

	res, err := PreConsumeUserSubscription("quota-policy-next-5h", 7005, "gpt-5.5", 0, 20)
	require.NoError(t, err)
	require.EqualValues(t, 20, res.PreConsumed)
}

func TestPreConsumeUserSubscriptionKeepsFiveHourBucketAnchoredToFirstUseAfterGap(t *testing.T) {
	truncateTables(t)

	policy := `{"mode":"all_limits_required","unit":"quota","limits":[{"key":"5h","name":"5 小时额度","amount":100,"window_seconds":18000,"reset":"rolling"}]}`
	plan := insertQuotaPolicySubscriptionPlan(t, 9109, policy)
	sub := insertActiveUserSubscriptionForPolicyTest(t, 7009, plan, 0)
	now := time.Now().Unix()
	sub.StartTime = now - 16*3600
	require.NoError(t, DB.Save(sub).Error)
	insertSubscriptionConsumeLogForPolicyTest(t, 7009, sub.Id, 10, now-12*3600)
	insertSubscriptionConsumeLogForPolicyTest(t, 7009, sub.Id, 90, now-4*3600)

	_, err := PreConsumeUserSubscription("quota-policy-5h-after-gap", 7009, "gpt-5.5", 0, 20)
	require.Error(t, err)
	require.Contains(t, err.Error(), "subscription quota insufficient")
}

func TestPreConsumeUserSubscriptionStartsNewSevenDayBucketFromFirstUse(t *testing.T) {
	truncateTables(t)

	policy := `{"mode":"all_limits_required","unit":"quota","limits":[{"key":"7d","name":"7 天额度","amount":100,"window_seconds":604800,"reset":"rolling"}]}`
	plan := insertQuotaPolicySubscriptionPlan(t, 9106, policy)
	sub := insertActiveUserSubscriptionForPolicyTest(t, 7006, plan, 0)
	now := time.Now().Unix()
	sub.StartTime = now - 30*24*3600
	require.NoError(t, DB.Save(sub).Error)
	insertSubscriptionConsumeLogForPolicyTest(t, 7006, sub.Id, 90, now-6*24*3600)

	_, err := PreConsumeUserSubscription("quota-policy-7d-bucket", 7006, "gpt-5.5", 0, 20)
	require.Error(t, err)
	require.Contains(t, err.Error(), "subscription quota insufficient")
}

func TestPreConsumeUserSubscriptionRejectsQuotaPolicySevenDayCurrentBucket(t *testing.T) {
	truncateTables(t)

	policy := `{"mode":"all_limits_required","unit":"quota","limits":[{"key":"7d","name":"7 天额度","amount":100,"window_seconds":604800,"reset":"rolling"}]}`
	plan := insertQuotaPolicySubscriptionPlan(t, 9108, policy)
	sub := insertActiveUserSubscriptionForPolicyTest(t, 7008, plan, 0)
	now := time.Now().Unix()
	sub.StartTime = now - 8*24*3600
	require.NoError(t, DB.Save(sub).Error)
	insertSubscriptionConsumeLogForPolicyTest(t, 7008, sub.Id, 90, now-12*3600)

	_, err := PreConsumeUserSubscription("quota-policy-7d-current-bucket", 7008, "gpt-5.5", 0, 20)
	require.Error(t, err)
	require.Contains(t, err.Error(), "subscription quota insufficient")
}

func TestPreConsumeUserSubscriptionUsesPreConsumedRecordsForQuotaPolicyLimit(t *testing.T) {
	truncateTables(t)

	policy := `{"mode":"all_limits_required","unit":"quota","limits":[{"key":"5h","name":"5 小时额度","amount":100,"window_seconds":18000,"reset":"rolling"}]}`
	plan := insertQuotaPolicySubscriptionPlan(t, 9107, policy)
	sub := insertActiveUserSubscriptionForPolicyTest(t, 7007, plan, 0)
	require.NoError(t, DB.Create(&SubscriptionPreConsumeRecord{
		RequestId:          "existing-pre-consume",
		UserId:             7007,
		UserSubscriptionId: sub.Id,
		PreConsumed:        90,
		Status:             "consumed",
		CreatedAt:          time.Now().Add(-time.Hour).Unix(),
		UpdatedAt:          time.Now().Add(-time.Hour).Unix(),
	}).Error)

	_, err := PreConsumeUserSubscription("quota-policy-pre-consume", 7007, "gpt-5.5", 0, 20)
	require.Error(t, err)
	require.Contains(t, err.Error(), "subscription quota insufficient")
}

func TestGetAllUserSubscriptionsIncludesQuotaPolicyStatus(t *testing.T) {
	truncateTables(t)

	policy := `{"mode":"all_limits_required","unit":"quota","limits":[{"key":"5h","name":"5 小时额度","amount":100,"window_seconds":18000,"reset":"rolling"},{"key":"7d","name":"7 天额度","amount":400,"window_seconds":604800,"reset":"rolling"},{"key":"monthly","name":"月总额度","amount":1000,"window_seconds":2592000,"reset":"subscription_cycle"}]}`
	plan := insertQuotaPolicySubscriptionPlan(t, 9111, policy)
	plan.QuotaResetPeriod = SubscriptionResetMonthly
	require.NoError(t, DB.Save(plan).Error)

	now := time.Now().Unix()
	sub := &UserSubscription{
		UserId:        7011,
		PlanId:        plan.Id,
		AmountTotal:   1000,
		AmountUsed:    300,
		StartTime:     now - 24*3600,
		EndTime:       now + 29*24*3600,
		Status:        "active",
		LastResetTime: now - 24*3600,
		NextResetTime: now + 29*24*3600,
		UpgradeGroup:  plan.UpgradeGroup,
	}
	require.NoError(t, DB.Create(sub).Error)
	insertSubscriptionConsumeLogForPolicyTest(t, 7011, sub.Id, 70, now-3600)
	require.NoError(t, DB.Create(&SubscriptionPreConsumeRecord{
		RequestId:          "quota-status-pre-consume",
		UserId:             7011,
		UserSubscriptionId: sub.Id,
		PreConsumed:        20,
		Status:             "pending",
		CreatedAt:          now - 1800,
		UpdatedAt:          now - 1800,
	}).Error)

	summaries, err := GetAllUserSubscriptions(7011)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	statuses := summaries[0].QuotaPolicyStatus
	require.Len(t, statuses, 3)

	byKey := map[string]SubscriptionQuotaPolicyStatus{}
	for _, status := range statuses {
		byKey[status.Key] = status
	}
	require.EqualValues(t, 90, byKey["5h"].Used)
	require.EqualValues(t, 100, byKey["5h"].Amount)
	require.EqualValues(t, 400, byKey["7d"].Amount)
	require.Greater(t, byKey["5h"].ResetTime, now)
	require.Greater(t, byKey["7d"].ResetTime, now)
	require.EqualValues(t, 300, byKey["monthly"].Used)
	require.EqualValues(t, 1000, byKey["monthly"].Amount)
	require.EqualValues(t, sub.NextResetTime, byKey["monthly"].ResetTime)
}

func TestGetAllUserSubscriptionsDoesNotDoubleCountSettledPreConsumeRecords(t *testing.T) {
	truncateTables(t)

	policy := `{"mode":"all_limits_required","unit":"quota","limits":[{"key":"7d","name":"7 天额度","amount":400,"window_seconds":604800,"reset":"rolling"},{"key":"monthly","name":"月总额度","amount":1000,"window_seconds":2592000,"reset":"subscription_cycle"}]}`
	plan := insertQuotaPolicySubscriptionPlan(t, 9114, policy)
	plan.QuotaResetPeriod = SubscriptionResetMonthly
	require.NoError(t, DB.Save(plan).Error)

	now := time.Now().Unix()
	sub := &UserSubscription{
		UserId:        7014,
		PlanId:        plan.Id,
		AmountTotal:   1000,
		AmountUsed:    30,
		StartTime:     now - 24*3600,
		EndTime:       now + 29*24*3600,
		Status:        "active",
		LastResetTime: now - 24*3600,
		NextResetTime: now + 29*24*3600,
		UpgradeGroup:  plan.UpgradeGroup,
	}
	require.NoError(t, DB.Create(sub).Error)
	require.NoError(t, DB.Create(&SubscriptionPreConsumeRecord{
		RequestId:          "settled-pre-consume",
		UserId:             7014,
		UserSubscriptionId: sub.Id,
		PreConsumed:        80,
		Status:             "consumed",
		CreatedAt:          now - 1800,
		UpdatedAt:          now - 1800,
	}).Error)
	insertSubscriptionConsumeLogWithRequestForPolicyTest(t, 7014, sub.Id, 30, now-1700, "settled-pre-consume")

	summaries, err := GetAllUserSubscriptions(7014)
	require.NoError(t, err)
	require.Len(t, summaries, 1)

	byKey := map[string]SubscriptionQuotaPolicyStatus{}
	for _, status := range summaries[0].QuotaPolicyStatus {
		byKey[status.Key] = status
	}
	require.EqualValues(t, 30, byKey["7d"].Used)
	require.EqualValues(t, 30, byKey["monthly"].Used)
}

func TestGetAllUserSubscriptionsOmitsUnusedRollingQuotaResetTime(t *testing.T) {
	truncateTables(t)

	policy := `{"mode":"all_limits_required","unit":"quota","limits":[{"key":"5h","name":"5 小时额度","amount":100,"window_seconds":18000,"reset":"rolling"},{"key":"7d","name":"7 天额度","amount":400,"window_seconds":604800,"reset":"rolling"},{"key":"monthly","name":"月总额度","amount":1000,"window_seconds":2592000,"reset":"subscription_cycle"}]}`
	plan := insertQuotaPolicySubscriptionPlan(t, 9112, policy)
	plan.QuotaResetPeriod = SubscriptionResetMonthly
	require.NoError(t, DB.Save(plan).Error)

	now := time.Now().Unix()
	sub := &UserSubscription{
		UserId:        7012,
		PlanId:        plan.Id,
		AmountTotal:   1000,
		AmountUsed:    0,
		StartTime:     now - 24*3600,
		EndTime:       now + 29*24*3600,
		Status:        "active",
		LastResetTime: now - 24*3600,
		NextResetTime: now + 29*24*3600,
		UpgradeGroup:  plan.UpgradeGroup,
	}
	require.NoError(t, DB.Create(sub).Error)

	summaries, err := GetAllUserSubscriptions(7012)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	statuses := summaries[0].QuotaPolicyStatus
	require.Len(t, statuses, 3)

	byKey := map[string]SubscriptionQuotaPolicyStatus{}
	for _, status := range statuses {
		byKey[status.Key] = status
	}
	require.EqualValues(t, 0, byKey["5h"].Used)
	require.EqualValues(t, 0, byKey["5h"].ResetTime)
	require.EqualValues(t, 0, byKey["7d"].Used)
	require.EqualValues(t, 0, byKey["7d"].ResetTime)
	require.EqualValues(t, sub.NextResetTime, byKey["monthly"].ResetTime)
}

func TestGetAllUserSubscriptionsIgnoresOtherSubscriptionUseForRollingResetTime(t *testing.T) {
	truncateTables(t)

	policy := `{"mode":"all_limits_required","unit":"quota","limits":[{"key":"7d","name":"7 天额度","amount":400,"window_seconds":604800,"reset":"rolling"}]}`
	plan := insertQuotaPolicySubscriptionPlan(t, 9113, policy)
	now := time.Now().Unix()
	usedSub := &UserSubscription{
		UserId:      7013,
		PlanId:      plan.Id,
		AmountTotal: 1000,
		StartTime:   now - 24*3600,
		EndTime:     now + 29*24*3600,
		Status:      "active",
	}
	unusedSub := &UserSubscription{
		UserId:      7013,
		PlanId:      plan.Id,
		AmountTotal: 1000,
		StartTime:   now - 24*3600,
		EndTime:     now + 29*24*3600,
		Status:      "active",
	}
	require.NoError(t, DB.Create(usedSub).Error)
	require.NoError(t, DB.Create(unusedSub).Error)
	insertSubscriptionConsumeLogForPolicyTest(t, 7013, usedSub.Id, 10, now-3600)

	statuses, err := BuildSubscriptionQuotaPolicyStatus(unusedSub, plan, now)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	require.EqualValues(t, 0, statuses[0].Used)
	require.EqualValues(t, 0, statuses[0].ResetTime)
}

func TestCalcNextResetTimeUsesMonthlyBillingAnchor(t *testing.T) {
	plan := &SubscriptionPlan{QuotaResetPeriod: SubscriptionResetMonthly}
	base := time.Date(2026, time.June, 21, 15, 30, 0, 0, time.UTC)

	next := calcNextResetTime(base, plan, base.Add(365*24*time.Hour).Unix())

	require.EqualValues(t, time.Date(2026, time.July, 21, 15, 30, 0, 0, time.UTC).Unix(), next)
}

func TestCalcNextResetTimeFallsBackToEndOfMonthForMissingBillingDay(t *testing.T) {
	plan := &SubscriptionPlan{QuotaResetPeriod: SubscriptionResetMonthly}
	base := time.Date(2026, time.January, 31, 12, 0, 0, 0, time.UTC)

	next := calcNextResetTime(base, plan, base.Add(365*24*time.Hour).Unix())

	require.EqualValues(t, time.Date(2026, time.February, 28, 12, 0, 0, 0, time.UTC).Unix(), next)
}

func TestCalcNextMonthlyBillingResetPreservesOriginalBillingDayAfterShortMonth(t *testing.T) {
	start := time.Date(2026, time.January, 31, 12, 0, 0, 0, time.UTC)
	now := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC).Unix()
	end := time.Date(2027, time.January, 31, 12, 0, 0, 0, time.UTC).Unix()

	next := calcNextMonthlyBillingResetTime(start.Unix(), now, end)

	require.EqualValues(t, time.Date(2026, time.March, 31, 12, 0, 0, 0, time.UTC).Unix(), next)
}

func TestMaybeResetMonthlySubscriptionPreservesOriginalBillingDayAfterShortMonth(t *testing.T) {
	truncateTables(t)

	plan := insertQuotaPolicySubscriptionPlan(t, 9110, "")
	plan.QuotaResetPeriod = SubscriptionResetMonthly
	require.NoError(t, DB.Save(plan).Error)

	start := time.Date(2026, time.January, 31, 12, 0, 0, 0, time.UTC)
	sub := &UserSubscription{
		UserId:        7010,
		PlanId:        plan.Id,
		AmountTotal:   1000,
		AmountUsed:    500,
		StartTime:     start.Unix(),
		EndTime:       time.Date(2027, time.January, 31, 12, 0, 0, 0, time.UTC).Unix(),
		Status:        "active",
		LastResetTime: start.Unix(),
		NextResetTime: time.Date(2026, time.February, 28, 12, 0, 0, 0, time.UTC).Unix(),
		UpgradeGroup:  plan.UpgradeGroup,
	}
	require.NoError(t, DB.Create(sub).Error)

	now := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC).Unix()
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return maybeResetUserSubscriptionWithPlanTx(tx, sub, plan, now)
	}))

	var refreshed UserSubscription
	require.NoError(t, DB.Where("id = ?", sub.Id).First(&refreshed).Error)
	require.EqualValues(t, 0, refreshed.AmountUsed)
	require.EqualValues(t, time.Date(2026, time.February, 28, 12, 0, 0, 0, time.UTC).Unix(), refreshed.LastResetTime)
	require.EqualValues(t, time.Date(2026, time.March, 31, 12, 0, 0, 0, time.UTC).Unix(), refreshed.NextResetTime)
}

func TestCalcPlanEndTimeUsesNaturalMonthDuration(t *testing.T) {
	plan := &SubscriptionPlan{DurationUnit: SubscriptionDurationMonth, DurationValue: 1}
	start := time.Date(2026, time.January, 31, 12, 0, 0, 0, time.UTC)

	end, err := calcPlanEndTime(start, plan)

	require.NoError(t, err)
	require.EqualValues(t, time.Date(2026, time.February, 28, 12, 0, 0, 0, time.UTC).Unix(), end)
}

func TestCalcPlanEndTimeUsesNaturalYearDuration(t *testing.T) {
	plan := &SubscriptionPlan{DurationUnit: SubscriptionDurationYear, DurationValue: 1}
	start := time.Date(2026, time.June, 21, 15, 30, 0, 0, time.UTC)

	end, err := calcPlanEndTime(start, plan)

	require.NoError(t, err)
	require.EqualValues(t, time.Date(2027, time.June, 21, 15, 30, 0, 0, time.UTC).Unix(), end)
}

func TestPreConsumeUserSubscriptionAllowsQuotaPolicyWithinLimits(t *testing.T) {
	truncateTables(t)

	policy := `{"mode":"all_limits_required","unit":"quota","limits":[{"key":"5h","name":"5 小时额度","amount":100,"window_seconds":18000,"reset":"rolling"},{"key":"monthly","name":"月总额度","amount":1000,"window_seconds":2592000,"reset":"subscription_cycle"}]}`
	plan := insertQuotaPolicySubscriptionPlan(t, 9102, policy)
	insertActiveUserSubscriptionForPolicyTest(t, 7002, plan, 80)

	res, err := PreConsumeUserSubscription("quota-policy-ok", 7002, "gpt-5.5", 0, 20)
	require.NoError(t, err)
	require.EqualValues(t, 20, res.PreConsumed)
	require.EqualValues(t, 100, res.AmountUsedAfter)
}

func TestPostConsumeUserSubscriptionDeltaRejectsQuotaPolicyLimit(t *testing.T) {
	truncateTables(t)

	policy := `{"mode":"all_limits_required","unit":"quota","limits":[{"key":"monthly","name":"月总额度","amount":100,"window_seconds":2592000,"reset":"subscription_cycle"}]}`
	plan := insertQuotaPolicySubscriptionPlan(t, 9103, policy)
	sub := insertActiveUserSubscriptionForPolicyTest(t, 7003, plan, 90)

	err := PostConsumeUserSubscriptionDelta(sub.Id, 20)
	require.Error(t, err)
	require.Contains(t, err.Error(), "subscription quota insufficient")
}

func TestValidateSubscriptionQuotaPolicyJSONRejectsEmptyLimits(t *testing.T) {
	err := ValidateSubscriptionQuotaPolicyJSON(`{"mode":"all_limits_required","unit":"quota","limits":[{"key":"5h","amount":0,"window_seconds":18000}]}`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no valid limits")
}

func TestAdminBindSubscriptionRefreshesExistingActiveSubscriptionSnapshot(t *testing.T) {
	truncateTables(t)

	plan := insertQuotaPolicySubscriptionPlan(t, 9104, `{"mode":"all_limits_required","unit":"quota","limits":[{"key":"monthly","name":"月总额度","amount":1000,"window_seconds":2592000,"reset":"subscription_cycle"}]}`)
	plan.QuotaResetPeriod = SubscriptionResetMonthly
	plan.TotalAmount = 1000
	require.NoError(t, DB.Save(plan).Error)

	sub := insertActiveUserSubscriptionForPolicyTest(t, 7004, plan, 90)
	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", plan.Id).Updates(map[string]interface{}{
		"total_amount":       2000,
		"quota_reset_period": SubscriptionResetMonthly,
	}).Error)
	InvalidateSubscriptionPlanCache(plan.Id)

	_, err := AdminBindSubscription(7004, plan.Id, "")
	require.NoError(t, err)

	var refreshed UserSubscription
	require.NoError(t, DB.Where("id = ?", sub.Id).First(&refreshed).Error)
	require.EqualValues(t, 2000, refreshed.AmountTotal)
	require.EqualValues(t, 90, refreshed.AmountUsed)
	require.Greater(t, refreshed.NextResetTime, int64(0))
}
