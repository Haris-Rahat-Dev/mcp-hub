package admin

import (
	"io/fs"
	"net/http"
)

// FS returns the embedded control-panel static files (under ui/dist).
func FS() (fs.FS, error) {
	return fs.Sub(uiFS, "ui/dist")
}

// UIHandler serves the SPA under /ui/.
func UIHandler() http.Handler {
	sub, err := FS()
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "control panel assets not embedded", http.StatusNotFound)
		})
	}
	return http.StripPrefix("/ui/", http.FileServer(http.FS(sub)))
}
