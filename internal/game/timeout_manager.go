package game

import (
	"fmt"
	"sync"
	"time"

	"telegram-dice-bot/internal/database"
	"telegram-dice-bot/internal/logger"
	"telegram-dice-bot/internal/models"
	"telegram-dice-bot/internal/security"
	"telegram-dice-bot/internal/utils"
)

// TimeoutManager 游戏超时管理器
type TimeoutManager struct {
	db            *database.DB
	security      *security.SecurityManager
	logger        *logger.Logger
	gameTimers    map[string]*GameTimer
	timerMutex    sync.RWMutex
	onGameExpired func(gameID string, chatID int64)
	cleanupTicker *time.Ticker
	stopCleanup   chan bool
}

// GameTimer 游戏定时器信息
type GameTimer struct {
	GameID    string
	Timer     *time.Timer
	ChatID    int64
	CreatedAt time.Time
	ExpiresAt time.Time
}

// NewTimeoutManager 创建超时管理器
func NewTimeoutManager(db *database.DB, security *security.SecurityManager, logger *logger.Logger) *TimeoutManager {
	tm := &TimeoutManager{
		db:          db,
		security:    security,
		logger:      logger,
		gameTimers:  make(map[string]*GameTimer),
		stopCleanup: make(chan bool),
	}

	// 启动定期清理任务
	tm.startCleanupTask()

	return tm
}

// SetGameExpiredCallback 设置游戏超时回调函数
func (tm *TimeoutManager) SetGameExpiredCallback(callback func(gameID string, chatID int64)) {
	tm.onGameExpired = callback
}

// SetGameTimeout 设置游戏超时定时器
func (tm *TimeoutManager) SetGameTimeout(gameID string, chatID int64, duration time.Duration) {
	tm.timerMutex.Lock()
	defer tm.timerMutex.Unlock()

	// 如果已存在定时器，先取消
	if existingTimer, exists := tm.gameTimers[gameID]; exists {
		existingTimer.Timer.Stop()
		delete(tm.gameTimers, gameID)
	}

	// 创建新的定时器
	timer := time.AfterFunc(duration, func() {
		tm.handleGameTimeout(gameID)
	})

	gameTimer := &GameTimer{
		GameID:    gameID,
		Timer:     timer,
		ChatID:    chatID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(duration),
	}

	tm.gameTimers[gameID] = gameTimer
	tm.logger.Info("设置游戏超时定时器: 游戏ID=%s, 超时时间=%v", gameID, duration)
}

// CancelGameTimeout 取消游戏超时定时器
func (tm *TimeoutManager) CancelGameTimeout(gameID string) {
	tm.timerMutex.Lock()
	defer tm.timerMutex.Unlock()

	if gameTimer, exists := tm.gameTimers[gameID]; exists {
		gameTimer.Timer.Stop()
		delete(tm.gameTimers, gameID)
		tm.logger.Info("取消游戏超时定时器: 游戏ID=%s", gameID)
	}
}

// handleGameTimeout 处理游戏超时
func (tm *TimeoutManager) handleGameTimeout(gameID string) {
	tm.logger.Info("处理游戏超时: 游戏ID=%s", gameID)

	// 获取游戏信息
	game, err := tm.db.GetGame(gameID)
	if err != nil {
		tm.logger.Error("获取游戏信息失败: %v", err)
		return
	}

	if game == nil {
		tm.logger.Error("游戏不存在: %s", gameID)
		return
	}

	// 检查游戏状态
	if game.Status != models.GameStatusWaiting {
		tm.logger.Info("游戏状态已变更，无需超时处理: 游戏ID=%s, 状态=%s", gameID, game.Status)
		tm.CancelGameTimeout(gameID)
		return
	}

	// 执行安全的超时退款
	if err := tm.expireGameSecure(game); err != nil {
		tm.logger.Error("游戏超时处理失败: 游戏ID=%s, 错误=%v", gameID, err)
		return
	}

	// 发送超时通知
	if tm.onGameExpired != nil {
		tm.onGameExpired(gameID, game.ChatID)
	}

	// 清理定时器
	tm.CancelGameTimeout(gameID)
}

// expireGameSecure 安全的游戏超时处理
func (tm *TimeoutManager) expireGameSecure(game *models.Game) error {
	// 获取玩家1信息
	player1, err := tm.db.GetUser(game.Player1ID)
	if err != nil {
		return fmt.Errorf("获取玩家1信息失败: %v", err)
	}

	if player1 == nil {
		return fmt.Errorf("玩家1不存在")
	}

	// 计算退款金额和新余额
	refundAmount := game.BetAmount
	newBalance := player1.Balance + refundAmount

	// 记录安全操作
	securityOp := tm.security.RecordOperation(
		game.Player1ID,
		&game.ID,
		"refund",
		refundAmount,
		player1.Balance,
		newBalance,
		map[string]interface{}{
			"operation": "timeout_refund",
			"reason":    "游戏超时自动退款",
			"game_id":   game.ID,
		},
	)

	// 验证安全操作
	if err := tm.security.ValidateOperation(securityOp.ID); err != nil {
		tm.security.FailOperation(securityOp.ID, fmt.Sprintf("安全验证失败: %v", err))
		return fmt.Errorf("安全验证失败: %v", err)
	}

	// 创建退款交易记录
	tx := &models.Transaction{
		ID:          utils.GenerateTransactionID(),
		UserID:      game.Player1ID,
		GameID:      &game.ID,
		Type:        models.TransactionTypeRefund,
		Amount:      refundAmount,
		Balance:     newBalance,
		Description: "游戏超时自动退款",
	}

	// 使用事务执行超时退款
	if err := tm.db.ExpireGameWithTransaction(game.ID, game.Player1ID, newBalance, tx); err != nil {
		tm.security.RollbackOperation(securityOp.ID, fmt.Sprintf("超时退款失败: %v", err))
		return fmt.Errorf("超时退款失败: %v", err)
	}

	// 标记安全操作完成
	tm.security.CompleteOperation(securityOp.ID)

	tm.logger.Info("游戏超时处理成功: 游戏ID=%s, 退款金额=%d", game.ID, refundAmount)
	return nil
}

// startCleanupTask 启动定期清理任务
func (tm *TimeoutManager) startCleanupTask() {
	tm.cleanupTicker = time.NewTicker(5 * time.Minute) // 每5分钟清理一次

	go func() {
		for {
			select {
			case <-tm.cleanupTicker.C:
				tm.cleanupExpiredTimers()
			case <-tm.stopCleanup:
				tm.cleanupTicker.Stop()
				return
			}
		}
	}()
}

// cleanupExpiredTimers 清理过期的定时器
func (tm *TimeoutManager) cleanupExpiredTimers() {
	tm.timerMutex.Lock()
	defer tm.timerMutex.Unlock()

	now := time.Now()
	expiredGameIDs := make([]string, 0)

	for gameID, gameTimer := range tm.gameTimers {
		// 检查定时器是否已过期（超过预期过期时间1分钟）
		if now.After(gameTimer.ExpiresAt.Add(1 * time.Minute)) {
			expiredGameIDs = append(expiredGameIDs, gameID)
		}
	}

	// 清理过期的定时器
	for _, gameID := range expiredGameIDs {
		if gameTimer, exists := tm.gameTimers[gameID]; exists {
			gameTimer.Timer.Stop()
			delete(tm.gameTimers, gameID)
			tm.logger.Info("清理过期定时器: 游戏ID=%s", gameID)
		}
	}

	if len(expiredGameIDs) > 0 {
		tm.logger.Info("定期清理完成，清理了 %d 个过期定时器", len(expiredGameIDs))
	}
}

// GetActiveTimers 获取活跃的定时器信息
func (tm *TimeoutManager) GetActiveTimers() map[string]*GameTimer {
	tm.timerMutex.RLock()
	defer tm.timerMutex.RUnlock()

	result := make(map[string]*GameTimer)
	for gameID, gameTimer := range tm.gameTimers {
		result[gameID] = &GameTimer{
			GameID:    gameTimer.GameID,
			ChatID:    gameTimer.ChatID,
			CreatedAt: gameTimer.CreatedAt,
			ExpiresAt: gameTimer.ExpiresAt,
		}
	}

	return result
}

// Stop 停止超时管理器
func (tm *TimeoutManager) Stop() {
	// 停止清理任务
	close(tm.stopCleanup)

	// 取消所有定时器
	tm.timerMutex.Lock()
	defer tm.timerMutex.Unlock()

	for gameID, gameTimer := range tm.gameTimers {
		gameTimer.Timer.Stop()
		delete(tm.gameTimers, gameID)
	}

	tm.logger.Info("超时管理器已停止")
}

// GetTimeoutStats 获取超时统计信息
func (tm *TimeoutManager) GetTimeoutStats() map[string]interface{} {
	tm.timerMutex.RLock()
	defer tm.timerMutex.RUnlock()

	stats := map[string]interface{}{
		"active_timers": len(tm.gameTimers),
		"timers":        make([]map[string]interface{}, 0),
	}

	for _, gameTimer := range tm.gameTimers {
		timerInfo := map[string]interface{}{
			"game_id":    gameTimer.GameID,
			"chat_id":    gameTimer.ChatID,
			"created_at": gameTimer.CreatedAt,
			"expires_at": gameTimer.ExpiresAt,
			"remaining":  time.Until(gameTimer.ExpiresAt).Seconds(),
		}
		stats["timers"] = append(stats["timers"].([]map[string]interface{}), timerInfo)
	}

	return stats
}
