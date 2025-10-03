package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"telegram-dice-bot/internal/bot"
	"telegram-dice-bot/internal/config"
	"telegram-dice-bot/internal/database"
	"telegram-dice-bot/internal/game"
)

func main() {
	// 加载.env文件
	if err := godotenv.Load(); err != nil {
		log.Printf("警告: 无法加载.env文件: %v", err)
	}

	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("加载配置失败:", err)
	}

	// 初始化数据库
	db, err := database.Init(cfg.DatabaseURL)
	if err != nil {
		log.Fatal("初始化数据库失败:", err)
	}
	defer db.Close()

	// 创建游戏管理器
	gameManager := game.NewManager(db)

	// 创建机器人实例
	telegramBot, err := bot.NewBot(cfg, db, gameManager)
	if err != nil {
		log.Fatal("创建机器人失败:", err)
	}

	// 设置游戏超时回调
	gameManager.SetGameExpiredCallback(telegramBot.OnGameExpired)

	// 启动机器人
	go func() {
		if err := telegramBot.Start(); err != nil {
			log.Fatal("启动机器人失败:", err)
		}
	}()

	log.Printf("🎲 Telegram骰子机器人已启动")
	log.Printf("📊 配置信息:")
	log.Printf("   - 端口: %s", cfg.Port)
	log.Printf("   - 数据库: %s", cfg.DatabaseURL)
	log.Printf("   - 手续费率: %.1f%%", cfg.CommissionRate*100)
	log.Printf("   - 下注范围: %d - %d", cfg.MinBet, cfg.MaxBet)

	// 等待中断信号
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	log.Printf("🛑 正在关闭机器人...")
	telegramBot.Stop()
	log.Printf("✅ 机器人已关闭")
}
