package webconsole

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
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
	mux.Handle("/", staticHeaders(staticBundle(staticDir)))
	return mux
}

// staticBundle exposes only a built index document and immutable assets below
// assets/. It refuses directory listings, dot paths and all non-build files.
func staticBundle(staticDir string) http.Handler {
	root := filepath.Clean(staticDir)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		requested := path.Clean("/" + r.URL.Path)
		if strings.Contains(requested, "/.") {
			http.NotFound(w, r)
			return
		}
		var file string
		switch {
		case requested == "/" || requested == "/index.html":
			file = filepath.Join(root, "index.html")
		case strings.HasPrefix(requested, "/assets/") && !strings.HasSuffix(requested, "/"):
			file = filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(requested, "/")))
		default:
			http.NotFound(w, r)
			return
		}
		info, err := os.Stat(file)
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		if strings.HasPrefix(requested, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-store")
		}
		http.ServeFile(w, r, file)
	})
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
