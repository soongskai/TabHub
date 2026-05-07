package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppName       string
	Port          string
	AdminUsername string
	AdminPassword string
	SessionSecret string
	SessionDays   int
	DBPath        string
	DataDir       string
	TZ            string
	SearchEngine  string
}

func Load() Config {
	fileValues := loadConfigFile(env("CONFIG_FILE", "config.env"))
	cfg := Config{
		AppName:       configValue(fileValues, "APP_NAME", "TabHub"),
		Port:          configValue(fileValues, "PORT", "8080"),
		AdminUsername: configValue(fileValues, "ADMIN_USERNAME", "admin"),
		AdminPassword: configValue(fileValues, "ADMIN_PASSWORD", "admin"),
		SessionSecret: configValue(fileValues, "SESSION_SECRET", "please-change-this-secret"),
		SessionDays:   configInt(fileValues, "SESSION_DAYS", 60),
		DBPath:        configValue(fileValues, "DB_PATH", "./data/app.db"),
		DataDir:       configValue(fileValues, "DATA_DIR", "./data"),
		TZ:            configValue(fileValues, "TZ", "Asia/Shanghai"),
		SearchEngine:  configValue(fileValues, "DEFAULT_SEARCH_ENGINE", "https://www.baidu.com/s?wd=%s"),
	}
	if cfg.SessionDays < 1 {
		cfg.SessionDays = 60
	}
	return cfg
}

func (c Config) SessionDuration() time.Duration {
	return time.Duration(c.SessionDays) * 24 * time.Hour
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func configValue(fileValues map[string]string, key, fallback string) string {
	if value, ok := fileValues[key]; ok && value != "" {
		fallback = value
	}
	return env(key, fallback)
}

func configInt(fileValues map[string]string, key string, fallback int) int {
	if value, ok := fileValues[key]; ok && value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			fallback = parsed
		}
	}
	return envInt(key, fallback)
}

func loadConfigFile(path string) map[string]string {
	values := map[string]string{}
	if strings.TrimSpace(path) == "" {
		return values
	}
	file, err := os.Open(path)
	if err != nil {
		return values
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key != "" {
			values[key] = value
		}
	}
	return values
}
