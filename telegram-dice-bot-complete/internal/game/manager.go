package game

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"
	"time"

	"telegram-dice-bot/internal/database"
	"telegram-dice-bot/internal/models"
	"telegram-dice-bot/internal/utils"
)

type Manager struct {
	db    *database.DB
	mutex sync.RWMutex
	// 游戏超时管理
	gameTimers map[string]*time.Timer
	timerMutex sync.RWMutex
	// 超时通知回调
	onGameExpired func(gameID string, chatID int64)
}

type GameResult struct {
	GameID  string
	Player1 *models.User
	Player2 *models.User
	// 玩家1的3个骰子
	Player1Dice1 int
	Player1Dice2 int
	Player1Dice3 int
	Player1Total int
	// 玩家2的3个骰子
	Player2Dice1 int
	Player2Dice2 int
	Player2Dice3 int
	Player2Total int
	Winner       *models.User
	WinAmount    int64
	Commission   int64
	BetAmount    int64
	RandomSeed   string
}

func NewManager(db *database.DB) *Manager {
	manager := &Manager{
		db:         db,
		gameTimers: make(map[string]*time.Timer),
	}

	// 启动定期清理过期游戏的后台任务
	go manager.startCleanupTask()

	return manager
}

// SetGameExpiredCallback 设置游戏超时回调函数
func (m *Manager) SetGameExpiredCallback(callback func(gameID string, chatID int64)) {
	m.onGameExpired = callback
}

func (m *Manager) CreateGame(playerID, chatID int64, betAmount int64) (string, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 验证输入参数
	if betAmount <= 0 {
		return "", fmt.Errorf("下注金额必须大于0")
	}

	if betAmount > 10000 {
		return "", fmt.Errorf("下注金额不能超过10000")
	}

	// 检查用户余额
	user, err := m.db.GetUser(playerID)
	if err != nil {
		return "", fmt.Errorf("获取用户信息失败: %v", err)
	}

	if user == nil {
		return "", fmt.Errorf("用户不存在")
	}

	if user.Balance < betAmount {
		return "", fmt.Errorf("余额不足")
	}

	// 创建游戏和交易记录
	gameID := utils.GenerateGameID()
	game := &models.Game{
		ID:        gameID,
		Player1ID: playerID,
		BetAmount: betAmount,
		Status:    models.GameStatusWaiting,
		ChatID:    chatID,
	}

	newBalance := user.Balance - betAmount
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
	if err := m.db.CreateGameWithTransaction(game, playerID, newBalance, tx); err != nil {
		return "", fmt.Errorf("创建游戏失败: %v", err)
	}

	// 设置60秒超时定时器
	m.setGameTimeout(gameID, 60*time.Second)

	return gameID, nil
}

func (m *Manager) JoinGame(gameID string, playerID int64) (*GameResult, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 验证输入参数
	if gameID == "" {
		return nil, fmt.Errorf("游戏ID不能为空")
	}

	if playerID <= 0 {
		return nil, fmt.Errorf("无效的玩家ID")
	}

	// 获取游戏信息
	game, err := m.db.GetGame(gameID)
	if err != nil {
		return nil, fmt.Errorf("获取游戏信息失败: %v", err)
	}

	if game == nil {
		return nil, fmt.Errorf("游戏不存在")
	}

	if game.Status != models.GameStatusWaiting {
		return nil, fmt.Errorf("游戏已开始或已结束")
	}

	if game.Player1ID == playerID {
		return nil, fmt.Errorf("不能加入自己创建的游戏")
	}

	// 检查玩家2余额
	player2, err := m.db.GetUser(playerID)
	if err != nil {
		return nil, fmt.Errorf("获取玩家信息失败: %v", err)
	}

	if player2 == nil {
		return nil, fmt.Errorf("玩家不存在")
	}

	if player2.Balance < game.BetAmount {
		return nil, fmt.Errorf("余额不足")
	}

	// 扣除玩家2的下注金额并记录交易
	newBalance := player2.Balance - game.BetAmount
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
	if err := m.db.JoinGameWithTransaction(gameID, playerID, newBalance, tx2); err != nil {
		return nil, fmt.Errorf("加入游戏失败: %v", err)
	}

	// 取消游戏超时定时器（有人加入了）
	m.cancelGameTimeout(gameID)

	// 开始游戏
	return m.playGame(game, playerID)
}

func (m *Manager) playGame(game *models.Game, player2ID int64) (*GameResult, error) {
	// 更新游戏状态为进行中，等待骰子结果
	game.Player2ID = &player2ID
	game.Status = models.GameStatusPlaying

	// 先更新游戏记录，但不设置骰子结果
	if err := m.db.UpdateGame(game); err != nil {
		return nil, err
	}

	// 获取玩家信息
	player1, err := m.db.GetUser(game.Player1ID)
	if err != nil {
		return nil, err
	}

	player2, err := m.db.GetUser(player2ID)
	if err != nil {
		return nil, err
	}

	// 返回游戏结果，但不包含骰子点数（将由TG动画提供）
	result := &GameResult{
		GameID:    game.ID,
		Player1:   player1,
		Player2:   player2,
		BetAmount: game.BetAmount,
	}

	return result, nil
}

// PlayGameWithDiceResults 使用TG骰子动画的实际结果完成游戏
func (m *Manager) PlayGameWithDiceResults(gameID string, p1d1, p1d2, p1d3, p2d1, p2d2, p2d3 int) (*GameResult, error) {
	// 获取游戏信息
	game, err := m.db.GetGame(gameID)
	if err != nil {
		return nil, err
	}

	if game == nil {
		return nil, fmt.Errorf("游戏不存在")
	}

	if game.Status != models.GameStatusPlaying {
		return nil, fmt.Errorf("游戏状态错误")
	}

	// 计算总和
	player1Total := p1d1 + p1d2 + p1d3
	player2Total := p2d1 + p2d2 + p2d3

	// 计算结果
	totalPot := game.BetAmount * 2
	commission := int64(float64(totalPot) * 0.1) // 10% 手续费
	winAmount := totalPot - commission

	// 检查是否平局
	if player1Total == player2Total {
		// 平局，退还下注金额
		if err := m.refundGame(game); err != nil {
			return nil, err
		}
		
		// 更新游戏状态为平局
		game.Status = models.GameStatusFinished
		game.Player1Dice1 = &p1d1
		game.Player1Dice2 = &p1d2
		game.Player1Dice3 = &p1d3
		game.Player2Dice1 = &p2d1
		game.Player2Dice2 = &p2d2
		game.Player2Dice3 = &p2d3
		
		if err := m.db.UpdateGame(game); err != nil {
			return nil, err
		}
		
		result, _ := m.buildGameResult(game, true)
		return result, nil
	}

	// 确定获胜者
	var winnerID int64
	if player1Total > player2Total {
		winnerID = game.Player1ID
	} else {
		winnerID = *game.Player2ID
	}

	// 获取获胜者信息
	winner, err := m.db.GetUser(winnerID)
	if err != nil {
		return nil, err
	}
	newWinnerBalance := winner.Balance + winAmount

	// 准备交易记录
	var transactions []*models.Transaction

	// 获胜交易记录
	winTx := &models.Transaction{
		ID:          utils.GenerateTransactionID(),
		UserID:      winnerID,
		GameID:      &game.ID,
		Type:        models.TransactionTypeWin,
		Amount:      winAmount,
		Balance:     newWinnerBalance,
		Description: fmt.Sprintf("赢得游戏 %s", game.ID),
	}
	transactions = append(transactions, winTx)

	// 手续费交易记录
	commissionTx := &models.Transaction{
		ID:          utils.GenerateTransactionID(),
		UserID:      0, // 系统账户
		GameID:      &game.ID,
		Type:        models.TransactionTypeCommission,
		Amount:      commission,
		Balance:     0,
		Description: fmt.Sprintf("游戏 %s 手续费", game.ID),
	}
	transactions = append(transactions, commissionTx)

	// 使用事务结算游戏
	if err := m.db.SettleGameWithTransaction(game.ID, &winnerID, commission, 
		p1d1, p1d2, p1d3, p2d1, p2d2, p2d3, newWinnerBalance, transactions); err != nil {
		return nil, err
	}

	// 更新本地游戏对象以构建结果
	game.Status = models.GameStatusFinished
	game.Player1Dice1 = &p1d1
	game.Player1Dice2 = &p1d2
	game.Player1Dice3 = &p1d3
	game.Player2Dice1 = &p2d1
	game.Player2Dice2 = &p2d2
	game.Player2Dice3 = &p2d3
	game.WinnerID = &winnerID
	game.Commission = commission

	result, err := m.buildGameResult(game, false)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (m *Manager) refundGame(game *models.Game) error {
	// 获取玩家1信息
	player1, err := m.db.GetUser(game.Player1ID)
	if err != nil {
		return err
	}
	newBalance1 := player1.Balance + game.BetAmount

	// 准备交易记录
	var transactions []*models.Transaction
	
	// 玩家1退款交易记录
	tx1 := &models.Transaction{
		ID:          fmt.Sprintf("refund_%s_%d", game.ID, game.Player1ID),
		UserID:      game.Player1ID,
		GameID:      &game.ID,
		Type:        models.TransactionTypeRefund,
		Amount:      game.BetAmount,
		Balance:     newBalance1,
		Description: "游戏退款",
	}
	transactions = append(transactions, tx1)

	var player2ID *int64
	var newBalance2 *int64

	// 如果有玩家2，准备玩家2的退款
	if game.Player2ID != nil {
		player2, err := m.db.GetUser(*game.Player2ID)
		if err != nil {
			return err
		}
		balance2 := player2.Balance + game.BetAmount
		player2ID = game.Player2ID
		newBalance2 = &balance2

		// 玩家2退款交易记录
		tx2 := &models.Transaction{
			ID:          fmt.Sprintf("refund_%s_%d", game.ID, *game.Player2ID),
			UserID:      *game.Player2ID,
			GameID:      &game.ID,
			Type:        models.TransactionTypeRefund,
			Amount:      game.BetAmount,
			Balance:     balance2,
			Description: "游戏退款",
		}
		transactions = append(transactions, tx2)
	}

	// 使用事务执行退款
	return m.db.RefundGameWithTransaction(game.Player1ID, newBalance1, player2ID, newBalance2, transactions)
}

func (m *Manager) buildGameResult(game *models.Game, isDraw bool) (*GameResult, error) {
	player1, err := m.db.GetUser(game.Player1ID)
	if err != nil {
		return nil, err
	}

	var player2 *models.User
	if game.Player2ID != nil {
		player2, err = m.db.GetUser(*game.Player2ID)
		if err != nil {
			return nil, err
		}
	}

	result := &GameResult{
		GameID:     game.ID,
		Player1:    player1,
		Player2:    player2,
		Commission: game.Commission,
		BetAmount:  game.BetAmount,
	}

	// 设置骰子数据
	if game.Player1Dice1 != nil && game.Player1Dice2 != nil && game.Player1Dice3 != nil {
		result.Player1Dice1 = *game.Player1Dice1
		result.Player1Dice2 = *game.Player1Dice2
		result.Player1Dice3 = *game.Player1Dice3
		result.Player1Total = result.Player1Dice1 + result.Player1Dice2 + result.Player1Dice3
	}

	if game.Player2Dice1 != nil && game.Player2Dice2 != nil && game.Player2Dice3 != nil {
		result.Player2Dice1 = *game.Player2Dice1
		result.Player2Dice2 = *game.Player2Dice2
		result.Player2Dice3 = *game.Player2Dice3
		result.Player2Total = result.Player2Dice1 + result.Player2Dice2 + result.Player2Dice3
	}

	if !isDraw && game.WinnerID != nil {
		if *game.WinnerID == game.Player1ID {
			result.Winner = player1
		} else {
			result.Winner = player2
		}
		result.WinAmount = game.BetAmount*2 - game.Commission
	}

	return result, nil
}

// generateRandomSeed 生成可验证的随机种子
func (m *Manager) generateRandomSeed(gameID string, player1ID, player2ID int64) string {
	// 使用游戏ID、玩家ID和当前时间戳生成种子
	data := fmt.Sprintf("%s-%d-%d-%d", gameID, player1ID, player2ID, time.Now().UnixNano())
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// rollDiceWithSeed 使用种子生成可验证的骰子结果（3个骰子）
func (m *Manager) rollDiceWithSeed(seed string, playerIndex int) (int, int, int, error) {
	// 将种子和玩家索引组合
	seedData := fmt.Sprintf("%s-%d", seed, playerIndex)
	hash := sha256.Sum256([]byte(seedData))

	// 使用哈希的前8字节作为随机数种子
	var seedInt int64
	for i := 0; i < 8; i++ {
		seedInt = (seedInt << 8) | int64(hash[i])
	}

	// 生成3个骰子的结果
	dice1 := (seedInt % 6) + 1
	if dice1 < 0 {
		dice1 = -dice1
	}
	if dice1 == 0 {
		dice1 = 1
	}
	if dice1 > 6 {
		dice1 = (dice1 % 6) + 1
	}

	// 为第二个骰子生成新的种子
	hash2 := sha256.Sum256([]byte(seedData + "dice2"))
	var seedInt2 int64
	for i := 0; i < 8; i++ {
		seedInt2 = (seedInt2 << 8) | int64(hash2[i])
	}
	dice2 := (seedInt2 % 6) + 1
	if dice2 < 0 {
		dice2 = -dice2
	}
	if dice2 == 0 {
		dice2 = 1
	}
	if dice2 > 6 {
		dice2 = (dice2 % 6) + 1
	}

	// 为第三个骰子生成新的种子
	hash3 := sha256.Sum256([]byte(seedData + "dice3"))
	var seedInt3 int64
	for i := 0; i < 8; i++ {
		seedInt3 = (seedInt3 << 8) | int64(hash3[i])
	}
	dice3 := (seedInt3 % 6) + 1
	if dice3 < 0 {
		dice3 = -dice3
	}
	if dice3 == 0 {
		dice3 = 1
	}
	if dice3 > 6 {
		dice3 = (dice3 % 6) + 1
	}

	return int(dice1), int(dice2), int(dice3), nil
}

func (m *Manager) rollDice() (int, int, int, error) {
	// 使用加密安全的随机数生成器生成3个骰子
	n1, err := rand.Int(rand.Reader, big.NewInt(6))
	if err != nil {
		return 0, 0, 0, err
	}

	n2, err := rand.Int(rand.Reader, big.NewInt(6))
	if err != nil {
		return 0, 0, 0, err
	}

	n3, err := rand.Int(rand.Reader, big.NewInt(6))
	if err != nil {
		return 0, 0, 0, err
	}

	return int(n1.Int64()) + 1, int(n2.Int64()) + 1, int(n3.Int64()) + 1, nil
}

// GetGameStats 获取游戏统计信息
func (m *Manager) GetGameStats() map[string]interface{} {
	// 这里可以添加统计信息的实现
	return map[string]interface{}{
		"total_games":      0,
		"total_volume":     0,
		"total_commission": 0,
	}
}

// setGameTimeout 设置游戏超时定时器
func (m *Manager) setGameTimeout(gameID string, duration time.Duration) {
	m.timerMutex.Lock()
	defer m.timerMutex.Unlock()

	// 如果已存在定时器，先取消
	if timer, exists := m.gameTimers[gameID]; exists {
		timer.Stop()
	}

	// 创建新的定时器
	timer := time.AfterFunc(duration, func() {
		m.expireGame(gameID)
	})

	m.gameTimers[gameID] = timer
}

// cancelGameTimeout 取消游戏超时定时器
func (m *Manager) cancelGameTimeout(gameID string) {
	m.timerMutex.Lock()
	defer m.timerMutex.Unlock()

	if timer, exists := m.gameTimers[gameID]; exists {
		timer.Stop()
		delete(m.gameTimers, gameID)
	}
}

// expireGame 处理游戏超时
func (m *Manager) expireGame(gameID string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 获取游戏信息
	game, err := m.db.GetGame(gameID)
	if err != nil || game == nil {
		return
	}

	// 只处理等待中的游戏
	if game.Status != models.GameStatusWaiting {
		return
	}

	// 更新游戏状态为过期
	if err := m.db.UpdateGameStatus(gameID, models.GameStatusExpired); err != nil {
		return
	}

	// 退还玩家1的下注金额
	player1, err := m.db.GetUser(game.Player1ID)
	if err != nil {
		return
	}

	newBalance := player1.Balance + game.BetAmount
	if err := m.db.UpdateUserBalance(game.Player1ID, newBalance); err != nil {
		return
	}

	// 记录退款交易
	tx := &models.Transaction{
		ID:          utils.GenerateTransactionID(),
		UserID:      game.Player1ID,
		GameID:      &gameID,
		Type:        models.TransactionTypeRefund,
		Amount:      game.BetAmount,
		Balance:     newBalance,
		Description: fmt.Sprintf("游戏超时退款 %s", gameID),
	}
	m.db.CreateTransaction(tx)

	// 发送超时通知
	if m.onGameExpired != nil {
		m.onGameExpired(gameID, game.ChatID)
	}

	// 清理定时器
	m.cancelGameTimeout(gameID)
}

// startCleanupTask 启动定期清理过期游戏的后台任务
func (m *Manager) startCleanupTask() {
	ticker := time.NewTicker(5 * time.Minute) // 每5分钟检查一次
	defer ticker.Stop()

	for range ticker.C {
		m.cleanupExpiredGames()
	}
}

// cleanupExpiredGames 清理过期的等待中游戏
func (m *Manager) cleanupExpiredGames() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 由于我们使用定时器机制，这个方法主要用于清理可能遗漏的定时器
	// 实际的游戏过期处理由 expireGame 方法完成

	// 清理已经不存在的游戏的定时器
	m.timerMutex.Lock()
	for gameID := range m.gameTimers {
		game, err := m.db.GetGame(gameID)
		if err != nil || game == nil || game.Status != models.GameStatusWaiting {
			// 游戏不存在或状态已改变，清理定时器
			if timer, exists := m.gameTimers[gameID]; exists {
				timer.Stop()
				delete(m.gameTimers, gameID)
			}
		}
	}
	m.timerMutex.Unlock()
}
