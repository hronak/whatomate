package frontend

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
)

// mimeTypes maps file extensions to MIME types
var mimeTypes = map[string]string{
	".js":    "application/javascript",
	".mjs":   "application/javascript",
	".css":   "text/css",
	".html":  "text/html",
	".json":  "application/json",
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".jpeg":  "image/jpeg",
	".gif":   "image/gif",
	".svg":   "image/svg+xml",
	".ico":   "image/x-icon",
	".woff":  "font/woff",
	".woff2": "font/woff2",
	".ttf":   "font/ttf",
	".eot":   "application/vnd.ms-fontobject",
}

//go:embed all:dist
var distFS embed.FS

// Handler returns a fasthttp handler that serves the frontend embedded into the
// binary at build time by `make build-prod`.
//
// basePath should be empty string for root deployment or "/subpath" for
// subdirectory. If the frontend is not embedded, the returned handler shows a
// helpful message instead.
func Handler(basePath string) fasthttp.RequestHandler {
	distSubFS, err := fs.Sub(distFS, "dist")
	if err != nil {
		return notEmbeddedHandler("Frontend not embedded: " + err.Error())
	}
	return handlerFor(basePath, distSubFS, false)
}

// DirHandler returns a handler that serves the frontend from dir on disk
// (typically frontend/dist) rather than from the embedded copy.
//
// This exists so the backend port doesn't lie during development. The embedded
// copy is whatever was snapshotted at the last `make build-prod`, so it goes
// stale silently — you edit the frontend, reload, and see the previous build
// with no indication why. Serving from disk means a plain `make frontend-build`
// is enough, with no Go rebuild and no restart: index.html is re-read per
// request, so the new asset hashes are picked up immediately.
//
// It is still a build, not live source. Vite on the frontend port remains the
// thing to develop against.
func DirHandler(basePath, dir string) fasthttp.RequestHandler {
	return handlerFor(basePath, os.DirFS(dir), true)
}

// DirHasIndex reports whether dir looks like a built frontend, so callers can
// warn at startup rather than serving 404s.
func DirHasIndex(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, "index.html"))
	return err == nil && !st.IsDir()
}

// IsEmbedded returns true if the frontend dist folder is embedded
func IsEmbedded() bool {
	entries, err := distFS.ReadDir("dist")
	if err != nil {
		return false
	}
	return len(entries) > 0
}

// injectBasePath rewrites index.html so a subpath deployment resolves its
// relative asset URLs correctly and the SPA knows where it is mounted.
func injectBasePath(indexContent []byte, basePath string) []byte {
	// Inject base tag right after <head> so it's processed before any relative URLs.
	// Base tag ensures relative URLs (./assets/...) resolve from basePath, not current page path.
	baseHref := basePath + "/"
	if basePath == "" {
		baseHref = "/"
	}
	baseTag := fmt.Sprintf(`<head><base href="%s">`, baseHref)
	modifiedHTML := strings.Replace(string(indexContent), "<head>", baseTag, 1)

	// Inject base path script before </head>
	basePathScript := fmt.Sprintf(`<script>window.__BASE_PATH__ = "%s";</script></head>`, basePath)
	return []byte(strings.Replace(modifiedHTML, "</head>", basePathScript, 1))
}

// handlerFor serves fsys as a single-page app: real files when they exist,
// index.html for everything else.
//
// When reload is false the injected index.html is built once up front, which
// also lets construction fail fast if the frontend isn't there. When it is true
// index.html is re-read on every request so an out-of-band rebuild of the
// directory takes effect without a restart.
func handlerFor(basePath string, fsys fs.FS, reload bool) fasthttp.RequestHandler {
	// Normalize base path
	basePath = strings.TrimSuffix(basePath, "/")

	// Written once during construction in the cached case and read-only
	// thereafter, so concurrent requests never race on it. In reload mode it
	// stays nil and every call re-reads from fsys.
	var cachedIndexHTML []byte

	indexHTML := func() ([]byte, error) {
		if cachedIndexHTML != nil {
			return cachedIndexHTML, nil
		}
		raw, err := fs.ReadFile(fsys, "index.html")
		if err != nil {
			return nil, err
		}
		injected := injectBasePath(raw, basePath)
		if !reload {
			cachedIndexHTML = injected
		}
		return injected, nil
	}

	if !reload {
		if _, err := indexHTML(); err != nil {
			return notEmbeddedHandler("Frontend not embedded: index.html not found. Run 'make build-prod' to embed frontend.")
		}
	}

	// Create file server
	fileServer := http.FileServer(http.FS(fsys))

	// Wrap with SPA fallback and proper MIME types
	spaHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Try to serve the file
		if path != "/" && !strings.HasPrefix(path, "/api") {
			// Check if file exists
			filePath := strings.TrimPrefix(path, "/")
			file, err := fsys.Open(filePath)
			if err == nil {
				defer func() { _ = file.Close() }()

				// Get file info for size
				stat, err := file.Stat()
				if err != nil {
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return
				}

				// Skip directories
				if stat.IsDir() {
					fileServer.ServeHTTP(w, r)
					return
				}

				// Set correct Content-Type based on file extension
				ext := strings.ToLower(filepath.Ext(filePath))
				if mimeType, ok := mimeTypes[ext]; ok {
					w.Header().Set("Content-Type", mimeType)
				} else {
					w.Header().Set("Content-Type", "application/octet-stream")
				}

				// Check Accept-Encoding and serve pre-compressed if available
				acceptEncoding := r.Header.Get("Accept-Encoding")
				var content []byte
				var contentEncoding string

				// Try Brotli first (better compression)
				if strings.Contains(acceptEncoding, "br") {
					if brContent, err := fs.ReadFile(fsys, filePath+".br"); err == nil {
						content = brContent
						contentEncoding = "br"
					}
				}

				// Fall back to gzip
				if content == nil && strings.Contains(acceptEncoding, "gzip") {
					if gzContent, err := fs.ReadFile(fsys, filePath+".gz"); err == nil {
						content = gzContent
						contentEncoding = "gzip"
					}
				}

				// Fall back to uncompressed
				if content == nil {
					content, err = fs.ReadFile(fsys, filePath)
					if err != nil {
						http.Error(w, "Internal Server Error", http.StatusInternalServerError)
						return
					}
				}

				if contentEncoding != "" {
					w.Header().Set("Content-Encoding", contentEncoding)
				}
				w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
				_, _ = w.Write(content)
				return
			}
		}

		// For root or non-existent files (SPA routes), serve modified index.html
		if path == "/" || (!strings.HasPrefix(path, "/api") && !strings.Contains(path, ".")) {
			index, err := indexHTML()
			if err != nil {
				http.Error(w, "Frontend not built: index.html not found. Run 'make frontend-build'.", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			// The disk copy changes underneath us by design; don't let a browser
			// or proxy pin the old asset hashes.
			if reload {
				w.Header().Set("Cache-Control", "no-store")
			}
			_, _ = w.Write(index)
			return
		}

		// Serve the actual file
		fileServer.ServeHTTP(w, r)
	})

	// Convert to fasthttp handler
	return fasthttpadaptor.NewFastHTTPHandler(spaHandler)
}

// notEmbeddedHandler returns a handler that displays a message when frontend is not embedded
func notEmbeddedHandler(message string) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		ctx.SetContentType("text/plain; charset=utf-8")
		ctx.SetStatusCode(fasthttp.StatusNotFound)
		_, _ = ctx.WriteString(message)
	}
}
