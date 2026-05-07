package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

func Open(dbPath string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	database.SetConnMaxLifetime(10 * time.Minute)

	if _, err := database.Exec(`PRAGMA journal_mode = WAL; PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000;`); err != nil {
		return nil, fmt.Errorf("apply pragmas: %w", err)
	}

	return database, nil
}

func Migrate(database *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			icon_name TEXT NOT NULL DEFAULT '',
			icon_svg TEXT NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL DEFAULT 0,
			is_system_default INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS bookmarks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			group_id INTEGER,
			title TEXT NOT NULL,
			url TEXT NOT NULL,
			icon_path TEXT NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			FOREIGN KEY(group_id) REFERENCES groups(id) ON DELETE SET NULL
		);`,
		`CREATE TABLE IF NOT EXISTS wallpapers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			filename TEXT NOT NULL,
			path TEXT NOT NULL,
			is_active INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS search_engines (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			url_template TEXT NOT NULL UNIQUE,
			icon_path TEXT NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);`,
	}

	for _, query := range queries {
		if _, err := database.Exec(query); err != nil {
			return err
		}
	}

	if _, err := database.Exec(`ALTER TABLE groups ADD COLUMN is_system_default INTEGER NOT NULL DEFAULT 0`); err != nil {
		if err.Error() != "SQL logic error: duplicate column name: is_system_default (1)" {
			if err.Error() != "duplicate column name: is_system_default" {
				return err
			}
		}
	}
	for _, query := range []string{
		`ALTER TABLE groups ADD COLUMN icon_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE groups ADD COLUMN icon_svg TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := database.Exec(query); err != nil {
			if err.Error() != "SQL logic error: duplicate column name: icon_name (1)" &&
				err.Error() != "SQL logic error: duplicate column name: icon_svg (1)" &&
				err.Error() != "duplicate column name: icon_name" &&
				err.Error() != "duplicate column name: icon_svg" {
				return err
			}
		}
	}

	return nil
}
