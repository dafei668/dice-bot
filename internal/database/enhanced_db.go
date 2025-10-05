package database

import (
	"fmt"
	"time"

	"telegram-dice-bot/internal/models"
)

// ExpireGameWithTransaction 在事务中处理游戏超时
func (db *DB) ExpireGameWithTransaction(gameID string, playerID int64, newBalance int64, transaction *models.Transaction) error {
	tx, err := db.BeginTx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. 更新游戏状态为过期
	query := `UPDATE games SET status = ?, updated_at = ? WHERE id = ? AND status = ?`
	result, err := tx.Exec(query, models.GameStatusExpired, time.Now(), gameID, models.GameStatusWaiting)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("游戏不存在或状态不正确")
	}

	// 2. 更新玩家余额
	if err := db.updateUserBalanceInTx(tx, playerID, newBalance); err != nil {
		return err
	}

	// 3. 创建退款交易记录
	if err := db.createTransactionInTx(tx, transaction); err != nil {
		return err
	}

	return tx.Commit()
}

// SettleGameWithTransactionEnhanced 增强的游戏结算方法，支持平局退款
func (db *DB) SettleGameWithTransactionEnhanced(gameID string, winnerID *int64, winAmount int64, commission int64, 
	dice1, dice2, dice3, dice4, dice5, dice6 int, transactions []*models.Transaction) error {
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

	// 2. 处理交易记录和余额更新
	for _, transaction := range transactions {
		// 更新用户余额
		if err := db.updateUserBalanceInTx(tx, transaction.UserID, transaction.Balance); err != nil {
			return err
		}
		
		// 创建交易记录
		if err := db.createTransactionInTx(tx, transaction); err != nil {
			return err
		}
	}

	return tx.Commit()
}