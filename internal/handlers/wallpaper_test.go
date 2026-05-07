package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"tabhub/internal/config"
	"tabhub/internal/db"
	"tabhub/internal/services"
	"testing"
)

func newWallpaperTestHandler(t *testing.T) *Handler {
	t.Helper()
	dataDir := t.TempDir()
	database, err := db.Open(filepath.Join(dataDir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatal(err)
	}
	return &Handler{
		Config:    config.Config{DataDir: dataDir},
		Service:   services.NewAppService(database),
		StaticDir: filepath.Join("..", "..", "web", "static"),
	}
}

func TestCurrentWallpaperServesActiveWallpaper(t *testing.T) {
	handler := newWallpaperTestHandler(t)
	wallpaperDir := filepath.Join(handler.Config.DataDir, "uploads", "wallpapers")
	if err := os.MkdirAll(wallpaperDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wallpaperDir, "active.txt"), []byte("active wallpaper"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := handler.Service.DB.Exec(`INSERT INTO wallpapers(filename, path, is_active, created_at) VALUES('active.txt', 'uploads/wallpapers/active.txt', 1, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/wallpaper/current", nil)
	handler.currentWallpaper(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if body := recorder.Body.String(); body != "active wallpaper" {
		t.Fatalf("body = %q, want active wallpaper", body)
	}
}

func TestCurrentWallpaperFallsBackToDefault(t *testing.T) {
	handler := newWallpaperTestHandler(t)
	if _, err := handler.Service.DB.Exec(`INSERT INTO wallpapers(filename, path, is_active, created_at) VALUES('missing.jpg', 'uploads/wallpapers/missing.jpg', 1, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/wallpaper/current", nil)
	handler.currentWallpaper(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if body := recorder.Body.Bytes(); !strings.HasPrefix(recorder.Header().Get("Content-Type"), "image/jpeg") || len(body) == 0 {
		t.Fatalf("expected default jpg fallback, content-type=%q len=%d", recorder.Header().Get("Content-Type"), len(body))
	}
}
