package utils

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"time"
)

// GenerateGameID 生成游戏ID
func GenerateGameID() string {
	timestamp := time.Now().Unix()
	randomNum, _ := rand.Int(rand.Reader, big.NewInt(10000))
	return fmt.Sprintf("GAME%d%04d", timestamp, randomNum.Int64())
}

// GenerateTransactionID 生成交易ID
func GenerateTransactionID() string {
	timestamp := time.Now().UnixNano()
	randomNum, _ := rand.Int(rand.Reader, big.NewInt(10000))
	return fmt.Sprintf("TX%d%04d", timestamp, randomNum.Int64())
}

// FormatBalance 格式化余额显示
func FormatBalance(balance int64) string {
	return fmt.Sprintf("%d", balance)
}

// ValidateBetAmount 验证下注金额
func ValidateBetAmount(amount int64, minBet, maxBet int64) error {
	if amount < minBet {
		return fmt.Errorf("最小下注金额为 %d", minBet)
	}
	if amount > maxBet {
		return fmt.Errorf("最大下注金额为 %d", maxBet)
	}
	return nil
}

// CalculateCommission 计算手续费
func CalculateCommission(amount int64, rate float64) int64 {
	return int64(float64(amount) * rate)
}

// SecureRandomInt 生成安全的随机整数
func SecureRandomInt(max int64) (int64, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(max))
	if err != nil {
		return 0, err
	}
	return n.Int64(), nil
}
