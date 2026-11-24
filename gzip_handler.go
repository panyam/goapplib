package goapplib

import (
	"net/http"
	"path/filepath"
	"strings"
)

// GzipFileServer wraps an http.Handler to serve pre-compressed .gz files when available
// and the client accepts gzip encoding.
//
// For WASM files, this serves .wasm.gz (4.7MB) instead of .wasm (25MB), reducing
// transfer size by ~81%.
func GzipFileServer(root http.FileSystem) http.Handler {
	fileServer := http.FileServer(root)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if client accepts gzip
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			fileServer.ServeHTTP(w, r)
			return
		}

		// Only try gzip for certain file types
		ext := filepath.Ext(r.URL.Path)
		if ext != ".wasm" && ext != ".js" {
			fileServer.ServeHTTP(w, r)
			return
		}

		// Check if .gz version exists. root opens it, so a path that climbs out of root finds
		// nothing, and the check works for any http.FileSystem rather than http.Dir alone.
		gzPath := r.URL.Path + ".gz"
		if f, err := root.Open(gzPath); err == nil {
			f.Close()
			// Serve the gzipped file with proper headers
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Content-Type", getContentType(ext))
			w.Header().Set("Vary", "Accept-Encoding")

			// Modify request to point to .gz file
			r.URL.Path = gzPath
		}

		fileServer.ServeHTTP(w, r)
	})
}

func getContentType(ext string) string {
	switch ext {
	case ".wasm":
		return "application/wasm"
	case ".js":
		return "application/javascript"
	default:
		return "application/octet-stream"
	}
}
