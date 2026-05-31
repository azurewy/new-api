package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestUpdateChannelBalanceUsesCompatibleUsageEndpointFor66AIAnthropic(t *testing.T) {
	setupModelListControllerTestDB(t)

	var requestedPath string
	var authorization string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"remaining":12.34,"unit":"USD","is_active":true}`))
	}))
	t.Cleanup(upstream.Close)

	baseURL := upstream.URL + "/antigravity"
	channel := &model.Channel{
		Type:    constant.ChannelTypeAnthropic,
		Key:     "test-api-key",
		Name:    "66ai",
		BaseURL: &baseURL,
	}
	require.NoError(t, model.DB.Create(channel).Error)

	balance, err := updateChannelBalance(channel)

	require.NoError(t, err)
	require.Equal(t, "/antigravity/v1/usage", requestedPath)
	require.Equal(t, "Bearer test-api-key", authorization)
	require.InDelta(t, 12.34, balance, 0.0001)

	var updated model.Channel
	require.NoError(t, model.DB.First(&updated, channel.Id).Error)
	require.InDelta(t, 12.34, updated.Balance, 0.0001)
}

func TestUpdateChannelBalanceDoesNotDuplicateCompatibleUsageV1Suffix(t *testing.T) {
	setupModelListControllerTestDB(t)

	var requestedPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"balance":6.66}`))
	}))
	t.Cleanup(upstream.Close)

	baseURL := upstream.URL + "/v1"
	channel := &model.Channel{
		Type:    constant.ChannelTypeAnthropic,
		Key:     "test-api-key",
		Name:    "66ai",
		BaseURL: &baseURL,
	}
	require.NoError(t, model.DB.Create(channel).Error)

	balance, err := updateChannelBalance(channel)

	require.NoError(t, err)
	require.Equal(t, "/v1/usage", requestedPath)
	require.InDelta(t, 6.66, balance, 0.0001)
}

func TestUpdateChannelBalanceFallsBackToCompatibleUsageEndpointForOpenAICompatibleChannels(t *testing.T) {
	for _, channelType := range []int{constant.ChannelTypeCustom, constant.ChannelTypeOpenAI} {
		t.Run(constant.GetChannelTypeName(channelType), func(t *testing.T) {
			setupModelListControllerTestDB(t)

			var requestedPaths []string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestedPaths = append(requestedPaths, r.URL.Path)
				if r.URL.Path == "/v1/usage" {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"quota":{"remaining":"9.87","unit":"USD"},"isValid":true}`))
					return
				}
				http.NotFound(w, r)
			}))
			t.Cleanup(upstream.Close)

			baseURL := upstream.URL
			channel := &model.Channel{
				Type:    channelType,
				Key:     "test-api-key",
				Name:    "66ai compatible",
				BaseURL: &baseURL,
			}
			require.NoError(t, model.DB.Create(channel).Error)

			balance, err := updateChannelBalance(channel)

			require.NoError(t, err)
			require.Contains(t, requestedPaths, "/v1/dashboard/billing/subscription")
			require.Contains(t, requestedPaths, "/v1/usage")
			require.InDelta(t, 9.87, balance, 0.0001)
		})
	}
}
