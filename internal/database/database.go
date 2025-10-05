package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"telegram-dice-bot/internal/models"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn *sql.DB
}

func Init(databaseURL string) (*DB, error) {
	// 优化SQLite连接参数
	conn, err := sql.Open("sqlite3", databaseURL+"?cache=shared&mode=rwc&_journal_mode=WAL&_synchronous=NORMAL&_cache_size=10000&_temp_store=memory")
	if err != nil {
		return nil, err
	}

	// 设置连接池参数
	conn.SetMaxOpenConns(25)
	conn.SetMaxIdleConns(25)
	conn.SetConnMaxLifetime(5 * time.Minute)

	db := &DB{conn: conn}

	if err := db.createTables(); err != nil {
		return nil, err
	}

	// 创建索引以提升查询性能
	if err := db.createIndexes(); err != nil {
		return nil, err
	}

	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) createTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY,
			username TEXT,
			first_name TEXT,
			last_name TEXT,
			balance INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS games (
			id TEXT PRIMARY KEY,
			player1_id INTEGER NOT NULL,
			player2_id INTEGER,
			bet_amount INTEGER NOT NULL,
			status TEXT DEFAULT 'waiting',
			player1_dice1 INTEGER,
			player1_dice2 INTEGER,
			player1_dice3 INTEGER,
			player2_dice1 INTEGER,
			player2_dice2 INTEGER,
			player2_dice3 INTEGER,
			winner_id INTEGER,
			commission INTEGER DEFAULT 0,
			chat_id INTEGER NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (player1_id) REFERENCES users(id),
			FOREIGN KEY (player2_id) REFERENCES users(id),
			FOREIGN KEY (winner_id) REFERENCES users(id)
		)`,
		`CREATE TABLE IF NOT EXISTS transactions (
			id TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL,
			game_id TEXT,
			type TEXT NOT NULL,
			amount INTEGER NOT NULL,
			balance INTEGER NOT NULL,
			description TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id),
			FOREIGN KEY (game_id) REFERENCES games(id)
		)`,
	}

	for _, query := range queries {
		if _, err := db.conn.Exec(query); err != nil {
			return err
		}
	}

	return nil
}

// 创建索引以提升查询性能
func (db *DB) createIndexes() error {
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_games_status_chat ON games(status, chat_id)`,
		`CREATE INDEX IF NOT EXISTS idx_games_player1 ON games(player1_id)`,
		`CREATE INDEX IF NOT EXISTS idx_games_player2 ON games(player2_id)`,
		`CREATE INDEX IF NOT EXISTS idx_games_created_at ON games(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_transactions_user ON transactions(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_transactions_game ON transactions(game_id)`,
		`CREATE INDEX IF NOT EXISTS idx_transactions_type ON transactions(type)`,
		`CREATE INDEX IF NOT EXISTS idx_users_username ON users(username)`,
	}

	for _, index := range indexes {
		if _, err := db.conn.Exec(index); err != nil {
			return err
		}
	}

	return nil
}

// User operations
func (db *DB) GetUser(userID int64) (*models.User, error) {
	user := &models.User{}
	query := `SELECT id, username, first_name, last_name, balance, created_at, updated_at 
			  FROM users WHERE id = ?`

	err := db.conn.QueryRow(query, userID).Scan(
		&user.ID, &user.Username, &user.FirstName, &user.LastName,
		&user.Balance, &user.CreatedAt, &user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	return user, err
}

func (db *DB) CreateUser(user *models.User) error {
	query := `INSERT INTO users (id, username, first_name, last_name, balance, created_at, updated_at)
			  VALUES (?, ?, ?, ?, ?, ?, ?)`

	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now

	_, err := db.conn.Exec(query, user.ID, user.Username, user.FirstName,
		user.LastName, user.Balance, user.CreatedAt, user.UpdatedAt)

	return err
}

// Transaction support methods
func (db *DB) BeginTx() (*sql.Tx, error) {
	return db.conn.Begin()
}

// CreateGameWithTransaction 在事务中创建游戏并扣除余额
func (db *DB) CreateGameWithTransaction(game *models.Game, userID int64, newBalance int64, transaction *models.Transaction) error {
	tx, err := db.BeginTx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. 再次验证用户当前余额（防止并发问题）
	var currentBalance int64
	query := `SELECT balance FROM users WHERE id = ?`
	err = tx.QueryRow(query, userID).Scan(&currentBalance)
	if err != nil {
		return fmt.Errorf("获取用户当前余额失败: %v", err)
	}

	// 验证余额是否足够
	if currentBalance < game.BetAmount {
		return fmt.Errorf("余额不足，请存款后再试。当前余额: %d，需要: %d", currentBalance, game.BetAmount)
	}

	// 重新计算新余额
	calculatedNewBalance := currentBalance - game.BetAmount
	if calculatedNewBalance < 0 {
		return fmt.Errorf("余额不足，请存款后再试")
	}

	// 更新交易记录中的余额
	transaction.Balance = calculatedNewBalance

	// 2. 扣除用户余额
	if err := db.updateUserBalanceInTx(tx, userID, calculatedNewBalance); err != nil {
		return err
	}

	// 3. 创建游戏
	if err := db.createGameInTx(tx, game); err != nil {
		return err
	}

	// 4. 创建交易记录
	if err := db.createTransactionInTx(tx, transaction); err != nil {
		return err
	}

	return tx.Commit()
}

// JoinGameWithTransaction 在事务中加入游戏并扣除余额
func (db *DB) JoinGameWithTransaction(gameID string, player2ID int64, newBalance int64, transaction *models.Transaction) error {
	tx, err := db.BeginTx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. 再次验证用户当前余额（防止并发问题）
	var currentBalance int64
	var betAmount int64
	query := `SELECT balance FROM users WHERE id = ?`
	err = tx.QueryRow(query, player2ID).Scan(&currentBalance)
	if err != nil {
		return fmt.Errorf("获取用户当前余额失败: %v", err)
	}

	// 获取游戏的下注金额
	query = `SELECT bet_amount FROM games WHERE id = ?`
	err = tx.QueryRow(query, gameID).Scan(&betAmount)
	if err != nil {
		return fmt.Errorf("获取游戏下注金额失败: %v", err)
	}

	// 验证余额是否足够
	if currentBalance < betAmount {
		return fmt.Errorf("余额不足，请存款后再试。当前余额: %d，需要: %d", currentBalance, betAmount)
	}

	// 重新计算新余额
	calculatedNewBalance := currentBalance - betAmount
	if calculatedNewBalance < 0 {
		return fmt.Errorf("余额不足，请存款后再试")
	}

	// 更新交易记录中的余额
	transaction.Balance = calculatedNewBalance

	// 2. 扣除用户余额
	if err := db.updateUserBalanceInTx(tx, player2ID, calculatedNewBalance); err != nil {
		return err
	}

	// 3. 更新游戏状态
	if err := db.updateGamePlayer2InTx(tx, gameID, player2ID); err != nil {
		return err
	}

	// 3. 创建交易记录
	if err := db.createTransactionInTx(tx, transaction); err != nil {
		return err
	}

	return tx.Commit()
}

// Helper methods for transaction operations
func (db *DB) updateUserBalanceInTx(tx *sql.Tx, userID int64, newBalance int64) error {
	if newBalance < 0 {
		return fmt.Errorf("余额不能为负数")
	}

	query := `UPDATE users SET balance = ?, updated_at = ? WHERE id = ?`
	result, err := tx.Exec(query, newBalance, time.Now(), userID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("用户不存在")
	}

	return nil
}

func (db *DB) createGameInTx(tx *sql.Tx, game *models.Game) error {
	query := `INSERT INTO games (id, player1_id, bet_amount, status, chat_id, created_at, updated_at)
			  VALUES (?, ?, ?, ?, ?, ?, ?)`

	now := time.Now()
	game.CreatedAt = now
	game.UpdatedAt = now

	_, err := tx.Exec(query, game.ID, game.Player1ID, game.BetAmount,
		game.Status, game.ChatID, game.CreatedAt, game.UpdatedAt)

	return err
}

func (db *DB) updateGamePlayer2InTx(tx *sql.Tx, gameID string, player2ID int64) error {
	query := `UPDATE games SET player2_id = ?, status = ?, updated_at = ? WHERE id = ? AND status = ?`
	result, err := tx.Exec(query, player2ID, models.GameStatusPlaying, time.Now(), gameID, models.GameStatusWaiting)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("游戏不存在或已开始")
	}

	return nil
}

func (db *DB) createTransactionInTx(tx *sql.Tx, transaction *models.Transaction) error {
	query := `INSERT INTO transactions (id, user_id, game_id, type, amount, balance, description, created_at)
			  VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	transaction.CreatedAt = time.Now()

	_, err := tx.Exec(query, transaction.ID, transaction.UserID, transaction.GameID, transaction.Type,
		transaction.Amount, transaction.Balance, transaction.Description, transaction.CreatedAt)

	return err
}

// SettleGameWithTransaction 在事务中结算游戏
func (db *DB) SettleGameWithTransaction(gameID string, winnerID *int64, commission int64, dice1, dice2, dice3, dice4, dice5, dice6 int, winnerNewBalance int64, transactions []*models.Transaction) error {
	tx, err := db.BeginTx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. 更新游戏状态和骰子结果
	query := `UPDATE games SET 
		status = ?, 
		winner_id = ?, 
		commission = ?,
		player1_dice1 = ?, player1_dice2 = ?, player1_dice3 = ?,
		player2_dice1 = ?, player2_dice2 = ?, player2_dice3 = ?,
		updated_at = ?
		WHERE id = ?`

	_, err = tx.Exec(query, models.GameStatusFinished, winnerID, commission,
		dice1, dice2, dice3, dice4, dice5, dice6, time.Now(), gameID)
	if err != nil {
		return err
	}

	// 2. 更新获胜者余额（如果不是平局）
	if winnerID != nil {
		if err := db.updateUserBalanceInTx(tx, *winnerID, winnerNewBalance); err != nil {
			return err
		}
	}

	// 3. 创建交易记录
	for _, transaction := range transactions {
		if err := db.createTransactionInTx(tx, transaction); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// RefundGameWithTransaction 在事务中退还游戏金额
func (db *DB) RefundGameWithTransaction(player1ID int64, player1NewBalance int64, player2ID *int64, player2NewBalance *int64, transactions []*models.Transaction) error {
	tx, err := db.BeginTx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. 退还玩家1余额
	if err := db.updateUserBalanceInTx(tx, player1ID, player1NewBalance); err != nil {
		return err
	}

	// 2. 退还玩家2余额（如果存在）
	if player2ID != nil && player2NewBalance != nil {
		if err := db.updateUserBalanceInTx(tx, *player2ID, *player2NewBalance); err != nil {
			return err
		}
	}

	// 3. 创建退款交易记录
	for _, transaction := range transactions {
		if err := db.createTransactionInTx(tx, transaction); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) UpdateUserBalance(userID int64, newBalance int64) error {
	// 验证余额不能为负数
	if newBalance < 0 {
		return fmt.Errorf("余额不能为负数")
	}

	query := `UPDATE users SET balance = ?, updated_at = ? WHERE id = ?`
	result, err := db.conn.Exec(query, newBalance, time.Now(), userID)
	if err != nil {
		return err
	}

	// 检查是否有行被更新
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("用户不存在")
	}

	return nil
}

// Game operations
func (db *DB) CreateGame(game *models.Game) error {
	query := `INSERT INTO games (id, player1_id, bet_amount, status, chat_id, created_at, updated_at)
			  VALUES (?, ?, ?, ?, ?, ?, ?)`

	now := time.Now()
	game.CreatedAt = now
	game.UpdatedAt = now

	_, err := db.conn.Exec(query, game.ID, game.Player1ID, game.BetAmount,
		game.Status, game.ChatID, game.CreatedAt, game.UpdatedAt)

	return err
}

func (db *DB) GetGame(gameID string) (*models.Game, error) {
	game := &models.Game{}
	query := `SELECT id, player1_id, player2_id, bet_amount, status, player1_dice1, 
			  player1_dice2, player1_dice3, player2_dice1, player2_dice2, player2_dice3,
			  winner_id, commission, chat_id, created_at, updated_at 
			  FROM games WHERE id = ?`

	err := db.conn.QueryRow(query, gameID).Scan(
		&game.ID, &game.Player1ID, &game.Player2ID, &game.BetAmount,
		&game.Status, &game.Player1Dice1, &game.Player1Dice2, &game.Player1Dice3,
		&game.Player2Dice1, &game.Player2Dice2, &game.Player2Dice3, &game.WinnerID,
		&game.Commission, &game.ChatID, &game.CreatedAt, &game.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	return game, err
}

func (db *DB) UpdateGame(game *models.Game) error {
	query := `UPDATE games SET player2_id = ?, status = ?, player1_dice1 = ?, 
			  player1_dice2 = ?, player1_dice3 = ?, player2_dice1 = ?, 
			  player2_dice2 = ?, player2_dice3 = ?, winner_id = ?, commission = ?, updated_at = ? 
			  WHERE id = ?`

	game.UpdatedAt = time.Now()

	_, err := db.conn.Exec(query, game.Player2ID, game.Status, game.Player1Dice1,
		game.Player1Dice2, game.Player1Dice3, game.Player2Dice1, game.Player2Dice2,
		game.Player2Dice3, game.WinnerID, game.Commission, game.UpdatedAt, game.ID)

	return err
}

// UpdateGameStatus 更新游戏状态
func (db *DB) UpdateGameStatus(gameID, status string) error {
	query := `UPDATE games SET status = ?, updated_at = ? WHERE id = ?`
	_, err := db.conn.Exec(query, status, time.Now(), gameID)
	return err
}

func (db *DB) GetWaitingGames(chatID int64) ([]*models.Game, error) {
	// 只允许群组聊天（负数ChatID）使用此方法
	if chatID >= 0 {
		return nil, fmt.Errorf("GetWaitingGames only accepts group chat IDs (negative values), got: %d", chatID)
	}

	query := `SELECT id, player1_id, player2_id, bet_amount, status, player1_dice1, 
			  player1_dice2, player1_dice3, player2_dice1, player2_dice2, player2_dice3,
			  winner_id, commission, chat_id, created_at, updated_at 
			  FROM games WHERE status = ? AND chat_id = ? ORDER BY created_at ASC`

	rows, err := db.conn.Query(query, models.GameStatusWaiting, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var games []*models.Game
	for rows.Next() {
		game := &models.Game{}
		err := rows.Scan(
			&game.ID, &game.Player1ID, &game.Player2ID, &game.BetAmount,
			&game.Status, &game.Player1Dice1, &game.Player1Dice2, &game.Player1Dice3,
			&game.Player2Dice1, &game.Player2Dice2, &game.Player2Dice3, &game.WinnerID,
			&game.Commission, &game.ChatID, &game.CreatedAt, &game.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		games = append(games, game)
	}

	return games, nil
}

// GetWaitingPrivateGames 获取所有私聊中的等待游戏（已废弃 - 私聊不支持游戏功能）
// DEPRECATED: 此函数已废弃，因为私聊中不再支持游戏功能
func (db *DB) GetWaitingPrivateGames() ([]*models.Game, error) {
	query := `SELECT id, player1_id, player2_id, bet_amount, status, player1_dice1, 
			  player1_dice2, player1_dice3, player2_dice1, player2_dice2, player2_dice3,
			  winner_id, commission, chat_id, created_at, updated_at 
			  FROM games WHERE status = ? AND chat_id > 0 ORDER BY created_at ASC`

	rows, err := db.conn.Query(query, models.GameStatusWaiting)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var games []*models.Game
	for rows.Next() {
		game := &models.Game{}
		err := rows.Scan(
			&game.ID, &game.Player1ID, &game.Player2ID, &game.BetAmount,
			&game.Status, &game.Player1Dice1, &game.Player1Dice2, &game.Player1Dice3,
			&game.Player2Dice1, &game.Player2Dice2, &game.Player2Dice3, &game.WinnerID,
			&game.Commission, &game.ChatID, &game.CreatedAt, &game.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		games = append(games, game)
	}

	return games, nil
}

// Transaction operations
func (db *DB) CreateTransaction(tx *models.Transaction) error {
	query := `INSERT INTO transactions (id, user_id, game_id, type, amount, balance, description, created_at)
			  VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	tx.CreatedAt = time.Now()

	_, err := db.conn.Exec(query, tx.ID, tx.UserID, tx.GameID, tx.Type,
		tx.Amount, tx.Balance, tx.Description, tx.CreatedAt)

	return err
}

// Admin backend methods
func (db *DB) GetUsersWithPagination(offset, limit int) ([]*models.User, error) {
	query := `SELECT id, username, first_name, last_name, balance, created_at, updated_at 
			  FROM users ORDER BY created_at DESC LIMIT ? OFFSET ?`

	rows, err := db.conn.Query(query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		user := &models.User{}
		err := rows.Scan(&user.ID, &user.Username, &user.FirstName, &user.LastName,
			&user.Balance, &user.CreatedAt, &user.UpdatedAt)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}

	return users, nil
}

func (db *DB) GetTotalUsersCount() (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM users`
	err := db.conn.QueryRow(query).Scan(&count)
	return count, err
}

// GetUsersWithFilters 根据筛选条件获取用户列表
func (db *DB) GetUsersWithFilters(offset, limit int, search, status, sortBy string) ([]*models.User, error) {
	query := `SELECT id, username, first_name, last_name, balance, created_at, updated_at 
			  FROM users WHERE 1=1`
	args := []interface{}{}

	// 添加搜索条件
	if search != "" {
		query += ` AND (CAST(id AS TEXT) LIKE ? OR username LIKE ?)`
		searchPattern := "%" + search + "%"
		args = append(args, searchPattern, searchPattern)
	}

	// 添加排序
	switch sortBy {
	case "balance":
		query += ` ORDER BY balance DESC`
	case "last_active":
		query += ` ORDER BY updated_at DESC`
	default:
		query += ` ORDER BY created_at DESC`
	}

	query += ` LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		user := &models.User{}
		err := rows.Scan(&user.ID, &user.Username, &user.FirstName, &user.LastName,
			&user.Balance, &user.CreatedAt, &user.UpdatedAt)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}

	return users, nil
}

// GetTotalUsersCountWithFilters 根据筛选条件获取用户总数
func (db *DB) GetTotalUsersCountWithFilters(search, status string) (int, error) {
	query := `SELECT COUNT(*) FROM users WHERE 1=1`
	args := []interface{}{}

	// 添加搜索条件
	if search != "" {
		query += ` AND (CAST(id AS TEXT) LIKE ? OR username LIKE ?)`
		searchPattern := "%" + search + "%"
		args = append(args, searchPattern, searchPattern)
	}

	var count int
	err := db.conn.QueryRow(query, args...).Scan(&count)
	return count, err
}

// UpdateUserInfo 更新用户信息
func (db *DB) UpdateUserInfo(userID int64, username string, balance int64) error {
	query := `UPDATE users SET username = ?, balance = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := db.conn.Exec(query, username, balance, userID)
	return err
}

func (db *DB) GetGamesWithPagination(offset, limit int) ([]*models.Game, error) {
	query := `SELECT id, player1_id, player2_id, bet_amount, status, player1_dice1, 
			  player1_dice2, player1_dice3, player2_dice1, player2_dice2, player2_dice3,
			  winner_id, commission, chat_id, created_at, updated_at 
			  FROM games ORDER BY created_at DESC LIMIT ? OFFSET ?`

	rows, err := db.conn.Query(query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var games []*models.Game
	for rows.Next() {
		game := &models.Game{}
		err := rows.Scan(&game.ID, &game.Player1ID, &game.Player2ID, &game.BetAmount,
			&game.Status, &game.Player1Dice1, &game.Player1Dice2, &game.Player1Dice3,
			&game.Player2Dice1, &game.Player2Dice2, &game.Player2Dice3, &game.WinnerID,
			&game.Commission, &game.ChatID, &game.CreatedAt, &game.UpdatedAt)
		if err != nil {
			return nil, err
		}
		games = append(games, game)
	}

	return games, nil
}

// GetUserGameHistory 获取用户游戏历史记录
func (db *DB) GetUserGameHistory(userID int64, limit int) ([]*models.Game, error) {
	query := `SELECT id, player1_id, player2_id, bet_amount, status, player1_dice1, 
			  player1_dice2, player1_dice3, player2_dice1, player2_dice2, player2_dice3,
			  winner_id, commission, chat_id, created_at, updated_at 
			  FROM games WHERE (player1_id = ? OR player2_id = ?) 
			  ORDER BY created_at DESC LIMIT ?`

	rows, err := db.conn.Query(query, userID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var games []*models.Game
	for rows.Next() {
		game := &models.Game{}
		err := rows.Scan(&game.ID, &game.Player1ID, &game.Player2ID, &game.BetAmount,
			&game.Status, &game.Player1Dice1, &game.Player1Dice2, &game.Player1Dice3,
			&game.Player2Dice1, &game.Player2Dice2, &game.Player2Dice3, &game.WinnerID,
			&game.Commission, &game.ChatID, &game.CreatedAt, &game.UpdatedAt)
		if err != nil {
			return nil, err
		}
		games = append(games, game)
	}

	return games, nil
}

// DeleteOldUserGames 删除用户的旧游戏记录，保留最新的keepCount条
func (db *DB) DeleteOldUserGames(userID int64, keepCount int) error {
	// 获取需要保留的游戏ID
	query := `SELECT id FROM games WHERE (player1_id = ? OR player2_id = ?) 
			  ORDER BY created_at DESC LIMIT ?`
	
	rows, err := db.conn.Query(query, userID, userID, keepCount)
	if err != nil {
		return err
	}
	defer rows.Close()

	var keepIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		keepIDs = append(keepIDs, id)
	}

	// 如果没有需要保留的记录，直接返回
	if len(keepIDs) == 0 {
		return nil
	}

	// 构建删除查询，删除不在保留列表中的记录
	placeholders := make([]string, len(keepIDs))
	args := []interface{}{userID, userID}
	for i, id := range keepIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}

	deleteQuery := fmt.Sprintf(`DELETE FROM games 
		WHERE (player1_id = ? OR player2_id = ?) 
		AND id NOT IN (%s)`, strings.Join(placeholders, ","))

	_, err = db.conn.Exec(deleteQuery, args...)
	return err
}

func (db *DB) GetRechargesWithPagination(offset, limit int) ([]*models.Transaction, error) {
	query := `SELECT id, user_id, game_id, type, amount, balance, description, created_at 
			  FROM transactions WHERE type IN ('deposit', 'withdraw') 
			  ORDER BY created_at DESC LIMIT ? OFFSET ?`

	rows, err := db.conn.Query(query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transactions []*models.Transaction
	for rows.Next() {
		tx := &models.Transaction{}
		err := rows.Scan(&tx.ID, &tx.UserID, &tx.GameID, &tx.Type,
			&tx.Amount, &tx.Balance, &tx.Description, &tx.CreatedAt)
		if err != nil {
			return nil, err
		}
		transactions = append(transactions, tx)
	}

	return transactions, nil
}

func (db *DB) GetTotalRechargesCount() (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM transactions WHERE type IN ('deposit', 'withdraw')`
	err := db.conn.QueryRow(query).Scan(&count)
	return count, err
}

func (db *DB) DeleteUser(userID int64) error {
	tx, err := db.BeginTx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 删除用户的交易记录
	_, err = tx.Exec("DELETE FROM transactions WHERE user_id = ?", userID)
	if err != nil {
		return err
	}

	// 删除用户参与的游戏
	_, err = tx.Exec("DELETE FROM games WHERE player1_id = ? OR player2_id = ?", userID, userID)
	if err != nil {
		return err
	}

	// 删除用户
	_, err = tx.Exec("DELETE FROM users WHERE id = ?", userID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (db *DB) GetTotalRechargeAmount() (int64, error) {
	var amount sql.NullInt64
	query := `SELECT SUM(amount) FROM transactions WHERE type = 'deposit'`
	err := db.conn.QueryRow(query).Scan(&amount)
	if err != nil {
		return 0, err
	}
	if !amount.Valid {
		return 0, nil
	}
	return amount.Int64, nil
}

func (db *DB) GetActiveUsersCount() (int, error) {
	var count int
	query := `SELECT COUNT(DISTINCT user_id) FROM transactions WHERE created_at >= datetime('now', '-7 days')`
	err := db.conn.QueryRow(query).Scan(&count)
	return count, err
}

func (db *DB) GetTodayGamesCount() (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM games WHERE created_at >= date('now')`
	err := db.conn.QueryRow(query).Scan(&count)
	return count, err
}

// HasPlayingGames 检查指定聊天是否有正在进行的游戏
func (db *DB) HasPlayingGames(chatID int64) (bool, error) {
	var count int
	query := `SELECT COUNT(*) FROM games WHERE status = ? AND chat_id = ?`
	err := db.conn.QueryRow(query, models.GameStatusPlaying, chatID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// HasActivelyPlayingGames 检查指定聊天是否有真正在进行中的游戏（已经开始掷骰子）
func (db *DB) HasActivelyPlayingGames(chatID int64) (bool, error) {
	var count int
	// 只有当游戏状态为playing且已经有骰子结果时，才认为游戏真正开始
	query := `SELECT COUNT(*) FROM games WHERE status = ? AND chat_id = ? AND 
			  (player1_dice1 IS NOT NULL OR player2_dice1 IS NOT NULL)`
	err := db.conn.QueryRow(query, models.GameStatusPlaying, chatID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
