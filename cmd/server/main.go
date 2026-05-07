package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"tabhub/internal/config"
	"tabhub/internal/db"
	"tabhub/internal/handlers"
	"tabhub/internal/services"
	"time"
)

func main() {
	cfg := config.Load()
	for _, dir := range []string{
		cfg.DataDir,
		filepath.Join(cfg.DataDir, "uploads"),
		filepath.Join(cfg.DataDir, "uploads", "wallpapers"),
		filepath.Join(cfg.DataDir, "uploads", "icons"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatalf("create dir %s: %v", dir, err)
		}
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer database.Close()

	if err := db.Migrate(database); err != nil {
		log.Fatalf("migrate db: %v", err)
	}

	service := services.NewAppService(database)
	if err := service.EnsureAdmin(cfg.AdminUsername, cfg.AdminPassword); err != nil {
		log.Fatalf("ensure admin: %v", err)
	}
	if err := service.EnsureDefaults(cfg.SearchEngine); err != nil {
		log.Fatalf("ensure defaults: %v", err)
	}
	if err := service.RefreshMissingGroupIcons(); err != nil {
		log.Printf("refresh group icons: %v", err)
	}

	tmpl := template.Must(template.ParseGlob(filepath.Join("web", "templates", "*.html")))
	handler := handlers.New(cfg, service, tmpl)
	mux := http.NewServeMux()
	handler.Register(mux, filepath.Join("web", "static"), cfg.DataDir)

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           loggingMiddleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	log.Printf("%s listening on :%s", cfg.AppName, cfg.Port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		if recorder.status >= http.StatusBadRequest {
			log.Printf("%s %s %d", r.Method, r.URL.Path, recorder.status)
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
