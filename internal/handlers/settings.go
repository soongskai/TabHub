package handlers

import "net/http"

func (h *Handler) wallpapers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	file, header, err := r.FormFile("wallpaper")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "wallpaper required"})
		return
	}
	defer file.Close()
	content, err := readLimitedFile(file, 10<<20)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid wallpaper"})
		return
	}
	if err := h.Service.SaveWallpaper(header.Filename, content, h.Config.DataDir); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
}

func (h *Handler) wallpaperByID(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.URL.Path, "/api/wallpapers/")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if r.Method == http.MethodPost {
		if err := h.Service.SetActiveWallpaper(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if r.Method == http.MethodDelete {
		if err := h.Service.DeleteWallpaper(id, h.Config.DataDir); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
}

func (h *Handler) settings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	for _, key := range []string{"search_engine", "show_time", "show_date", "overlay_opacity", "theme"} {
		if value := r.FormValue(key); value != "" {
			if key == "theme" && value != "dark" && value != "light" {
				value = "dark"
			}
			if err := h.Service.UpsertSetting(key, value); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
