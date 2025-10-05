package cache

import (
	"fmt"
	"log"
	"sync"
	"time"

	"telegram-dice-bot/internal/models"
)

// BalanceCache 实时余额缓存系统
type BalanceCache struct {
	cache       sync.Map // userID -> *CachedBalance
	db          DatabaseInterface
	mutex       sync.RWMutex
	subscribers sync.Map // userID -> []chan BalanceUpdate
}

// CachedBalance 缓存的余额信息
type CachedBalance struct {
	UserID    int64
	Balance   int64
	UpdatedAt time.Time
	Version   int64 // 版本号，用于乐观锁
}

// BalanceUpdate 余额更新通知
type BalanceUpdate struct {
	UserID    int64
	OldBalance int64
	NewBalance int64
	Timestamp time.Time
	Source    string // 更新来源：game, recharge, withdraw等
}

// DatabaseInterface 数据库接口
type DatabaseInterface interface {
	GetUser(userID int64) (*models.User, error)
	UpdateUserBalance(userID int64, newBalance int64) error
}

// NewBalanceCache 创建新的余额缓存
func NewBalanceCache(db DatabaseInterface) *BalanceCache {
	cache := &BalanceCache{
		db: db,
	}
	
	// 启动定期一致性检查
	go cache.startConsistencyCheck()
	
	log.Printf("✅ 实时余额缓存系统已启动")
	return cache
}

// GetBalance 获取用户余额（优先从缓存）
func (bc *BalanceCache) GetBalance(userID int64) (int64, error) {
	// 先尝试从缓存获取
	if cached, exists := bc.cache.Load(userID); exists {
		cachedBalance := cached.(*CachedBalance)
		
		// 检查缓存是否过期（5分钟）
		if time.Since(cachedBalance.UpdatedAt) < 5*time.Minute {
			return cachedBalance.Balance, nil
		}
	}
	
	// 缓存未命中或过期，从数据库获取
	return bc.refreshBalance(userID)
}

// UpdateBalance 更新用户余额（实时更新缓存）
func (bc *BalanceCache) UpdateBalance(userID int64, newBalance int64, source string) error {
	bc.mutex.Lock()
	defer bc.mutex.Unlock()
	
	// 获取旧余额
	oldBalance := int64(0)
	if cached, exists := bc.cache.Load(userID); exists {
		oldBalance = cached.(*CachedBalance).Balance
	}
	
	// 更新数据库
	if err := bc.db.UpdateUserBalance(userID, newBalance); err != nil {
		return fmt.Errorf("数据库更新失败: %v", err)
	}
	
	// 立即更新缓存
	cachedBalance := &CachedBalance{
		UserID:    userID,
		Balance:   newBalance,
		UpdatedAt: time.Now(),
		Version:   time.Now().UnixNano(), // 使用纳秒时间戳作为版本号
	}
	bc.cache.Store(userID, cachedBalance)
	
	// 发送余额变更通知
	update := BalanceUpdate{
		UserID:     userID,
		OldBalance: oldBalance,
		NewBalance: newBalance,
		Timestamp:  time.Now(),
		Source:     source,
	}
	bc.notifySubscribers(userID, update)
	
	log.Printf("💰 余额实时更新: 用户%d %s %d->%d (来源:%s)", 
		userID, 
		func() string {
			if newBalance > oldBalance {
				return "增加"
			}
			return "减少"
		}(), 
		oldBalance, newBalance, source)
	
	return nil
}

// refreshBalance 从数据库刷新余额到缓存
func (bc *BalanceCache) refreshBalance(userID int64) (int64, error) {
	user, err := bc.db.GetUser(userID)
	if err != nil {
		return 0, fmt.Errorf("获取用户信息失败: %v", err)
	}
	
	if user == nil {
		return 0, nil // 用户不存在，返回0余额
	}
	
	// 更新缓存
	cachedBalance := &CachedBalance{
		UserID:    userID,
		Balance:   user.Balance,
		UpdatedAt: time.Now(),
		Version:   time.Now().UnixNano(),
	}
	bc.cache.Store(userID, cachedBalance)
	
	return user.Balance, nil
}

// SubscribeBalanceUpdates 订阅余额更新通知
func (bc *BalanceCache) SubscribeBalanceUpdates(userID int64) <-chan BalanceUpdate {
	ch := make(chan BalanceUpdate, 10) // 缓冲通道
	
	// 获取或创建订阅者列表
	subscribers, _ := bc.subscribers.LoadOrStore(userID, make([]chan BalanceUpdate, 0))
	subscriberList := subscribers.([]chan BalanceUpdate)
	subscriberList = append(subscriberList, ch)
	bc.subscribers.Store(userID, subscriberList)
	
	return ch
}

// notifySubscribers 通知订阅者余额变更
func (bc *BalanceCache) notifySubscribers(userID int64, update BalanceUpdate) {
	if subscribers, exists := bc.subscribers.Load(userID); exists {
		subscriberList := subscribers.([]chan BalanceUpdate)
		
		// 异步通知所有订阅者
		go func() {
			for _, ch := range subscriberList {
				select {
				case ch <- update:
				case <-time.After(100 * time.Millisecond):
					// 超时则跳过，避免阻塞
				}
			}
		}()
	}
}

// startConsistencyCheck 启动定期一致性检查
func (bc *BalanceCache) startConsistencyCheck() {
	ticker := time.NewTicker(5 * time.Minute) // 每5分钟检查一次
	defer ticker.Stop()
	
	for range ticker.C {
		bc.performConsistencyCheck()
	}
}

// performConsistencyCheck 执行一致性检查
func (bc *BalanceCache) performConsistencyCheck() {
	checkCount := 0
	inconsistentCount := 0
	
	bc.cache.Range(func(key, value interface{}) bool {
		userID := key.(int64)
		cached := value.(*CachedBalance)
		
		// 从数据库获取最新余额
		user, err := bc.db.GetUser(userID)
		if err != nil {
			log.Printf("⚠️ 一致性检查失败: 用户%d, 错误: %v", userID, err)
			return true
		}
		
		if user == nil {
			// 用户不存在，清除缓存
			bc.cache.Delete(userID)
			return true
		}
		
		checkCount++
		
		// 检查余额是否一致
		if cached.Balance != user.Balance {
			inconsistentCount++
			log.Printf("🔄 发现余额不一致: 用户%d, 缓存:%d, 数据库:%d, 强制刷新", 
				userID, cached.Balance, user.Balance)
			
			// 强制刷新缓存
			bc.refreshBalance(userID)
		}
		
		return true
	})
	
	if checkCount > 0 {
		log.Printf("🔍 余额一致性检查完成: 检查%d个用户, 发现%d个不一致", 
			checkCount, inconsistentCount)
	}
}

// GetCacheStats 获取缓存统计信息
func (bc *BalanceCache) GetCacheStats() map[string]interface{} {
	cacheSize := 0
	subscriberCount := 0
	
	bc.cache.Range(func(key, value interface{}) bool {
		cacheSize++
		return true
	})
	
	bc.subscribers.Range(func(key, value interface{}) bool {
		subscriberList := value.([]chan BalanceUpdate)
		subscriberCount += len(subscriberList)
		return true
	})
	
	return map[string]interface{}{
		"cache_size":       cacheSize,
		"subscriber_count": subscriberCount,
		"system_status":    "active",
	}
}

// ClearUserCache 清除指定用户的缓存
func (bc *BalanceCache) ClearUserCache(userID int64) {
	bc.cache.Delete(userID)
	bc.subscribers.Delete(userID)
}

// InvalidateCache 使缓存失效（用于测试或紧急情况）
func (bc *BalanceCache) InvalidateCache() {
	bc.cache.Range(func(key, value interface{}) bool {
		bc.cache.Delete(key)
		return true
	})
	log.Printf("🔄 余额缓存已全部清除")
}