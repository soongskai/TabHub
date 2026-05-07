package handlers

import (
	"encoding/json"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"tabhub/internal/auth"
	"tabhub/internal/config"
	"tabhub/internal/services"
	"time"
)

type Handler struct {
	Config    config.Config
	Sessions  *auth.SessionManager
	Service   *services.AppService
	Tmpl      *template.Template
	StaticDir string
}

type HomeViewData struct {
	AppName             string
	Groups              []services.Group
	Bookmarks           []services.Bookmark
	SearchEngines       []services.SearchEngine
	Wallpapers          []services.Wallpaper
	Settings            map[string]string
	WallpaperURL        string
	OverlayOpacity      string
	Theme               string
	ShowTime            bool
	ShowDate            bool
	DefaultSearchEngine string
	Now                 time.Time
	RequireLogin        bool
	LoginError          string
}

func New(cfg config.Config, service *services.AppService, tmpl *template.Template) *Handler {
	return &Handler{
		Config:   cfg,
		Sessions: auth.NewSessionManager(cfg.SessionSecret),
		Service:  service,
		Tmpl:     tmpl,
	}
}

func (h *Handler) Register(mux *http.ServeMux, staticDir, dataDir string) {
	h.StaticDir = staticDir
	mux.HandleFunc("/favicon.ico", h.favicon)
	mux.HandleFunc("/wallpaper/current", h.currentWallpaper)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))
	mux.Handle("/uploads/", h.requireAuth(http.StripPrefix("/uploads/", http.FileServer(http.Dir(filepath.Join(dataDir, "uploads")))).ServeHTTP))
	mux.HandleFunc("/", h.requireAuth(h.home))
	mux.HandleFunc("/login", h.login)
	mux.HandleFunc("/logout", h.logout)
	mux.HandleFunc("/api/home", h.requireAuth(h.apiHome))
	mux.HandleFunc("/api/groups", h.requireAuth(h.groups))
	mux.HandleFunc("/api/groups/", h.requireAuth(h.groupByID))
	mux.HandleFunc("/api/bookmarks", h.requireAuth(h.bookmarks))
	mux.HandleFunc("/api/bookmarks/", h.requireAuth(h.bookmarkByID))
	mux.HandleFunc("/api/search-engines", h.requireAuth(h.searchEngines))
	mux.HandleFunc("/api/search-engines/", h.requireAuth(h.searchEngineByID))
	mux.HandleFunc("/api/wallpapers", h.requireAuth(h.wallpapers))
	mux.HandleFunc("/api/wallpapers/", h.requireAuth(h.wallpaperByID))
	mux.HandleFunc("/api/settings", h.requireAuth(h.settings))
}

func (h *Handler) favicon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "image/x-icon")
	http.ServeFile(w, r, "favicon.ico")
}

func (h *Handler) currentWallpaper(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	wallpaper, err := h.Service.GetActiveWallpaper()
	if err == nil && wallpaper != nil && wallpaper.Path != "" {
		relPath := strings.TrimPrefix(filepath.ToSlash(wallpaper.Path), "/")
		fullPath := filepath.Join(h.Config.DataDir, filepath.FromSlash(relPath))
		if cleanDataDir, err := filepath.Abs(h.Config.DataDir); err == nil {
			if cleanFullPath, err := filepath.Abs(fullPath); err == nil && strings.HasPrefix(cleanFullPath, cleanDataDir+string(filepath.Separator)) {
				if info, err := os.Stat(cleanFullPath); err == nil && !info.IsDir() {
					http.ServeFile(w, r, cleanFullPath)
					return
				}
			}
		}
	}
	staticDir := h.StaticDir
	if staticDir == "" {
		staticDir = filepath.Join("web", "static")
	}
	http.ServeFile(w, r, filepath.Join(staticDir, "default-wallpaper.jpg"))
}

func (h *Handler) home(w http.ResponseWriter, r *http.Request) {
	groups, bookmarks, searchEngines, settings, wallpaper, err := h.Service.GetHomeData()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wallpapers, err := h.Service.ListWallpapers()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wallpaperURL := "/static/default-wallpaper.jpg"
	if wallpaper != nil && wallpaper.Path != "" {
		wallpaperURL = "/" + strings.TrimPrefix(wallpaper.Path, "/")
	}
	overlayOpacity := settings["overlay_opacity"]
	if overlayOpacity == "" {
		overlayOpacity = "0.32"
	}
	theme := settings["theme"]
	if theme != "light" {
		theme = "dark"
	}
	showTime := settings["show_time"] != "false"
	showDate := settings["show_date"] != "false"
	_, requireLogin := r.URL.Query()["login"]
	data := HomeViewData{
		AppName:             h.Config.AppName,
		Groups:              groups,
		Bookmarks:           bookmarks,
		SearchEngines:       searchEngines,
		Wallpapers:          wallpapers,
		Settings:            settings,
		WallpaperURL:        wallpaperURL,
		OverlayOpacity:      overlayOpacity,
		Theme:               theme,
		ShowTime:            showTime,
		ShowDate:            showDate,
		DefaultSearchEngine: h.Config.SearchEngine,
		Now:                 time.Now(),
		RequireLogin:        requireLogin,
		LoginError:          r.URL.Query().Get("error"),
	}
	if err := h.Tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		data := map[string]string{
			"AppName": h.Config.AppName,
			"Error":   r.URL.Query().Get("error"),
		}
		if err := h.Tmpl.ExecuteTemplate(w, "login.html", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	user, err := h.Service.Authenticate(r.FormValue("username"), r.FormValue("password"))
	if err != nil {
		http.Redirect(w, r, "/login?error=用户名或密码错误", http.StatusSeeOther)
		return
	}
	expiresAt := time.Now().Add(h.Config.SessionDuration())
	token := h.Sessions.Sign(user.ID, expiresAt)
	http.SetCookie(w, &http.Cookie{Name: "session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: expiresAt, MaxAge: int(h.Config.SessionDuration().Seconds())})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "session", Value: "", Path: "/", HttpOnly: true, MaxAge: -1, Expires: time.Unix(0, 0)})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *Handler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err != nil {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "请先登录"})
				return
			}
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if _, _, ok := h.Sessions.Parse(cookie.Value); !ok {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "", Path: "/", HttpOnly: true, MaxAge: -1, Expires: time.Unix(0, 0)})
			if strings.HasPrefix(r.URL.Path, "/api/") {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "请先登录"})
				return
			}
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func parseID(path, prefix string) (int64, error) {
	value := strings.TrimPrefix(path, prefix)
	parts := strings.Split(strings.Trim(value, "/"), "/")
	return strconv.ParseInt(parts[0], 10, 64)
}

func readLimitedFile(file io.Reader, limit int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(file, limit))
}
