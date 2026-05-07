package handlers

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"tabhub/internal/services"
)

func (h *Handler) apiHome(w http.ResponseWriter, r *http.Request) {
	groups, bookmarks, searchEngines, settings, wallpaper, err := h.Service.GetHomeData()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"groups":        groups,
		"bookmarks":     bookmarks,
		"searchEngines": searchEngines,
		"settings":      settings,
		"wallpaper":     wallpaper,
		"sessionDays":   h.Config.SessionDays,
	})
}

func (h *Handler) groups(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut && strings.TrimPrefix(r.URL.Path, "/api/groups/") == "reorder" {
		h.reorderGroups(w, r)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name required"})
		return
	}
	if err := h.Service.CreateGroup(name); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
}

func (h *Handler) groupByID(w http.ResponseWriter, r *http.Request) {
	if strings.TrimPrefix(r.URL.Path, "/api/groups/") == "reorder" {
		h.reorderGroups(w, r)
		return
	}
	id, err := parseID(r.URL.Path, "/api/groups/")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	switch r.Method {
	case http.MethodPut:
		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name required"})
			return
		}
		if err := h.Service.UpdateGroup(id, name); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case http.MethodDelete:
		if err := h.Service.DeleteGroup(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *Handler) reorderGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	rawIDs := strings.TrimSpace(r.FormValue("ids"))
	if rawIDs == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ids required"})
		return
	}
	parts := strings.Split(rawIDs, ",")
	ids := make([]int64, 0, len(parts))
	seen := map[int64]struct{}{}
	for _, part := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || id <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid ids"})
			return
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if err := h.Service.ReorderGroups(ids); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) bookmarks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	url := strings.TrimSpace(r.FormValue("url"))
	if title == "" || url == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title and url required"})
		return
	}
	var groupID *int64
	if raw := strings.TrimSpace(r.FormValue("group_id")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err == nil {
			groupID = &parsed
		}
	}
	iconPath, err := h.readBookmarkIconUpload(r, url)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := h.Service.CreateBookmarkWithIcon(groupID, title, url, iconPath); err != nil {
		_ = h.Service.CleanupUnusedIcons(h.Config.DataDir)
		if errors.Is(err, services.ErrDuplicateBookmark) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "同一分组中已存在相同名称或网址"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	_ = h.Service.CleanupUnusedIcons(h.Config.DataDir)
	writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
}

func (h *Handler) bookmarkByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/bookmarks/")
	if path == "reorder" {
		h.reorderBookmarks(w, r)
		return
	}
	if strings.HasSuffix(path, "/refresh-icon") {
		id, err := parseID(r.URL.Path, "/api/bookmarks/")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if err := h.Service.RefreshBookmarkIcon(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		_ = h.Service.CleanupUnusedIcons(h.Config.DataDir)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	id, err := parseID(r.URL.Path, "/api/bookmarks/")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	switch r.Method {
	case http.MethodPut:
		title := strings.TrimSpace(r.FormValue("title"))
		url := strings.TrimSpace(r.FormValue("url"))
		var groupID *int64
		if raw := strings.TrimSpace(r.FormValue("group_id")); raw != "" {
			parsed, err := strconv.ParseInt(raw, 10, 64)
			if err == nil {
				groupID = &parsed
			}
		}
		iconPath, err := h.readBookmarkIconUpload(r, url)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := h.Service.UpdateBookmarkWithIcon(id, groupID, title, url, iconPath); err != nil {
			_ = h.Service.CleanupUnusedIcons(h.Config.DataDir)
			if errors.Is(err, services.ErrDuplicateBookmark) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "同一分组中已存在相同名称或网址"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		_ = h.Service.CleanupUnusedIcons(h.Config.DataDir)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case http.MethodDelete:
		if err := h.Service.DeleteBookmark(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		_ = h.Service.CleanupUnusedIcons(h.Config.DataDir)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *Handler) reorderBookmarks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	rawIDs := strings.TrimSpace(r.FormValue("ids"))
	if rawIDs == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ids required"})
		return
	}
	parts := strings.Split(rawIDs, ",")
	ids := make([]int64, 0, len(parts))
	seen := map[int64]struct{}{}
	for _, part := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || id <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid ids"})
			return
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if err := h.Service.ReorderBookmarks(ids); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) readBookmarkIconUpload(r *http.Request, rawURL string) (string, error) {
	file, header, err := r.FormFile("icon")
	if err == http.ErrMissingFile {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, 1024*1024+1))
	if err != nil {
		return "", err
	}
	return services.SaveUploadedBookmarkIcon(rawURL, header.Filename, content, h.Config.DataDir)
}

func (h *Handler) searchEngines(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	template := strings.TrimSpace(r.FormValue("url_template"))
	if name == "" || template == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and url_template required"})
		return
	}
	if err := h.Service.CreateSearchEngine(name, template); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
}

func (h *Handler) searchEngineByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/search-engines/")
	id, err := parseID(r.URL.Path, "/api/search-engines/")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if strings.HasSuffix(path, "/activate") {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if err := h.Service.ActivateSearchEngine(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	switch r.Method {
	case http.MethodPut:
		name := strings.TrimSpace(r.FormValue("name"))
		template := strings.TrimSpace(r.FormValue("url_template"))
		if name == "" || template == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and url_template required"})
			return
		}
		if err := h.Service.UpdateSearchEngine(id, name, template); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case http.MethodDelete:
		if err := h.Service.DeleteSearchEngine(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}
