package validator

import (
	"fmt"
	"sync"
	"time"

	"telegram-dice-bot/internal/database"
)

// BalanceValidator 余额验证器，提供实时资金校验保护
type BalanceValidator struct {
	db    *database.DB
	mutex sync.RWMutex
	// 用户操作频率限制
	userOperations map[int64]time.Time
	// 操作间隔限制（防止频繁操作）
	operationInterval time.Duration
}

// NewBalanceValidator 创建新的余额验证器
func NewBalanceValidator(db *database.DB) *BalanceValidator {
	return &BalanceValidator{
		db:                db,
		userOperations:    make(map[int64]time.Time),
		operationInterval: 1 * time.Second, // 1秒间隔限制
	}
}

// ValidateUserBalance 验证用户余额是否足够进行指定金额的操作
func (v *BalanceValidator) ValidateUserBalance(userID int64, requiredAmount int64) error {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	// 检查操作频率限制
	if lastOp, exists := v.userOperations[userID]; exists {
		if time.Since(lastOp) < v.operationInterval {
			return fmt.Errorf("操作过于频繁，请稍后再试")
		}
	}

	// 获取用户当前余额
	user, err := v.db.GetUser(userID)
	if err != nil {
		return fmt.Errorf("获取用户信息失败: %v", err)
	}

	if user == nil {
		return fmt.Errorf("用户不存在")
	}

	// 验证余额
	if user.Balance < requiredAmount {
		return fmt.Errorf("余额不足，请存款后再试。当前余额: %d，需要: %d", user.Balance, requiredAmount)
	}

	// 二次验证：确保扣除后不会为负数
	if user.Balance-requiredAmount < 0 {
		return fmt.Errorf("余额不足，请存款后再试")
	}

	// 记录操作时间
	v.userOperations[userID] = time.Now()

	return nil
}

// ValidateGameOperation 验证游戏操作的余额要求
func (v *BalanceValidator) ValidateGameOperation(userID int64, gameID string, operationType string) error {
	// 获取游戏信息
	game, err := v.db.GetGame(gameID)
	if err != nil {
		return fmt.Errorf("获取游戏信息失败: %v", err)
	}

	if game == nil {
		return fmt.Errorf("游戏不存在")
	}

	// 根据操作类型验证余额
	switch operationType {
	case "create", "join":
		return v.ValidateUserBalance(userID, game.BetAmount)
	default:
		return fmt.Errorf("未知的游戏操作类型: %s", operationType)
	}
}

// ValidateTransactionSafety 验证交易安全性
func (v *BalanceValidator) ValidateTransactionSafety(userID int64, amount int64, transactionType string) error {
	// 对于扣款操作，进行严格验证
	if amount < 0 {
		return v.ValidateUserBalance(userID, -amount)
	}

	// 对于加款操作，检查是否合理
	if amount > 0 {
		// 防止异常大额加款
		maxSingleCredit := int64(100000) // 最大单次加款限制
		if amount > maxSingleCredit {
			return fmt.Errorf("单次加款金额过大: %d，最大允许: %d", amount, maxSingleCredit)
		}
	}

	return nil
}

// CleanupOldOperations 清理过期的操作记录
func (v *BalanceValidator) CleanupOldOperations() {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	cutoff := time.Now().Add(-10 * time.Minute) // 清理10分钟前的记录
	for userID, lastOp := range v.userOperations {
		if lastOp.Before(cutoff) {
			delete(v.userOperations, userID)
		}
	}
}

// StartCleanupRoutine 启动清理协程
func (v *BalanceValidator) StartCleanupRoutine() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute) // 每5分钟清理一次
		defer ticker.Stop()

		for range ticker.C {
			v.CleanupOldOperations()
		}
	}()
}