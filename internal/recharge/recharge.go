package recharge

import (
	"bufio"
	"crypto/md5"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"telegram-dice-bot/internal/database"
	"telegram-dice-bot/internal/models"
)

// RechargeManager 充值管理器
type RechargeManager struct {
	db            *database.DB
	usdtAddresses []string
	addressMutex  sync.RWMutex
	addressFile   string
}

// UserRechargeInfo 用户充值信息
type UserRechargeInfo struct {
	UserID          int64     `json:"user_id" db:"user_id"`
	USDTAddress     string    `json:"usdt_address" db:"usdt_address"`
	AddressIndex    int       `json:"address_index" db:"address_index"`
	TotalRecharged  float64   `json:"total_recharged" db:"total_recharged"`
	LastRechargeAt  time.Time `json:"last_recharge_at" db:"last_recharge_at"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
}

// RechargeRecord 充值记录
type RechargeRecord struct {
	ID            int64     `json:"id" db:"id"`
	UserID        int64     `json:"user_id" db:"user_id"`
	USDTAddress   string    `json:"usdt_address" db:"usdt_address"`
	Amount        float64   `json:"amount" db:"amount"`
	TxHash        string    `json:"tx_hash" db:"tx_hash"`
	Status        string    `json:"status" db:"status"` // pending, confirmed, failed
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
	ConfirmedAt   *time.Time `json:"confirmed_at" db:"confirmed_at"`
}

// NewRechargeManager 创建充值管理器
func NewRechargeManager(db *database.DB, addressFile string) (*RechargeManager, error) {
	rm := &RechargeManager{
		db:          db,
		addressFile: addressFile,
	}

	// 加载USDT地址
	if err := rm.loadUSDTAddresses(); err != nil {
		return nil, fmt.Errorf("加载USDT地址失败: %v", err)
	}

	// 初始化数据库表
	if err := rm.initTables(); err != nil {
		return nil, fmt.Errorf("初始化充值表失败: %v", err)
	}

	log.Printf("✅ 充值管理器初始化成功，加载了 %d 个USDT地址", len(rm.usdtAddresses))
	return rm, nil
}

// loadUSDTAddresses 加载USDT地址
func (rm *RechargeManager) loadUSDTAddresses() error {
	file, err := os.Open(rm.addressFile)
	if err != nil {
		return fmt.Errorf("打开地址文件失败: %v", err)
	}
	defer file.Close()

	var addresses []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		address := strings.TrimSpace(scanner.Text())
		if address != "" && strings.HasPrefix(address, "T") && len(address) == 34 {
			addresses = append(addresses, address)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("读取地址文件失败: %v", err)
	}

	if len(addresses) == 0 {
		return fmt.Errorf("没有找到有效的USDT地址")
	}

	rm.addressMutex.Lock()
	rm.usdtAddresses = addresses
	rm.addressMutex.Unlock()

	return nil
}

// initTables 初始化数据库表
func (rm *RechargeManager) initTables() error {
	// 创建用户充值信息表
	createUserRechargeTable := `
	CREATE TABLE IF NOT EXISTS user_recharge_info (
		user_id INTEGER PRIMARY KEY,
		usdt_address TEXT NOT NULL UNIQUE,
		address_index INTEGER NOT NULL UNIQUE,
		total_recharged REAL DEFAULT 0,
		last_recharge_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`

	// 创建充值记录表
	createRechargeRecordTable := `
	CREATE TABLE IF NOT EXISTS recharge_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		usdt_address TEXT NOT NULL,
		amount REAL NOT NULL,
		tx_hash TEXT,
		status TEXT DEFAULT 'pending',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		confirmed_at DATETIME,
		FOREIGN KEY (user_id) REFERENCES users (id)
	)`

	tx, err := rm.db.BeginTx()
	if err != nil {
		return fmt.Errorf("开始事务失败: %v", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(createUserRechargeTable); err != nil {
		return fmt.Errorf("创建用户充值信息表失败: %v", err)
	}

	if _, err := tx.Exec(createRechargeRecordTable); err != nil {
		return fmt.Errorf("创建充值记录表失败: %v", err)
	}

	return tx.Commit()
}

// GetUserRechargeAddress 获取用户的专属充值地址
func (rm *RechargeManager) GetUserRechargeAddress(userID int64) (string, error) {
	// 先检查用户是否已经有分配的地址
	var info UserRechargeInfo
	var lastRechargeStr string
	query := `SELECT user_id, usdt_address, address_index, total_recharged, 
		       COALESCE(last_recharge_at, ''), created_at 
		FROM user_recharge_info WHERE user_id = ?`
	
	// 使用事务来查询
	tx, err := rm.db.BeginTx()
	if err != nil {
		return "", fmt.Errorf("开始事务失败: %v", err)
	}
	defer tx.Rollback()

	err = tx.QueryRow(query, userID).Scan(
		&info.UserID, &info.USDTAddress, &info.AddressIndex,
		&info.TotalRecharged, &lastRechargeStr, &info.CreatedAt)

	if err == nil {
		// 解析最后充值时间
		if lastRechargeStr != "" {
			if t, err := time.Parse("2006-01-02 15:04:05", lastRechargeStr); err == nil {
				info.LastRechargeAt = t
			}
		}
		// 用户已有地址，直接返回
		tx.Commit()
		return info.USDTAddress, nil
	}

	if err != sql.ErrNoRows {
		return "", fmt.Errorf("查询用户充值信息失败: %v", err)
	}

	// 用户没有地址，需要分配新地址
	tx.Commit()
	return rm.assignNewAddress(userID)
}

// assignNewAddress 为用户分配新的充值地址
func (rm *RechargeManager) assignNewAddress(userID int64) (string, error) {
	rm.addressMutex.Lock()
	defer rm.addressMutex.Unlock()

	// 使用用户ID的哈希来确定地址索引，确保同一用户总是得到相同地址
	hash := md5.Sum([]byte(fmt.Sprintf("user_%d", userID)))
	addressIndex := int(hash[0])<<24 + int(hash[1])<<16 + int(hash[2])<<8 + int(hash[3])
	addressIndex = addressIndex % len(rm.usdtAddresses)
	if addressIndex < 0 {
		addressIndex = -addressIndex
	}

	tx, err := rm.db.BeginTx()
	if err != nil {
		return "", fmt.Errorf("开始事务失败: %v", err)
	}
	defer tx.Rollback()

	// 检查该索引是否已被使用
	var existingUserID int64
	err = tx.QueryRow("SELECT user_id FROM user_recharge_info WHERE address_index = ?", addressIndex).Scan(&existingUserID)
	
	if err == nil {
		// 地址已被使用，寻找下一个可用地址
		for i := 0; i < len(rm.usdtAddresses); i++ {
			testIndex := (addressIndex + i) % len(rm.usdtAddresses)
			err = tx.QueryRow("SELECT user_id FROM user_recharge_info WHERE address_index = ?", testIndex).Scan(&existingUserID)
			if err == sql.ErrNoRows {
				addressIndex = testIndex
				break
			}
		}
	}

	if addressIndex >= len(rm.usdtAddresses) {
		return "", fmt.Errorf("没有可用的USDT地址")
	}

	address := rm.usdtAddresses[addressIndex]

	// 保存用户地址分配信息
	_, err = tx.Exec(`
		INSERT INTO user_recharge_info (user_id, usdt_address, address_index, created_at)
		VALUES (?, ?, ?, ?)`,
		userID, address, addressIndex, time.Now())

	if err != nil {
		return "", fmt.Errorf("保存用户地址分配失败: %v", err)
	}

	if err = tx.Commit(); err != nil {
		return "", fmt.Errorf("提交事务失败: %v", err)
	}

	log.Printf("✅ 为用户 %d 分配充值地址: %s (索引: %d)", userID, address, addressIndex)
	return address, nil
}

// GetUserRechargeInfo 获取用户充值信息
func (rm *RechargeManager) GetUserRechargeInfo(userID int64) (*UserRechargeInfo, error) {
	var info UserRechargeInfo
	var lastRechargeStr string
	
	tx, err := rm.db.BeginTx()
	if err != nil {
		return nil, fmt.Errorf("开始事务失败: %v", err)
	}
	defer tx.Rollback()

	query := `SELECT user_id, usdt_address, address_index, total_recharged, 
		       COALESCE(last_recharge_at, ''), created_at 
		FROM user_recharge_info WHERE user_id = ?`
	
	err = tx.QueryRow(query, userID).Scan(
		&info.UserID, &info.USDTAddress, &info.AddressIndex,
		&info.TotalRecharged, &lastRechargeStr, &info.CreatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("用户充值信息不存在")
		}
		return nil, fmt.Errorf("查询用户充值信息失败: %v", err)
	}

	// 解析最后充值时间
	if lastRechargeStr != "" {
		if t, err := time.Parse("2006-01-02 15:04:05", lastRechargeStr); err == nil {
			info.LastRechargeAt = t
		}
	}

	tx.Commit()
	return &info, nil
}

// AddRechargeRecord 添加充值记录
func (rm *RechargeManager) AddRechargeRecord(userID int64, amount float64, txHash string) error {
	// 获取用户的充值地址
	address, err := rm.GetUserRechargeAddress(userID)
	if err != nil {
		return fmt.Errorf("获取用户充值地址失败: %v", err)
	}

	tx, err := rm.db.BeginTx()
	if err != nil {
		return fmt.Errorf("开始事务失败: %v", err)
	}
	defer tx.Rollback()

	// 添加充值记录
	_, err = tx.Exec(`
		INSERT INTO recharge_records (user_id, usdt_address, amount, tx_hash, status, created_at)
		VALUES (?, ?, ?, ?, 'pending', ?)`,
		userID, address, amount, txHash, time.Now())

	if err != nil {
		return fmt.Errorf("添加充值记录失败: %v", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %v", err)
	}

	log.Printf("✅ 添加充值记录: 用户 %d, 金额 %.2f USDT, 交易哈希 %s", userID, amount, txHash)
	return nil
}

// ConfirmRecharge 确认充值
func (rm *RechargeManager) ConfirmRecharge(recordID int64, actualAmount float64) error {
	tx, err := rm.db.BeginTx()
	if err != nil {
		return fmt.Errorf("开始事务失败: %v", err)
	}
	defer tx.Rollback()

	// 获取充值记录
	var record RechargeRecord
	err = tx.QueryRow(`
		SELECT id, user_id, usdt_address, amount, tx_hash, status
		FROM recharge_records WHERE id = ?`, recordID).Scan(
		&record.ID, &record.UserID, &record.USDTAddress,
		&record.Amount, &record.TxHash, &record.Status)

	if err != nil {
		return fmt.Errorf("查询充值记录失败: %v", err)
	}

	if record.Status != "pending" {
		return fmt.Errorf("充值记录状态不是待确认")
	}

	// 更新充值记录状态
	_, err = tx.Exec(`
		UPDATE recharge_records 
		SET status = 'confirmed', confirmed_at = ?, amount = ?
		WHERE id = ?`, time.Now(), actualAmount, recordID)

	if err != nil {
		return fmt.Errorf("更新充值记录失败: %v", err)
	}

	// 计算游戏币数量 (1 USDT = 10 游戏币)
	gameCoins := int64(actualAmount * 10)

	// 更新用户余额
	_, err = tx.Exec(`
		UPDATE users SET balance = balance + ? WHERE id = ?`,
		gameCoins, record.UserID)

	if err != nil {
		return fmt.Errorf("更新用户余额失败: %v", err)
	}

	// 添加交易记录
	_, err = tx.Exec(`
		INSERT INTO transactions (user_id, type, amount, description, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		record.UserID, models.TransactionTypeDeposit, gameCoins,
		fmt.Sprintf("USDT充值确认 %.2f USDT -> %d 游戏币", actualAmount, gameCoins),
		time.Now())

	if err != nil {
		return fmt.Errorf("添加交易记录失败: %v", err)
	}

	// 更新用户充值信息
	_, err = tx.Exec(`
		UPDATE user_recharge_info 
		SET total_recharged = total_recharged + ?, last_recharge_at = ?
		WHERE user_id = ?`, actualAmount, time.Now(), record.UserID)

	if err != nil {
		return fmt.Errorf("更新用户充值信息失败: %v", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %v", err)
	}

	log.Printf("✅ 充值确认成功: 用户 %d, 金额 %.2f USDT, 获得 %d 游戏币", 
		record.UserID, actualAmount, gameCoins)
	return nil
}

// GetRechargeRecords 获取用户充值记录
func (rm *RechargeManager) GetRechargeRecords(userID int64, limit int) ([]RechargeRecord, error) {
	tx, err := rm.db.BeginTx()
	if err != nil {
		return nil, fmt.Errorf("开始事务失败: %v", err)
	}
	defer tx.Rollback()

	rows, err := tx.Query(`
		SELECT id, user_id, usdt_address, amount, COALESCE(tx_hash, ''), 
		       status, created_at, COALESCE(confirmed_at, '')
		FROM recharge_records 
		WHERE user_id = ? 
		ORDER BY created_at DESC 
		LIMIT ?`, userID, limit)

	if err != nil {
		return nil, fmt.Errorf("查询充值记录失败: %v", err)
	}
	defer rows.Close()

	var records []RechargeRecord
	for rows.Next() {
		var record RechargeRecord
		var confirmedAtStr string
		var createdAtStr string

		err := rows.Scan(&record.ID, &record.UserID, &record.USDTAddress,
			&record.Amount, &record.TxHash, &record.Status,
			&createdAtStr, &confirmedAtStr)

		if err != nil {
			return nil, fmt.Errorf("扫描充值记录失败: %v", err)
		}

		// 解析创建时间
		if createdAtStr != "" {
			if t, err := time.Parse("2006-01-02 15:04:05", createdAtStr); err == nil {
				record.CreatedAt = t
			}
		}

		// 解析确认时间
		if confirmedAtStr != "" {
			if t, err := time.Parse("2006-01-02 15:04:05", confirmedAtStr); err == nil {
				record.ConfirmedAt = &t
			}
		}

		records = append(records, record)
	}

	tx.Commit()
	return records, nil
}

// GetAddressCount 获取地址总数
func (rm *RechargeManager) GetAddressCount() int {
	rm.addressMutex.RLock()
	defer rm.addressMutex.RUnlock()
	return len(rm.usdtAddresses)
}

// GetUsedAddressCount 获取已使用地址数量
func (rm *RechargeManager) GetUsedAddressCount() (int, error) {
	tx, err := rm.db.BeginTx()
	if err != nil {
		return 0, fmt.Errorf("开始事务失败: %v", err)
	}
	defer tx.Rollback()

	var count int
	err = tx.QueryRow("SELECT COUNT(*) FROM user_recharge_info").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("查询已使用地址数量失败: %v", err)
	}
	
	tx.Commit()
	return count, nil
}