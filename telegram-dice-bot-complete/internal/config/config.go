package config

import (
	"os"
	"strconv"
)

type Config struct {
	BotToken       string
	DatabaseURL    string
	Port           string
	CommissionRate float64 // 平台抽水比例，默认10%
	MinBet         int64   // 最小下注金额
	MaxBet         int64   // 最大下注金额
}

func Load() (*Config, error) {
	cfg := &Config{
		BotToken:       getEnv("BOT_TOKEN", ""),
		DatabaseURL:    getEnv("DATABASE_URL", "dice_bot.db"),
		Port:           getEnv("PORT", "8080"),
		CommissionRate: getEnvFloat("COMMISSION_RATE", 0.1), // 默认10%
		MinBet:         getEnvInt("MIN_BET", 1),
		MaxBet:         getEnvInt("MAX_BET", 10000),
	}

	if cfg.BotToken == "" {
		// 如果没有设置环境变量，使用默认测试token（需要用户替换）
		cfg.BotToken = "YOUR_BOT_TOKEN_HERE"
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
