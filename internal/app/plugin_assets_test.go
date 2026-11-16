package app

import (
	"net/http"
	"net/http/httptest"
	desktopplugins "ssh-connect/internal/desktop/plugins"
	"strings"
	"testing"
)

func TestPluginAssetsBypassWailsInjectionAndRestrictFrameNavigation(t *testing.T) {
	app := &App{desktopPluginViews: desktopplugins.NewViewHost()}
	document, err := app.desktopPluginViews.Open(1, "example.monitor", "session", "<div>fixture</div>", nil)
	if err != nil {
		t.Fatal(err)
	}
	downstream := false
	handler := app.desktopAssetMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downstream = true
		w.Write([]byte("Wails injection placeholder"))
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "wails://localhost"+document.URL, nil))
	if downstream || strings.Contains(response.Body.String(), "Wails injection placeholder") {
		t.Fatal("plugin HTML entered the Wails asset injection chain")
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "wails://localhost/", nil))
	policy := response.Header().Get("Content-Security-Policy")
	if !downstream || !strings.Contains(policy, "wails://localhost/__plugin_view/") || strings.Contains(policy, "'self'") || strings.Contains(policy, "data:") {
		t.Fatal("parent CSP permits navigating a plugin frame into a fresh app/external realm", policy)
	}
}
