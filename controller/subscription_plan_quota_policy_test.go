package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupSubscriptionPlanControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousUsingSQLite := common.UsingSQLite
	previousUsingMySQL := common.UsingMySQL
	previousUsingPostgreSQL := common.UsingPostgreSQL
	previousRedisEnabled := common.RedisEnabled

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	model.DB = db
	model.LOG_DB = db

	if err := db.AutoMigrate(&model.SubscriptionPlan{}); err != nil {
		t.Fatalf("failed to migrate subscription plans: %v", err)
	}

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.UsingSQLite = previousUsingSQLite
		common.UsingMySQL = previousUsingMySQL
		common.UsingPostgreSQL = previousUsingPostgreSQL
		common.RedisEnabled = previousRedisEnabled
	})

	return db
}

func TestAdminListSubscriptionPlansReturnsManagedQuotaAmounts(t *testing.T) {
	db := setupSubscriptionPlanControllerTestDB(t)
	policy := `{"mode":"all_limits_required","unit":"quota","limits":[{"key":"5h","name":"5 小时额度","amount":100000,"window_seconds":18000,"reset":"rolling"},{"key":"7d","name":"7 天额度","amount":400000,"window_seconds":604800,"reset":"rolling"},{"key":"monthly","name":"月总额度","amount":1000000,"window_seconds":2592000,"reset":"subscription_cycle"}]}`
	if err := db.Create(&model.SubscriptionPlan{
		Title:           "Free",
		DurationUnit:    model.SubscriptionDurationMonth,
		DurationValue:   1,
		Enabled:         true,
		QuotaPolicyJSON: policy,
	}).Error; err != nil {
		t.Fatalf("failed to create plan: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/subscription/admin/plans", nil)

	AdminListSubscriptionPlans(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Success bool `json:"success"`
		Data    []struct {
			Plan struct {
				QuotaPolicy5HAmount      int64 `json:"quota_policy_5h_amount"`
				QuotaPolicy7DAmount      int64 `json:"quota_policy_7d_amount"`
				QuotaPolicyMonthlyAmount int64 `json:"quota_policy_monthly_amount"`
			} `json:"plan"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v body=%s", err, recorder.Body.String())
	}
	if !response.Success || len(response.Data) != 1 {
		t.Fatalf("unexpected response: success=%v len=%d body=%s", response.Success, len(response.Data), recorder.Body.String())
	}
	got := response.Data[0].Plan
	if got.QuotaPolicy5HAmount != 100000 || got.QuotaPolicy7DAmount != 400000 || got.QuotaPolicyMonthlyAmount != 1000000 {
		t.Fatalf(
			"unexpected managed quota amounts: got 5h=%d 7d=%d monthly=%d",
			got.QuotaPolicy5HAmount,
			got.QuotaPolicy7DAmount,
			got.QuotaPolicyMonthlyAmount,
		)
	}
}

func TestAdminListSubscriptionPlansReadsExistingQuotaPolicyColumn(t *testing.T) {
	db := setupSubscriptionPlanControllerTestDB(t)
	policy := `{"mode":"all_limits_required","unit":"quota","limits":[{"key":"5h","name":"5 小时额度","amount":100000,"window_seconds":18000,"reset":"rolling"},{"key":"7d","name":"7 天额度","amount":400000,"window_seconds":604800,"reset":"rolling"},{"key":"monthly","name":"月总额度","amount":1000000,"window_seconds":2592000,"reset":"subscription_cycle"}]}`
	if err := db.Exec(`
		INSERT INTO subscription_plans
			(title, price_amount, currency, duration_unit, duration_value, enabled, total_amount, quota_reset_period, quota_policy, created_at, updated_at)
		VALUES
			(?, 0, 'USD', ?, 1, true, 1000000, 'monthly', ?, 1, 1)
	`, "Free", model.SubscriptionDurationMonth, policy).Error; err != nil {
		t.Fatalf("failed to seed plan through quota_policy column: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/subscription/admin/plans", nil)

	AdminListSubscriptionPlans(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Success bool `json:"success"`
		Data    []struct {
			Plan struct {
				QuotaPolicy              string `json:"quota_policy"`
				QuotaPolicy5HAmount      int64  `json:"quota_policy_5h_amount"`
				QuotaPolicy7DAmount      int64  `json:"quota_policy_7d_amount"`
				QuotaPolicyMonthlyAmount int64  `json:"quota_policy_monthly_amount"`
			} `json:"plan"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v body=%s", err, recorder.Body.String())
	}
	if !response.Success || len(response.Data) != 1 {
		t.Fatalf("unexpected response: success=%v len=%d body=%s", response.Success, len(response.Data), recorder.Body.String())
	}
	got := response.Data[0].Plan
	if got.QuotaPolicy == "" {
		t.Fatalf("expected quota_policy to be read from existing column, body=%s", recorder.Body.String())
	}
	if got.QuotaPolicy5HAmount != 100000 || got.QuotaPolicy7DAmount != 400000 || got.QuotaPolicyMonthlyAmount != 1000000 {
		t.Fatalf(
			"unexpected managed quota amounts: got 5h=%d 7d=%d monthly=%d",
			got.QuotaPolicy5HAmount,
			got.QuotaPolicy7DAmount,
			got.QuotaPolicyMonthlyAmount,
		)
	}
}
