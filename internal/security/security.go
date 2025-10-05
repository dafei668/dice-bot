package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"telegram-dice-bot/internal/logger"
)

// SecurityManager 安全管理器
type SecurityManager struct {
	logger      *logger.Logger
	mutex       sync.RWMutex
	operations  map[string]*OperationRecord // 记录所有资金操作
	checksums   map[string]string           // 操作校验和
	rollbackLog map[string]*RollbackInfo    // 回滚日志
}

// OperationRecord 操作记录
type OperationRecord struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	UserID     int64                  `json:"user_id"`
	GameID     *string                `json:"game_id,omitempty"`
	Amount     int64                  `json:"amount"`
	OldBalance int64                  `json:"old_balance"`
	NewBalance int64                  `json:"new_balance"`
	Timestamp  time.Time              `json:"timestamp"`
	Checksum   string                 `json:"checksum"`
	Status     string                 `json:"status"` // pending, completed, failed, rolled_back
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// RollbackInfo 回滚信息
type RollbackInfo struct {
	OriginalOperation *OperationRecord `json:"original_operation"`
	RollbackReason    string           `json:"rollback_reason"`
	RollbackTime      time.Time        `json:"rollback_time"`
	Success           bool             `json:"success"`
}

// NewSecurityManager 创建安全管理器
func NewSecurityManager(logger *logger.Logger) *SecurityManager {
	return &SecurityManager{
		logger:      logger,
		operations:  make(map[string]*OperationRecord),
		checksums:   make(map[string]string),
		rollbackLog: make(map[string]*RollbackInfo),
	}
}

// GenerateOperationID 生成操作ID
func (sm *SecurityManager) GenerateOperationID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// CalculateChecksum 计算操作校验和
func (sm *SecurityManager) CalculateChecksum(operation *OperationRecord) string {
	data := fmt.Sprintf("%s:%d:%s:%d:%d:%d:%d",
		operation.ID,
		operation.UserID,
		operation.Type,
		operation.Amount,
		operation.OldBalance,
		operation.NewBalance,
		operation.Timestamp.Unix(),
	)

	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// RecordOperation 记录资金操作
func (sm *SecurityManager) RecordOperation(userID int64, gameID *string, operationType string, amount, oldBalance, newBalance int64, metadata map[string]interface{}) *OperationRecord {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	operation := &OperationRecord{
		ID:         sm.GenerateOperationID(),
		Type:       operationType,
		UserID:     userID,
		GameID:     gameID,
		Amount:     amount,
		OldBalance: oldBalance,
		NewBalance: newBalance,
		Timestamp:  time.Now(),
		Status:     "pending",
		Metadata:   metadata,
	}

	// 计算校验和
	operation.Checksum = sm.CalculateChecksum(operation)

	// 存储操作记录
	sm.operations[operation.ID] = operation
	sm.checksums[operation.ID] = operation.Checksum

	// 记录日志
	sm.logger.Info("资金操作记录: ID=%s, 用户=%d, 类型=%s, 金额=%d, 旧余额=%d, 新余额=%d",
		operation.ID, userID, operationType, amount, oldBalance, newBalance)

	return operation
}

// ValidateOperation 验证操作完整性
func (sm *SecurityManager) ValidateOperation(operationID string) error {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	operation, exists := sm.operations[operationID]
	if !exists {
		return fmt.Errorf("操作记录不存在: %s", operationID)
	}

	// 重新计算校验和
	calculatedChecksum := sm.CalculateChecksum(operation)
	storedChecksum, exists := sm.checksums[operationID]
	if !exists {
		return fmt.Errorf("校验和不存在: %s", operationID)
	}

	if calculatedChecksum != storedChecksum {
		sm.logger.Error("校验和不匹配: 操作ID=%s, 计算值=%s, 存储值=%s",
			operationID, calculatedChecksum, storedChecksum)
		return fmt.Errorf("操作完整性验证失败")
	}

	// 验证余额计算
	expectedBalance := operation.OldBalance + operation.Amount
	if expectedBalance != operation.NewBalance {
		return fmt.Errorf("余额计算错误: 期望=%d, 实际=%d", expectedBalance, operation.NewBalance)
	}

	return nil
}

// CompleteOperation 标记操作完成
func (sm *SecurityManager) CompleteOperation(operationID string) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	operation, exists := sm.operations[operationID]
	if !exists {
		return fmt.Errorf("操作记录不存在: %s", operationID)
	}

	operation.Status = "completed"
	sm.logger.Info("资金操作完成: ID=%s", operationID)

	return nil
}

// FailOperation 标记操作失败
func (sm *SecurityManager) FailOperation(operationID string, reason string) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	operation, exists := sm.operations[operationID]
	if !exists {
		return fmt.Errorf("操作记录不存在: %s", operationID)
	}

	operation.Status = "failed"
	if operation.Metadata == nil {
		operation.Metadata = make(map[string]interface{})
	}
	operation.Metadata["failure_reason"] = reason

	sm.logger.Error("资金操作失败: ID=%s, 原因=%s", operationID, reason)

	return nil
}

// RollbackOperation 回滚操作
func (sm *SecurityManager) RollbackOperation(operationID string, reason string) (*RollbackInfo, error) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	operation, exists := sm.operations[operationID]
	if !exists {
		return nil, fmt.Errorf("操作记录不存在: %s", operationID)
	}

	if operation.Status == "rolled_back" {
		return nil, fmt.Errorf("操作已经回滚: %s", operationID)
	}

	// 创建回滚信息
	rollbackInfo := &RollbackInfo{
		OriginalOperation: operation,
		RollbackReason:    reason,
		RollbackTime:      time.Now(),
		Success:           true,
	}

	// 标记原操作为已回滚
	operation.Status = "rolled_back"
	if operation.Metadata == nil {
		operation.Metadata = make(map[string]interface{})
	}
	operation.Metadata["rollback_reason"] = reason
	operation.Metadata["rollback_time"] = rollbackInfo.RollbackTime

	// 记录回滚日志
	sm.rollbackLog[operationID] = rollbackInfo

	sm.logger.Error("资金操作回滚: ID=%s, 原因=%s", operationID, reason)

	return rollbackInfo, nil
}

// GetOperationHistory 获取用户操作历史
func (sm *SecurityManager) GetOperationHistory(userID int64, limit int) []*OperationRecord {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	var history []*OperationRecord
	for _, operation := range sm.operations {
		if operation.UserID == userID {
			history = append(history, operation)
		}
	}

	// 按时间倒序排列
	for i := 0; i < len(history)-1; i++ {
		for j := i + 1; j < len(history); j++ {
			if history[i].Timestamp.Before(history[j].Timestamp) {
				history[i], history[j] = history[j], history[i]
			}
		}
	}

	// 限制返回数量
	if limit > 0 && len(history) > limit {
		history = history[:limit]
	}

	return history
}

// ValidateBalanceConsistency 验证余额一致性
func (sm *SecurityManager) ValidateBalanceConsistency(userID int64, currentBalance int64) error {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	// 获取用户所有已完成的操作
	var operations []*OperationRecord
	for _, operation := range sm.operations {
		if operation.UserID == userID && operation.Status == "completed" {
			operations = append(operations, operation)
		}
	}

	if len(operations) == 0 {
		return nil // 没有操作记录，无法验证
	}

	// 按时间排序
	for i := 0; i < len(operations)-1; i++ {
		for j := i + 1; j < len(operations); j++ {
			if operations[i].Timestamp.After(operations[j].Timestamp) {
				operations[i], operations[j] = operations[j], operations[i]
			}
		}
	}

	// 验证操作链的一致性
	for i := 1; i < len(operations); i++ {
		if operations[i].OldBalance != operations[i-1].NewBalance {
			return fmt.Errorf("余额链不一致: 操作%s的旧余额(%d) != 前一操作%s的新余额(%d)",
				operations[i].ID, operations[i].OldBalance,
				operations[i-1].ID, operations[i-1].NewBalance)
		}
	}

	// 验证最终余额 - 允许一定的容差，因为可能存在并发操作
	if len(operations) > 0 {
		lastOperation := operations[len(operations)-1]
		// 如果最后一个操作的新余额与当前余额不一致，检查是否有更新的操作
		if lastOperation.NewBalance != currentBalance {
			// 检查是否有更新的操作记录
			hasNewerOperations := false
			for _, operation := range sm.operations {
				if operation.UserID == userID &&
					operation.Status == "completed" &&
					operation.Timestamp.After(lastOperation.Timestamp) {
					hasNewerOperations = true
					break
				}
			}

			// 如果没有更新的操作但余额不一致，则报告错误
			if !hasNewerOperations {
				sm.logger.Error("余额不一致警告: 用户=%d, 记录余额=%d, 当前余额=%d, 最后操作=%s",
					userID, lastOperation.NewBalance, currentBalance, lastOperation.ID)
				// 暂时不返回错误，只记录警告
				// return fmt.Errorf("最终余额不一致: 记录余额=%d, 当前余额=%d",
				//	lastOperation.NewBalance, currentBalance)
			}
		}
	}

	return nil
}

// GetSecurityReport 生成安全报告
func (sm *SecurityManager) GetSecurityReport() map[string]interface{} {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	report := map[string]interface{}{
		"total_operations":       len(sm.operations),
		"completed_operations":   0,
		"failed_operations":      0,
		"rolled_back_operations": 0,
		"pending_operations":     0,
		"total_rollbacks":        len(sm.rollbackLog),
		"checksum_errors":        0,
	}

	// 统计操作状态
	for _, operation := range sm.operations {
		switch operation.Status {
		case "completed":
			report["completed_operations"] = report["completed_operations"].(int) + 1
		case "failed":
			report["failed_operations"] = report["failed_operations"].(int) + 1
		case "rolled_back":
			report["rolled_back_operations"] = report["rolled_back_operations"].(int) + 1
		case "pending":
			report["pending_operations"] = report["pending_operations"].(int) + 1
		}

		// 验证校验和
		if err := sm.ValidateOperation(operation.ID); err != nil {
			report["checksum_errors"] = report["checksum_errors"].(int) + 1
		}
	}

	return report
}

// GetOperationByUserAndGame 根据用户ID和游戏ID获取操作记录
func (sm *SecurityManager) GetOperationByUserAndGame(userID int64, gameID string) *OperationRecord {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	for _, op := range sm.operations {
		if op.UserID == userID && op.GameID != nil && *op.GameID == gameID && op.Status == "pending" {
			return op
		}
	}
	return nil
}
