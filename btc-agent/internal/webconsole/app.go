package webconsole

import (
	"net/http"
	"path/filepath"
)

// App serves the typed API before static assets. staticDir is an explicit build
// output directory, never the repository root or the runtime reports directory.
func (a *API) App(staticDir string) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/", a.Handler())
	mux.Handle("/healthz", a.Handler())
	if staticDir == "" {
		mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) })
		return mux
	}
	mux.Handle("/", staticHeaders(http.FileServer(http.Dir(filepath.Clean(staticDir)))))
	return mux
}

func staticHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'; object-src 'none'")
		next.ServeHTTP(w, r)
	})
}
