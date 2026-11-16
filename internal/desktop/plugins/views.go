package plugins

import (
	_ "embed"
	"encoding/base64"
	"errors"
	"github.com/google/uuid"
	"net/http"
	"strings"
	"sync"
)

const ViewPath = "/__plugin_view/"
const ViewCSP = "default-src 'none'; script-src 'unsafe-inline' data:; style-src 'unsafe-inline'; img-src data:; font-src data:; connect-src 'none'; frame-src 'none'; worker-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'; sandbox allow-scripts"
const PluginPermissionsPolicy = "camera=(), microphone=(), geolocation=(), clipboard-read=(), clipboard-write=(), display-capture=()"

//go:embed view_bootstrap.js
var viewBootstrap string

type ViewDocument struct {
	URL        string `json:"url"`
	InstanceID string `json:"instanceId"`
}
type viewEntry struct {
	owner                     uint
	pluginID, sessionID, html string
}
type ViewHost struct {
	mu                  sync.RWMutex
	documents           map[string]viewEntry
	nativeBridgeGuarded bool
	closed              bool
}

func NewViewHost() *ViewHost { return &ViewHost{documents: map[string]viewEntry{}} }
func (v *ViewHost) Open(owner uint, pluginID, sessionID, html string, bundle *Localizations) (ViewDocument, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return ViewDocument{}, errors.New("plugin views are closed")
	}
	count := 0
	for _, entry := range v.documents {
		if entry.owner == owner {
			count++
		}
	}
	if count >= 32 {
		return ViewDocument{}, errors.New("plugin view instance limit")
	}
	id := uuid.NewString()
	config, err := marshalJSON(struct {
		InstanceID          string         `json:"instanceId"`
		HTML                string         `json:"html"`
		Bundle              *Localizations `json:"bundle"`
		NativeBridgeGuarded bool           `json:"nativeBridgeGuarded"`
	}{id, html, bundle, v.nativeBridgeGuarded})
	if err != nil {
		return ViewDocument{}, err
	}
	source := viewBootstrap + "\ntry { initializePluginView(" + string(config) + "); } catch (error) { document.write('<p id=\"aterm-view-error\" role=\"alert\">Plugin view could not initialize.</p>'); console.error('Plugin view initialization failed', String(error.message).slice(0,300)); }"
	document := `<!doctype html><html><head><meta charset="utf-8"><script src="data:text/javascript;base64,` + base64.StdEncoding.EncodeToString([]byte(source)) + `"></script></head><body></body></html>`
	v.documents[id] = viewEntry{owner: owner, pluginID: pluginID, sessionID: sessionID, html: document}
	return ViewDocument{URL: ViewPath + id, InstanceID: id}, nil
}
func (v *ViewHost) Close(owner uint, id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if value, ok := v.documents[id]; ok && value.owner != owner {
		return errors.New("invalid view owner")
	}
	delete(v.documents, id)
	return nil
}
func (v *ViewHost) Release(id string, owner *uint, sessionID string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for key, value := range v.documents {
		if (id == "" || value.pluginID == id) && (owner == nil || value.owner == *owner) && (sessionID == "" || value.sessionID == sessionID) {
			delete(v.documents, key)
		}
	}
}
func (v *ViewHost) Document(id string) (string, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	value, ok := v.documents[id]
	return value.html, ok
}
func (v *ViewHost) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	id := strings.TrimPrefix(request.URL.Path, ViewPath)
	html, ok := v.Document(id)
	if !ok || request.URL.RawQuery != "" || !strings.HasPrefix(request.URL.Path, ViewPath) {
		http.NotFound(writer, request)
		return
	}
	ServePluginDocument(writer, request, html, ViewCSP)
}
func ServePluginDocument(writer http.ResponseWriter, request *http.Request, html, csp string) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Content-Security-Policy", csp)
	writer.Header().Set("Permissions-Policy", PluginPermissionsPolicy)
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	if request.Method == http.MethodGet {
		_, _ = writer.Write([]byte(html))
	}
}

func (v *ViewHost) SetNativeBridgeGuarded(guarded bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.nativeBridgeGuarded = guarded
}

func (v *ViewHost) Shutdown() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.closed = true
	v.documents = map[string]viewEntry{}
}
