package models

import (
	"time"
)

// User 用户模型
type User struct {
	ID        int64     `json:"id" db:"id"`
	Username  string    `json:"username" db:"username"`
	FirstName string    `json:"first_name" db:"first_name"`
	LastName  string    `json:"last_name" db:"last_name"`
	Balance   int64     `json:"balance" db:"balance"` // 余额（以最小单位计算）
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// Game 游戏模型
type Game struct {
	ID        string `json:"id" db:"id"`
	Player1ID int64  `json:"player1_id" db:"player1_id"`
	Player2ID *int64 `json:"player2_id" db:"player2_id"`
	BetAmount int64  `json:"bet_amount" db:"bet_amount"`
	Status    string `json:"status" db:"status"` // waiting, playing, finished, cancelled
	// 玩家1的3个骰子
	Player1Dice1 *int `json:"player1_dice1" db:"player1_dice1"`
	Player1Dice2 *int `json:"player1_dice2" db:"player1_dice2"`
	Player1Dice3 *int `json:"player1_dice3" db:"player1_dice3"`
	// 玩家2的3个骰子
	Player2Dice1 *int      `json:"player2_dice1" db:"player2_dice1"`
	Player2Dice2 *int      `json:"player2_dice2" db:"player2_dice2"`
	Player2Dice3 *int      `json:"player2_dice3" db:"player2_dice3"`
	WinnerID     *int64    `json:"winner_id" db:"winner_id"`
	Commission   int64     `json:"commission" db:"commission"` // 平台抽水
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
	ChatID       int64     `json:"chat_id" db:"chat_id"` // 群组ID
}

// Transaction 交易记录
type Transaction struct {
	ID          string    `json:"id" db:"id"`
	UserID      int64     `json:"user_id" db:"user_id"`
	GameID      *string   `json:"game_id" db:"game_id"`
	Type        string    `json:"type" db:"type"` // bet, win, commission, deposit, withdraw
	Amount      int64     `json:"amount" db:"amount"`
	Balance     int64     `json:"balance" db:"balance"` // 交易后余额
	Description string    `json:"description" db:"description"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// GameStatus 游戏状态常量
const (
	GameStatusWaiting   = "waiting"
	GameStatusPlaying   = "playing"
	GameStatusFinished  = "finished"
	GameStatusCancelled = "cancelled"
	GameStatusExpired   = "expired"
)

// TransactionType 交易类型常量
const (
	TransactionTypeBet        = "bet"
	TransactionTypeWin        = "win"
	TransactionTypeCommission = "commission"
	TransactionTypeDeposit    = "deposit"
	TransactionTypeWithdraw   = "withdraw"
	TransactionTypeRefund     = "refund"
)
