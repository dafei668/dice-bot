package database

import (
	"database/sql"
	"fmt"
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

	// 1. 扣除用户余额
	if err := db.updateUserBalanceInTx(tx, userID, newBalance); err != nil {
		return err
	}

	// 2. 创建游戏
	if err := db.createGameInTx(tx, game); err != nil {
		return err
	}

	// 3. 创建交易记录
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

	// 1. 扣除用户余额
	if err := db.updateUserBalanceInTx(tx, player2ID, newBalance); err != nil {
		return err
	}

	// 2. 更新游戏状态
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

// Transaction operations
func (db *DB) CreateTransaction(tx *models.Transaction) error {
	query := `INSERT INTO transactions (id, user_id, game_id, type, amount, balance, description, created_at)
			  VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	tx.CreatedAt = time.Now()

	_, err := db.conn.Exec(query, tx.ID, tx.UserID, tx.GameID, tx.Type,
		tx.Amount, tx.Balance, tx.Description, tx.CreatedAt)

	return err
}
