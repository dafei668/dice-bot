package test

import (
	"telegram-dice-bot/internal/bot"
	"telegram-dice-bot/internal/config"
	"telegram-dice-bot/internal/database"
	"telegram-dice-bot/internal/game"
	"telegram-dice-bot/internal/models"
	"testing"
)

func TestBotCreation(t *testing.T) {
	// 跳过需要真实Bot Token的测试
	t.Skip("跳过需要真实Bot Token的测试")

	// 创建测试配置
	cfg := &config.Config{
		BotToken:    "test_token",
		DatabaseURL: ":memory:",
		Port:        "8080",
		// CommissionRate 字段不存在，已移除
		MinBet: 1,
		MaxBet: 1000,
	}

	// 创建内存数据库
	db, err := database.Init(cfg.DatabaseURL)
	if err != nil {
		t.Fatalf("创建数据库失败: %v", err)
	}
	defer db.Close()

	// 创建游戏管理器
	gameManager := game.NewManager(db, 0.05)

	// 创建机器人实例
	_, err = bot.NewBot(cfg, db, gameManager)
	if err != nil {
		t.Fatalf("创建机器人失败: %v", err)
	}
}

func TestGameManager(t *testing.T) {
	// 创建测试配置
	cfg := &config.Config{
		DatabaseURL: ":memory:",
	}

	// 创建内存数据库
	db, err := database.Init(cfg.DatabaseURL)
	if err != nil {
		t.Fatalf("创建数据库失败: %v", err)
	}
	defer db.Close()

	// 创建游戏管理器
	gameManager := game.NewManager(db, 0.05)

	// 先创建测试用户
	testUser := &models.User{
		ID:        123,
		Username:  "testuser",
		FirstName: "Test",
		Balance:   1000,
	}
	err = db.CreateUser(testUser)
	if err != nil {
		t.Fatalf("创建测试用户失败: %v", err)
	}

	// 测试创建游戏
	gameID, err := gameManager.CreateGame(123, 456, 100)
	if err != nil {
		t.Fatalf("创建游戏失败: %v", err)
	}

	if gameID == "" {
		t.Fatal("游戏ID不能为空")
	}

	// 测试获取等待中的游戏
	games, err := db.GetWaitingGames(456)
	if err != nil {
		t.Fatalf("获取等待游戏失败: %v", err)
	}

	if len(games) != 1 {
		t.Fatalf("期望1个等待游戏，实际得到%d个", len(games))
	}
}

func TestDatabase(t *testing.T) {
	// 创建内存数据库
	db, err := database.Init(":memory:")
	if err != nil {
		t.Fatalf("创建数据库失败: %v", err)
	}
	defer db.Close()

	// 测试用户操作
	userID := int64(123)

	// 创建用户
	testUser := &models.User{
		ID:        userID,
		Username:  "testuser",
		FirstName: "Test",
		Balance:   1000,
	}
	err = db.CreateUser(testUser)
	if err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}

	// 获取用户
	user, err := db.GetUser(userID)
	if err != nil {
		t.Fatalf("获取用户失败: %v", err)
	}

	if user.Balance != 1000 { // 默认余额
		t.Fatalf("期望余额1000，实际得到%d", user.Balance)
	}

	// 更新用户余额
	err = db.UpdateUserBalance(userID, 1500)
	if err != nil {
		t.Fatalf("更新用户余额失败: %v", err)
	}

	// 验证余额更新
	updatedUser, err := db.GetUser(userID)
	if err != nil {
		t.Fatalf("获取更新后用户失败: %v", err)
	}

	if updatedUser.Balance != 1500 {
		t.Fatalf("期望余额1500，实际得到%d", updatedUser.Balance)
	}
}
