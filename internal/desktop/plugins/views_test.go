package plugins

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestViewDocumentsAreOpaqueBoundedAndRevoked(t *testing.T) {
	host := NewViewHost()
	document, err := host.Open(1, "example.monitor", "session", "<script>pluginMarkup()</script>", nil)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	host.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://localhost"+document.URL, nil))
	if response.Code != 200 || !strings.Contains(response.Header().Get("Content-Security-Policy"), "sandbox allow-scripts") || !strings.Contains(response.Header().Get("Content-Security-Policy"), "connect-src 'none'") || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(response.Code, response.Header())
	}
	html := response.Body.String()
	if strings.Contains(html, "pluginMarkup()") {
		t.Fatal("untrusted markup was parsed before hardening")
	}
	marker := "data:text/javascript;base64,"
	at := strings.Index(html, marker) + len(marker)
	end := strings.Index(html[at:], `"`) + at
	source, err := base64.StdEncoding.DecodeString(html[at:end])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), "'webkit'") || strings.Index(string(source), "document.write(config.html)") < strings.Index(string(source), "Object.defineProperty(globalThis,key") {
		t.Fatal("native bridge/realm hardening does not precede markup")
	}
	if err = host.Close(2, document.InstanceID); err == nil {
		t.Fatal("foreign owner closed a view")
	}
	owner := uint(1)
	host.Release("example.monitor", &owner, "session")
	response = httptest.NewRecorder()
	host.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://localhost"+document.URL, nil))
	if response.Code != 404 {
		t.Fatal("revoked document still available")
	}
	for i := 0; i < 32; i++ {
		if _, err = host.Open(1, "example.monitor", "session", "<div>fixture</div>", nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = host.Open(1, "example.monitor", "session", "extra", nil); err == nil {
		t.Fatal("view quota was not enforced")
	}
}
