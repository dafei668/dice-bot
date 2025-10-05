package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	BotToken    string  `json:"bot_token"`
	DatabaseURL string  `json:"database_url"`
	Port        string  `json:"port"`
	FeeRate     float64 `json:"fee_rate"`
	MinBet      int64   `json:"min_bet"`
	MaxBet      int64   `json:"max_bet"`

	// HTTPS配置
	Domain       string `json:"domain"`
	EnableHTTPS  bool   `json:"enable_https"`
	HTTPSPort    string `json:"https_port"`
	CertCacheDir string `json:"cert_cache_dir"`
	AdminEmail   string `json:"admin_email"`

	// 管理员配置
	AdminIDs []int64 `json:"admin_ids"`
}

func Load() (*Config, error) {
	cfg := &Config{
		BotToken:    getEnv("BOT_TOKEN", ""),
		DatabaseURL: getEnv("DATABASE_URL", "dice_bot.db"),
		Port:        getEnv("PORT", "8080"),
		FeeRate:     getEnvFloat("FEE_RATE", 0.1), // 默认10%
		MinBet:      getEnvInt("MIN_BET", 1),
		MaxBet:      getEnvInt("MAX_BET", 100),

		// HTTPS配置
		Domain:       getEnv("DOMAIN", ""),
		EnableHTTPS:  getEnvBool("ENABLE_HTTPS", false),
		HTTPSPort:    getEnv("HTTPS_PORT", "443"),
		CertCacheDir: getEnv("CERT_CACHE_DIR", "./certs"),
		AdminEmail:   getEnv("ADMIN_EMAIL", ""),

		// 管理员配置
		AdminIDs: getEnvInt64Slice("ADMIN_IDS", []int64{}),
	}

	if cfg.BotToken == "" {
		// 如果没有设置环境变量，使用默认测试token（需要用户替换）
		cfg.BotToken = "YOUR_BOT_TOKEN_HERE"
	}

	// 设置默认管理员ID（如果没有配置）
	if len(cfg.AdminIDs) == 0 {
		// 默认管理员ID，建议通过环境变量配置
		cfg.AdminIDs = []int64{123456789} // 替换为你的Telegram用户ID
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.ParseInt(value, 10, 64); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}

func getEnvInt64Slice(key string, defaultValue []int64) []int64 {
	if value := os.Getenv(key); value != "" {
		// 格式: "123,456,789"
		parts := strings.Split(value, ",")
		var result []int64

		for _, part := range parts {
			if i, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil {
				result = append(result, i)
			}
		}

		if len(result) > 0 {
			return result
		}
	}
	return defaultValue
}
