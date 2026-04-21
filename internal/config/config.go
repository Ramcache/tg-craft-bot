package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv             string
	BotToken           string
	DatabaseURL        string
	LogLevel           string
	BotPollTimeout     int
	WorkerPollInterval time.Duration
	Timezone           string
	AdminTelegramIDs   map[int64]struct{}
}

func Load() (Config, error) {
	_ = godotenv.Load()

	cfg := Config{
		AppEnv:         getEnv("APP_ENV", "local"),
		BotToken:       getEnv("BOT_TOKEN", ""),
		DatabaseURL:    getEnv("DATABASE_URL", ""),
		LogLevel:       getEnv("LOG_LEVEL", "info"),
		BotPollTimeout: getEnvInt("BOT_POLL_TIMEOUT", 30),
		Timezone:       getEnv("TIMEZONE", "Europe/Amsterdam"),
	}

	if cfg.BotToken == "" {
		return Config{}, fmt.Errorf("BOT_TOKEN is required")
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	dur, err := time.ParseDuration(getEnv("WORKER_POLL_INTERVAL", "15s"))
	if err != nil {
		return Config{}, fmt.Errorf("parse WORKER_POLL_INTERVAL: %w", err)
	}
	cfg.WorkerPollInterval = dur

	cfg.AdminTelegramIDs = parseAdminIDs(getEnv("ADMIN_TELEGRAM_IDS", ""))

	return cfg, nil
}

func getEnv(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func getEnvInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func parseAdminIDs(raw string) map[int64]struct{} {
	out := make(map[int64]struct{})
	if strings.TrimSpace(raw) == "" {
		return out
	}

	parts := strings.Split(raw, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		id, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			continue
		}
		out[id] = struct{}{}
	}
	return out
}
