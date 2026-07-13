package api

import (
	"net/http"

	"github.com/olshmore/ytter/internal/storage"
)

// PublicMediaHTTPHandler serves local branding uploads and media GETs
func (server *Server) PublicMediaHTTPHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		local, ok := server.objectStore.(*storage.Local)
		if !ok || local == nil {
			http.NotFound(w, r)
			return
		}
		local.HandleMedia(w, r)
	}
}
