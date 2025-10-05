package game

import (
	"fmt"
	"sync"
	"time"

	"telegram-dice-bot/internal/config"
	"telegram-dice-bot/internal/database"
	"telegram-dice-bot/internal/logger"
	"telegram-dice-bot/internal/models"
	"telegram-dice-bot/internal/security"
	"telegram-dice-bot/internal/utils"
)

// EnhancedManager 增强的游戏管理器，集成安全机制
type EnhancedManager struct {
	*Manager                                    // 嵌入原有管理器
	security        *security.SecurityManager  // 安全管理器
	logger          *logger.Logger             // 日志系统
	operationMutex  sync.RWMutex               // 操作级别互斥锁
	auditLog        map[string]*AuditRecord    // 审计日志
	auditMutex      sync.RWMutex               // 审计日志互斥锁
}

// AuditRecord 审计记录
type AuditRecord struct {
	ID          string                 `json:"id"`
	Operation   string                 `json:"operation"`
	UserID      int64                  `json:"user_id"`
	GameID      *string                `json:"game_id,omitempty"`
	Details     map[string]interface{} `json:"details"`
	Timestamp   time.Time              `json:"timestamp"`
	Success     bool                   `json:"success"`
	ErrorMsg    string                 `json:"error_msg,omitempty"`
	Duration    time.Duration          `json:"duration"`
}

// NewEnhancedManager 创建增强的游戏管理器
func NewEnhancedManager(db *database.DB, cfg *config.Config, feeRate float64, logger *logger.Logger) *EnhancedManager {
	// 创建原有管理器
	originalManager := NewManager(db, cfg, feeRate)
	
	// 创建安全管理器
	securityManager := security.NewSecurityManager(logger)
	
	return &EnhancedManager{
		Manager:  originalManager,
		security: securityManager,
		logger:   logger,
		auditLog: make(map[string]*AuditRecord),
	}
}

// CreateGameSecure 安全的创建游戏方法
func (em *EnhancedManager) CreateGameSecure(playerID, chatID int64, betAmount int64) (string, error) {
	startTime := time.Now()
	auditID := em.security.GenerateOperationID()
	
	// 记录审计开始
	audit := &AuditRecord{
		ID:        auditID,
		Operation: "create_game",
		UserID:    playerID,
		Details: map[string]interface{}{
			"chat_id":    chatID,
			"bet_amount": betAmount,
		},
		Timestamp: startTime,
	}
	
	defer func() {
		audit.Duration = time.Since(startTime)
		em.recordAudit(audit)
	}()

	em.operationMutex.Lock()
	defer em.operationMutex.Unlock()

	// 验证输入参数
	if betAmount <= 0 {
		audit.Success = false
		audit.ErrorMsg = "下注金额必须大于0"
		return "", fmt.Errorf("%s", audit.ErrorMsg)
	}

	if betAmount > em.config.MaxBet {
		audit.Success = false
		audit.ErrorMsg = fmt.Sprintf("下注金额不能超过%d", em.config.MaxBet)
		return "", fmt.Errorf(audit.ErrorMsg)
	}

	// 获取用户信息
	user, err := em.db.GetUser(playerID)
	if err != nil {
		audit.Success = false
		audit.ErrorMsg = fmt.Sprintf("获取用户信息失败: %v", err)
		return "", fmt.Errorf(audit.ErrorMsg)
	}

	if user == nil {
		audit.Success = false
		audit.ErrorMsg = "用户不存在"
		return "", fmt.Errorf(audit.ErrorMsg)
	}

	// 记录操作前的余额
	oldBalance := user.Balance
	audit.Details["old_balance"] = oldBalance

	// 严格的余额验证
	if user.Balance < betAmount {
		audit.Success = false
		audit.ErrorMsg = fmt.Sprintf("余额不足，请存款后再试。当前余额: %d，需要: %d", user.Balance, betAmount)
		return "", fmt.Errorf(audit.ErrorMsg)
	}

	// 计算新余额
	newBalance := user.Balance - betAmount
	if newBalance < 0 {
		audit.Success = false
		audit.ErrorMsg = "余额不足，请存款后再试"
		return "", fmt.Errorf(audit.ErrorMsg)
	}

	audit.Details["new_balance"] = newBalance

	// 记录安全操作
	gameID := utils.GenerateGameID()
	audit.Details["game_id"] = gameID
	
	securityOp := em.security.RecordOperation(
		playerID, 
		&gameID, 
		"bet", 
		-betAmount, 
		oldBalance, 
		newBalance,
		map[string]interface{}{
			"operation": "create_game",
			"chat_id":   chatID,
		},
	)

	// 验证操作完整性
	if err := em.security.ValidateOperation(securityOp.ID); err != nil {
		audit.Success = false
		audit.ErrorMsg = fmt.Sprintf("安全验证失败: %v", err)
		em.security.FailOperation(securityOp.ID, audit.ErrorMsg)
		return "", fmt.Errorf(audit.ErrorMsg)
	}

	// 创建游戏和交易记录
	game := &models.Game{
		ID:        gameID,
		Player1ID: playerID,
		BetAmount: betAmount,
		Status:    models.GameStatusWaiting,
		ChatID:    chatID,
	}

	tx := &models.Transaction{
		ID:          utils.GenerateTransactionID(),
		UserID:      playerID,
		GameID:      &gameID,
		Type:        models.TransactionTypeBet,
		Amount:      -betAmount,
		Balance:     newBalance,
		Description: fmt.Sprintf("参与游戏 %s", gameID),
	}

	// 使用事务确保原子性
	if err := em.db.CreateGameWithTransaction(game, playerID, newBalance, tx); err != nil {
		audit.Success = false
		audit.ErrorMsg = fmt.Sprintf("创建游戏失败: %v", err)
		
		// 回滚安全操作
		em.security.RollbackOperation(securityOp.ID, audit.ErrorMsg)
		return "", fmt.Errorf(audit.ErrorMsg)
	}

	// 标记安全操作完成
	em.security.CompleteOperation(securityOp.ID)

	// 设置超时定时器
	em.setGameTimeout(gameID, 60*time.Second)

	// 记录成功
	audit.Success = true
	em.logger.Info(fmt.Sprintf("游戏创建成功: 用户=%d, 游戏ID=%s, 金额=%d", playerID, gameID, betAmount))

	return gameID, nil
}

// JoinGameSecure 安全的加入游戏方法
func (em *EnhancedManager) JoinGameSecure(gameID string, playerID int64) (*GameResult, error) {
	startTime := time.Now()
	auditID := em.security.GenerateOperationID()
	
	// 记录审计开始
	audit := &AuditRecord{
		ID:        auditID,
		Operation: "join_game",
		UserID:    playerID,
		GameID:    &gameID,
		Details: map[string]interface{}{
			"game_id": gameID,
		},
		Timestamp: startTime,
	}
	
	defer func() {
		audit.Duration = time.Since(startTime)
		em.recordAudit(audit)
	}()

	em.operationMutex.Lock()
	defer em.operationMutex.Unlock()

	// 验证输入参数
	if gameID == "" {
		audit.Success = false
		audit.ErrorMsg = "游戏ID不能为空"
		return nil, fmt.Errorf(audit.ErrorMsg)
	}

	if playerID <= 0 {
		audit.Success = false
		audit.ErrorMsg = "无效的玩家ID"
		return nil, fmt.Errorf(audit.ErrorMsg)
	}

	// 获取游戏信息
	game, err := em.db.GetGame(gameID)
	if err != nil {
		audit.Success = false
		audit.ErrorMsg = fmt.Sprintf("获取游戏信息失败: %v", err)
		return nil, fmt.Errorf(audit.ErrorMsg)
	}

	if game == nil {
		audit.Success = false
		audit.ErrorMsg = "游戏不存在"
		return nil, fmt.Errorf(audit.ErrorMsg)
	}

	audit.Details["bet_amount"] = game.BetAmount
	audit.Details["player1_id"] = game.Player1ID

	if game.Status != models.GameStatusWaiting {
		audit.Success = false
		audit.ErrorMsg = "游戏已开始或已结束"
		return nil, fmt.Errorf(audit.ErrorMsg)
	}

	if game.Player1ID == playerID {
		audit.Success = false
		audit.ErrorMsg = "不能加入自己创建的游戏"
		return nil, fmt.Errorf(audit.ErrorMsg)
	}

	// 使用余额验证器进行预验证
	if err := em.validator.ValidateUserBalance(playerID, game.BetAmount); err != nil {
		audit.Success = false
		audit.ErrorMsg = err.Error()
		return nil, err
	}

	// 获取玩家信息
	player2, err := em.db.GetUser(playerID)
	if err != nil {
		audit.Success = false
		audit.ErrorMsg = fmt.Sprintf("获取玩家信息失败: %v", err)
		return nil, fmt.Errorf(audit.ErrorMsg)
	}

	if player2 == nil {
		audit.Success = false
		audit.ErrorMsg = "玩家不存在"
		return nil, fmt.Errorf(audit.ErrorMsg)
	}

	// 记录操作前的余额
	oldBalance := player2.Balance
	audit.Details["old_balance"] = oldBalance

	// 严格的余额验证
	if player2.Balance < game.BetAmount {
		audit.Success = false
		audit.ErrorMsg = fmt.Sprintf("余额不足，请存款后再试。当前余额: %d，需要: %d", player2.Balance, game.BetAmount)
		return nil, fmt.Errorf("余额不足，请存款后再试。当前余额: %d，需要: %d", player2.Balance, game.BetAmount)
	}

	// 计算新余额
	newBalance := player2.Balance - game.BetAmount
	if newBalance < 0 {
		audit.Success = false
		audit.ErrorMsg = "余额不足，请存款后再试"
		return nil, fmt.Errorf(audit.ErrorMsg)
	}

	audit.Details["new_balance"] = newBalance

	// 记录安全操作
	securityOp := em.security.RecordOperation(
		playerID, 
		&gameID, 
		"bet", 
		-game.BetAmount, 
		oldBalance, 
		newBalance,
		map[string]interface{}{
			"operation": "join_game",
			"game_id":   gameID,
		},
	)

	// 验证操作完整性
	if err := em.security.ValidateOperation(securityOp.ID); err != nil {
		audit.Success = false
		audit.ErrorMsg = fmt.Sprintf("安全验证失败: %v", err)
		em.security.FailOperation(securityOp.ID, audit.ErrorMsg)
		return nil, fmt.Errorf(audit.ErrorMsg)
	}

	// 创建交易记录
	tx2 := &models.Transaction{
		ID:          utils.GenerateTransactionID(),
		UserID:      playerID,
		GameID:      &gameID,
		Type:        models.TransactionTypeBet,
		Amount:      -game.BetAmount,
		Balance:     newBalance,
		Description: fmt.Sprintf("参与游戏 %s", gameID),
	}

	// 使用事务确保原子性
	if err := em.db.JoinGameWithTransaction(gameID, playerID, newBalance, tx2); err != nil {
		audit.Success = false
		audit.ErrorMsg = fmt.Sprintf("加入游戏失败: %v", err)
		
		// 回滚安全操作
		em.security.RollbackOperation(securityOp.ID, audit.ErrorMsg)
		return nil, fmt.Errorf(audit.ErrorMsg)
	}

	// 标记安全操作完成
	em.security.CompleteOperation(securityOp.ID)

	// 取消游戏超时定时器
	em.cancelGameTimeout(gameID)

	// playGameAndSettleNoLock 开始游戏并自动结算（不使用锁，避免死锁）
	result, err := em.playGameAndSettleNoLock(game, playerID)
	if err != nil {
		audit.Success = false
		audit.ErrorMsg = fmt.Sprintf("开始游戏失败: %v", err)
		return nil, err
	}

	// 记录成功
	audit.Success = true
	audit.Details["result"] = map[string]interface{}{
		"winner":  result.Winner,
		"is_draw": result.Winner == nil,
	}
	
	em.logger.Info(fmt.Sprintf("加入游戏成功: 用户=%d, 游戏ID=%s, 金额=%d", playerID, gameID, game.BetAmount))

	return result, nil
}

// recordAudit 记录审计日志
func (em *EnhancedManager) recordAudit(audit *AuditRecord) {
	em.auditMutex.Lock()
	defer em.auditMutex.Unlock()
	
	em.auditLog[audit.ID] = audit
	
	// 记录到日志系统
	if audit.Success {
		em.logger.Info(fmt.Sprintf("审计记录: %s - %s 成功, 用户=%d, 耗时=%v", 
			audit.ID, audit.Operation, audit.UserID, audit.Duration))
	} else {
		em.logger.Error(fmt.Sprintf("审计记录: %s - %s 失败, 用户=%d, 错误=%s, 耗时=%v", 
			audit.ID, audit.Operation, audit.UserID, audit.ErrorMsg, audit.Duration))
	}
}

// GetSecurityReport 获取安全报告
func (em *EnhancedManager) GetSecurityReport() map[string]interface{} {
	securityReport := em.security.GetSecurityReport()
	
	em.auditMutex.RLock()
	defer em.auditMutex.RUnlock()
	
	// 添加审计统计
	auditStats := map[string]interface{}{
		"total_audits":      len(em.auditLog),
		"successful_audits": 0,
		"failed_audits":     0,
	}
	
	for _, audit := range em.auditLog {
		if audit.Success {
			auditStats["successful_audits"] = auditStats["successful_audits"].(int) + 1
		} else {
			auditStats["failed_audits"] = auditStats["failed_audits"].(int) + 1
		}
	}
	
	securityReport["audit_stats"] = auditStats
	return securityReport
}

// ValidateUserBalanceConsistency 验证用户余额一致性
func (em *EnhancedManager) ValidateUserBalanceConsistency(userID int64) error {
	user, err := em.db.GetUser(userID)
	if err != nil {
		return fmt.Errorf("获取用户信息失败: %v", err)
	}
	
	if user == nil {
		return fmt.Errorf("用户不存在")
	}
	
	return em.security.ValidateBalanceConsistency(userID, user.Balance)
}

// PlayGameWithDiceResultsSecure 安全的游戏结算方法
func (em *EnhancedManager) PlayGameWithDiceResultsSecure(gameID string, dice1, dice2, dice3, dice4, dice5, dice6 int) (*GameResult, error) {
	startTime := time.Now()
	auditID := em.security.GenerateOperationID()
	
	// 记录审计开始
	audit := &AuditRecord{
		ID:        auditID,
		Operation: "game_settlement",
		GameID:    &gameID,
		Details: map[string]interface{}{
			"game_id": gameID,
			"dice":    []int{dice1, dice2, dice3, dice4, dice5, dice6},
		},
		Timestamp: startTime,
	}
	
	defer func() {
		audit.Duration = time.Since(startTime)
		em.recordAudit(audit)
	}()

	em.operationMutex.Lock()
	defer em.operationMutex.Unlock()

	// 获取游戏信息
	game, err := em.db.GetGame(gameID)
	if err != nil {
		audit.Success = false
		audit.ErrorMsg = fmt.Sprintf("获取游戏信息失败: %v", err)
		return nil, fmt.Errorf(audit.ErrorMsg)
	}

	if game == nil {
		audit.Success = false
		audit.ErrorMsg = "游戏不存在"
		return nil, fmt.Errorf(audit.ErrorMsg)
	}

	if game.Status != models.GameStatusPlaying {
		audit.Success = false
		audit.ErrorMsg = "游戏状态不正确"
		return nil, fmt.Errorf(audit.ErrorMsg)
	}

	// 计算骰子总和
	player1Total := dice1 + dice2 + dice3
	player2Total := dice4 + dice5 + dice6
	
	audit.Details["player1_total"] = player1Total
	audit.Details["player2_total"] = player2Total

	// 计算总奖池和抽水
	totalPot := game.BetAmount * 2
	commission := utils.CalculateCommission(totalPot, em.feeRate)
	
	audit.Details["total_pot"] = totalPot
	audit.Details["commission"] = commission

	var transactions []*models.Transaction
	var winnerID *int64
	var winAmount int64

	// 判断游戏结果
	if player1Total == player2Total {
		// 平局 - 退还双方本金
		audit.Details["result"] = "draw"
		
		// 玩家1退款
		player1, err := em.db.GetUser(game.Player1ID)
		if err != nil {
			audit.Success = false
			audit.ErrorMsg = fmt.Sprintf("获取玩家1信息失败: %v", err)
			return nil, fmt.Errorf(audit.ErrorMsg)
		}
		
		newBalance1 := player1.Balance + game.BetAmount
		
		// 记录玩家1退款安全操作
		securityOp1 := em.security.RecordOperation(
			game.Player1ID,
			&gameID,
			"refund",
			game.BetAmount,
			player1.Balance,
			newBalance1,
			map[string]interface{}{
				"operation": "draw_refund",
				"reason":    "平局退款",
			},
		)

		tx1 := &models.Transaction{
			ID:          utils.GenerateTransactionID(),
			UserID:      game.Player1ID,
			GameID:      &gameID,
			Type:        models.TransactionTypeRefund,
			Amount:      game.BetAmount,
			Balance:     newBalance1,
			Description: "平局退款",
		}
		transactions = append(transactions, tx1)

		// 玩家2退款
		if game.Player2ID != nil {
			player2, err := em.db.GetUser(*game.Player2ID)
			if err != nil {
				audit.Success = false
				audit.ErrorMsg = fmt.Sprintf("获取玩家2信息失败: %v", err)
				em.security.FailOperation(securityOp1.ID, audit.ErrorMsg)
				return nil, fmt.Errorf(audit.ErrorMsg)
			}
			
			newBalance2 := player2.Balance + game.BetAmount
			
			// 记录玩家2退款安全操作
			securityOp2 := em.security.RecordOperation(
				*game.Player2ID,
				&gameID,
				"refund",
				game.BetAmount,
				player2.Balance,
				newBalance2,
				map[string]interface{}{
					"operation": "draw_refund",
					"reason":    "平局退款",
				},
			)

			tx2 := &models.Transaction{
				ID:          utils.GenerateTransactionID(),
				UserID:      *game.Player2ID,
				GameID:      &gameID,
				Type:        models.TransactionTypeRefund,
				Amount:      game.BetAmount,
				Balance:     newBalance2,
				Description: "平局退款",
			}
			transactions = append(transactions, tx2)

			// 验证安全操作
			if err := em.security.ValidateOperation(securityOp2.ID); err != nil {
				audit.Success = false
				audit.ErrorMsg = fmt.Sprintf("玩家2安全验证失败: %v", err)
				em.security.FailOperation(securityOp1.ID, audit.ErrorMsg)
				em.security.FailOperation(securityOp2.ID, audit.ErrorMsg)
				return nil, fmt.Errorf(audit.ErrorMsg)
			}
		}

		// 验证玩家1安全操作
		if err := em.security.ValidateOperation(securityOp1.ID); err != nil {
			audit.Success = false
			audit.ErrorMsg = fmt.Sprintf("玩家1安全验证失败: %v", err)
			em.security.FailOperation(securityOp1.ID, audit.ErrorMsg)
			return nil, fmt.Errorf(audit.ErrorMsg)
		}

	} else {
		// 有胜负
		if player1Total > player2Total {
			winnerID = &game.Player1ID
			audit.Details["winner"] = "player1"
		} else {
			winnerID = game.Player2ID
			audit.Details["winner"] = "player2"
		}
		
		winAmount = totalPot - commission
		audit.Details["win_amount"] = winAmount

		// 获取获胜者信息
		winner, err := em.db.GetUser(*winnerID)
		if err != nil {
			audit.Success = false
			audit.ErrorMsg = fmt.Sprintf("获取获胜者信息失败: %v", err)
			return nil, fmt.Errorf(audit.ErrorMsg)
		}

		newWinnerBalance := winner.Balance + winAmount

		// 记录获胜者奖金安全操作
		securityOp := em.security.RecordOperation(
			*winnerID,
			&gameID,
			"win",
			winAmount,
			winner.Balance,
			newWinnerBalance,
			map[string]interface{}{
				"operation":   "game_win",
				"commission":  commission,
				"total_pot":   totalPot,
			},
		)

		// 验证安全操作
		if err := em.security.ValidateOperation(securityOp.ID); err != nil {
			audit.Success = false
			audit.ErrorMsg = fmt.Sprintf("获胜者安全验证失败: %v", err)
			em.security.FailOperation(securityOp.ID, audit.ErrorMsg)
			return nil, fmt.Errorf(audit.ErrorMsg)
		}

		// 创建获胜者交易记录
		tx := &models.Transaction{
			ID:          utils.GenerateTransactionID(),
			UserID:      *winnerID,
			GameID:      &gameID,
			Type:        models.TransactionTypeWin,
			Amount:      winAmount,
			Balance:     newWinnerBalance,
			Description: fmt.Sprintf("游戏获胜奖金 %s", gameID),
		}
		transactions = append(transactions, tx)
	}

	// 使用事务执行结算
	err = em.db.SettleGameWithTransactionEnhanced(gameID, winnerID, winAmount, int64(commission), 
		dice1, dice2, dice3, dice4, dice5, dice6, transactions)
	if err != nil {
		audit.Success = false
		audit.ErrorMsg = fmt.Sprintf("游戏结算失败: %v", err)
		
		// 回滚所有安全操作
		for _, tx := range transactions {
			if op := em.security.GetOperationByUserAndGame(tx.UserID, gameID); op != nil {
				em.security.RollbackOperation(op.ID, audit.ErrorMsg)
			}
		}
		return nil, fmt.Errorf(audit.ErrorMsg)
	}

	// 标记所有安全操作完成
	for _, tx := range transactions {
		if op := em.security.GetOperationByUserAndGame(tx.UserID, gameID); op != nil {
			em.security.CompleteOperation(op.ID)
		}
	}

	// 构建游戏结果 - 重新获取更新后的游戏信息
	updatedGame, err := em.db.GetGame(gameID)
	if err != nil {
		audit.Success = false
		audit.ErrorMsg = fmt.Sprintf("获取更新后游戏信息失败: %v", err)
		return nil, fmt.Errorf(audit.ErrorMsg)
	}
	
	result, err := em.buildGameResult(updatedGame, player1Total == player2Total)
	if err != nil {
		audit.Success = false
		audit.ErrorMsg = fmt.Sprintf("构建游戏结果失败: %v", err)
		return nil, fmt.Errorf(audit.ErrorMsg)
	}

	// 记录成功
	audit.Success = true
	audit.UserID = game.Player1ID // 设置主要用户ID
	em.logger.Info(fmt.Sprintf("游戏结算成功: 游戏ID=%s, 结果=%s", gameID, audit.Details["result"]))

	return result, nil
}

// ExpireGameSecure 安全的游戏超时处理方法
func (em *EnhancedManager) ExpireGameSecure(gameID string) error {
	startTime := time.Now()
	auditID := em.security.GenerateOperationID()
	
	// 记录审计开始
	audit := &AuditRecord{
		ID:        auditID,
		Operation: "game_timeout",
		GameID:    &gameID,
		Details: map[string]interface{}{
			"game_id": gameID,
			"reason":  "游戏超时",
		},
		Timestamp: startTime,
	}
	
	defer func() {
		audit.Duration = time.Since(startTime)
		em.recordAudit(audit)
	}()

	em.operationMutex.Lock()
	defer em.operationMutex.Unlock()

	// 获取游戏信息
	game, err := em.db.GetGame(gameID)
	if err != nil {
		audit.Success = false
		audit.ErrorMsg = fmt.Sprintf("获取游戏信息失败: %v", err)
		return fmt.Errorf(audit.ErrorMsg)
	}

	if game == nil {
		audit.Success = false
		audit.ErrorMsg = "游戏不存在"
		return fmt.Errorf(audit.ErrorMsg)
	}

	if game.Status != models.GameStatusWaiting {
		audit.Success = false
		audit.ErrorMsg = "游戏状态不正确，无法超时"
		return fmt.Errorf(audit.ErrorMsg)
	}

	audit.UserID = game.Player1ID

	// 获取玩家1信息 - 重新从数据库获取最新余额
	player1, err := em.db.GetUser(game.Player1ID)
	if err != nil {
		audit.Success = false
		audit.ErrorMsg = fmt.Sprintf("获取玩家1信息失败: %v", err)
		return fmt.Errorf(audit.ErrorMsg)
	}

	// 计算退款金额和新余额
	refundAmount := game.BetAmount
	newBalance := player1.Balance + refundAmount
	
	// 验证余额逻辑 - 确保退款后余额合理
	if player1.Balance < 0 {
		audit.Success = false
		audit.ErrorMsg = "用户余额异常，无法执行退款"
		return fmt.Errorf(audit.ErrorMsg)
	}
	
	audit.Details["refund_amount"] = refundAmount
	audit.Details["old_balance"] = player1.Balance
	audit.Details["new_balance"] = newBalance

	// 记录安全操作 - 退款操作，金额为正数
	securityOp := em.security.RecordOperation(
		game.Player1ID,
		&gameID,
		"refund",
		refundAmount,  // 退款金额为正数
		player1.Balance,
		newBalance,
		map[string]interface{}{
			"operation": "timeout_refund",
			"reason":    "游戏超时退款",
		},
	)

	// 验证安全操作
	if err := em.security.ValidateOperation(securityOp.ID); err != nil {
		audit.Success = false
		audit.ErrorMsg = fmt.Sprintf("安全验证失败: %v", err)
		em.security.FailOperation(securityOp.ID, audit.ErrorMsg)
		return fmt.Errorf(audit.ErrorMsg)
	}

	// 创建退款交易记录
	tx := &models.Transaction{
		ID:          utils.GenerateTransactionID(),
		UserID:      game.Player1ID,
		GameID:      &gameID,
		Type:        models.TransactionTypeRefund,
		Amount:      refundAmount,
		Balance:     newBalance,
		Description: "游戏超时退款",
	}

	// 使用事务执行超时退款
	err = em.db.ExpireGameWithTransaction(gameID, game.Player1ID, newBalance, tx)
	if err != nil {
		audit.Success = false
		audit.ErrorMsg = fmt.Sprintf("超时退款失败: %v", err)
		em.security.RollbackOperation(securityOp.ID, audit.ErrorMsg)
		return fmt.Errorf(audit.ErrorMsg)
	}

	// 标记安全操作完成
	em.security.CompleteOperation(securityOp.ID)

	// 取消游戏超时定时器
	em.cancelGameTimeout(gameID)

	// 发送超时通知
	if em.onGameExpired != nil {
		em.onGameExpired(gameID, game.ChatID)
	}

	// 记录成功
	audit.Success = true
	em.logger.Info(fmt.Sprintf("游戏超时处理成功: 游戏ID=%s, 退款金额=%d", gameID, refundAmount))

	return nil
}

// ...

// playGameAndSettleNoLock 开始游戏并自动结算（不使用锁，避免死锁）
func (em *EnhancedManager) playGameAndSettleNoLock(game *models.Game, player2ID int64) (*GameResult, error) {
	// 更新游戏状态为进行中
	game.Player2ID = &player2ID
	game.Status = models.GameStatusPlaying

	// 更新游戏记录
	if err := em.db.UpdateGame(game); err != nil {
		return nil, err
	}

	// 生成随机骰子结果 (1-6)
	dice1, _ := utils.SecureRandomInt(6)
	dice1 += 1
	dice2, _ := utils.SecureRandomInt(6)
	dice2 += 1
	dice3, _ := utils.SecureRandomInt(6)
	dice3 += 1
	dice4, _ := utils.SecureRandomInt(6)
	dice4 += 1
	dice5, _ := utils.SecureRandomInt(6)
	dice5 += 1
	dice6, _ := utils.SecureRandomInt(6)
	dice6 += 1

	// 直接调用原始Manager的PlayGameWithDiceResults方法避免死锁
	return em.Manager.PlayGameWithDiceResults(game.ID, int(dice1), int(dice2), int(dice3), int(dice4), int(dice5), int(dice6))
}

// ...

// GetDB 获取数据库实例（用于测试）
func (em *EnhancedManager) GetDB() *database.DB {
	return em.db
}