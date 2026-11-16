package app

import (
	"net/http"
	desktopplugins "ssh-connect/internal/desktop/plugins"
	"strings"
)

// Plugin documents bypass Wails' internal asset/runtime injection. Parent CSP
// permits only these document routes, so a plugin cannot navigate its frame to
// the application, an external site or a fresh unrestricted data: realm.
func (a *App) desktopAssetMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		path := request.URL.Path
		if strings.HasPrefix(path, desktopplugins.ViewPath) {
			if a.desktopPluginViews == nil {
				http.NotFound(writer, request)
				return
			}
			a.desktopPluginViews.ServeHTTP(writer, request)
			return
		}
		origins := []string{"wails://localhost", "http://wails.localhost", "https://wails.localhost"}
		if request.URL.Host != "" && (request.URL.Scheme == "http" || request.URL.Scheme == "https" || request.URL.Scheme == "wails") {
			origin := request.URL.Scheme + "://" + request.URL.Host
			found := false
			for _, existing := range origins {
				if existing == origin {
					found = true
				}
			}
			if !found {
				origins = append(origins, origin)
			}
		}
		sources := []string{}
		for _, origin := range origins {
			sources = append(sources, origin+desktopplugins.ViewPath)
		}
		writer.Header().Set("Content-Security-Policy", "frame-src "+strings.Join(sources, " ")+"; object-src 'none'")
		next.ServeHTTP(writer, request)
	})
}
