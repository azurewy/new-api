package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

func TestStartOIDCLoginRedirectsWithServerGeneratedState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	restoreOIDCStartSettings(t)
	common.SessionSecret = "test-session-secret"
	system_setting.ServerAddress = "https://llmapi.wzcon.com"
	*system_setting.GetOIDCSettings() = system_setting.OIDCSettings{
		Enabled:               true,
		ClientId:              "newapi",
		AuthorizationEndpoint: "https://api.wzcon.com/api/oidc/authorize",
	}

	router := gin.New()
	store := cookie.NewStore([]byte(common.SessionSecret))
	router.Use(sessions.Sessions("session", store))
	router.GET("/oauth/oidc/start", StartOIDCLogin)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/oauth/oidc/start?aff=invite1", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d body=%s", w.Code, w.Body.String())
	}
	location := w.Header().Get("Location")
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse redirect location: %v", err)
	}
	if parsed.Scheme != "https" || parsed.Host != "api.wzcon.com" || parsed.Path != "/api/oidc/authorize" {
		t.Fatalf("unexpected redirect location: %s", location)
	}
	query := parsed.Query()
	if query.Get("client_id") != "newapi" {
		t.Fatalf("expected client_id=newapi, got %q", query.Get("client_id"))
	}
	if query.Get("redirect_uri") != "https://llmapi.wzcon.com/oauth/oidc" {
		t.Fatalf("unexpected redirect_uri: %q", query.Get("redirect_uri"))
	}
	if query.Get("response_type") != "code" || query.Get("scope") != "openid profile email" {
		t.Fatalf("unexpected oidc params: %s", location)
	}
	if state := query.Get("state"); len(strings.TrimSpace(state)) < 12 {
		t.Fatalf("expected generated state, got %q", state)
	}
	if cookie := w.Header().Get("Set-Cookie"); !strings.Contains(cookie, "session=") {
		t.Fatalf("expected session cookie, got %q", cookie)
	}
}

func restoreOIDCStartSettings(t *testing.T) {
	t.Helper()
	previousServerAddress := system_setting.ServerAddress
	previousOIDCSettings := *system_setting.GetOIDCSettings()
	t.Cleanup(func() {
		system_setting.ServerAddress = previousServerAddress
		*system_setting.GetOIDCSettings() = previousOIDCSettings
	})
}
