package cache

import (
	"log"
	"sync"
	"time"

	"telegram-dice-bot/internal/models"
)

// GameHistoryCache 游戏历史缓存系统
type GameHistoryCache struct {
	cache       sync.Map // userID -> *UserGameHistory
	db          GameDatabaseInterface
	maxRecords  int      // 每用户最大记录数
	cacheTTL    time.Duration // 缓存过期时间
}

// UserGameHistory 用户游戏历史
type UserGameHistory struct {
	UserID    int64
	Games     []*models.Game // 最近的游戏记录，按时间倒序
	UpdatedAt time.Time
	mutex     sync.RWMutex
}

// GameDatabaseInterface 游戏数据库接口
type GameDatabaseInterface interface {
	GetUserGameHistory(userID int64, limit int) ([]*models.Game, error)
	DeleteOldUserGames(userID int64, keepCount int) error
	CreateGame(game *models.Game) error
	UpdateGame(game *models.Game) error
}

// NewGameHistoryCache 创建新的游戏历史缓存
func NewGameHistoryCache(db GameDatabaseInterface) *GameHistoryCache {
	cache := &GameHistoryCache{
		db:         db,
		maxRecords: 5,                // 每用户最多保留5局游戏记录
		cacheTTL:   10 * time.Minute, // 缓存10分钟
	}
	
	// 启动定期清理任务
	go cache.startCleanupRoutine()
	
	log.Printf("✅ 游戏历史缓存系统已启动 (每用户最多%d局记录)", cache.maxRecords)
	return cache
}

// GetUserGameHistory 获取用户游戏历史
func (ghc *GameHistoryCache) GetUserGameHistory(userID int64) ([]*models.Game, error) {
	// 尝试从缓存获取
	if cached, exists := ghc.cache.Load(userID); exists {
		history := cached.(*UserGameHistory)
		history.mutex.RLock()
		defer history.mutex.RUnlock()
		
		// 检查缓存是否过期
		if time.Since(history.UpdatedAt) < ghc.cacheTTL {
			// 返回副本，避免并发修改
			games := make([]*models.Game, len(history.Games))
			copy(games, history.Games)
			return games, nil
		}
	}
	
	// 缓存未命中或过期，从数据库获取
	return ghc.refreshUserHistory(userID)
}

// AddGameRecord 添加游戏记录
func (ghc *GameHistoryCache) AddGameRecord(userID int64, game *models.Game) error {
	// 获取或创建用户历史记录
	historyInterface, _ := ghc.cache.LoadOrStore(userID, &UserGameHistory{
		UserID: userID,
		Games:  make([]*models.Game, 0, ghc.maxRecords),
	})
	history := historyInterface.(*UserGameHistory)
	
	history.mutex.Lock()
	defer history.mutex.Unlock()
	
	// 添加新游戏到历史记录开头
	history.Games = append([]*models.Game{game}, history.Games...)
	
	// 限制记录数量
	if len(history.Games) > ghc.maxRecords {
		// 保留最新的记录
		history.Games = history.Games[:ghc.maxRecords]
		
		// 异步删除数据库中的旧记录
		go func() {
			if err := ghc.db.DeleteOldUserGames(userID, ghc.maxRecords); err != nil {
				log.Printf("⚠️ 删除用户%d旧游戏记录失败: %v", userID, err)
			} else {
				log.Printf("🗑️ 已清理用户%d的旧游戏记录，保留最新%d局", userID, ghc.maxRecords)
			}
		}()
	}
	
	history.UpdatedAt = time.Now()
	
	log.Printf("📝 添加游戏记录: 用户%d, 游戏%s, 当前记录数:%d", 
		userID, game.ID, len(history.Games))
	
	return nil
}

// UpdateGameRecord 更新游戏记录
func (ghc *GameHistoryCache) UpdateGameRecord(userID int64, updatedGame *models.Game) error {
	if cached, exists := ghc.cache.Load(userID); exists {
		history := cached.(*UserGameHistory)
		history.mutex.Lock()
		defer history.mutex.Unlock()
		
		// 查找并更新对应的游戏记录
		for i, game := range history.Games {
			if game.ID == updatedGame.ID {
				history.Games[i] = updatedGame
				history.UpdatedAt = time.Now()
				
				log.Printf("🔄 更新游戏记录: 用户%d, 游戏%s", userID, updatedGame.ID)
				return nil
			}
		}
	}
	
	// 如果缓存中没有找到，刷新整个历史记录
	_, err := ghc.refreshUserHistory(userID)
	return err
}

// refreshUserHistory 从数据库刷新用户历史记录
func (ghc *GameHistoryCache) refreshUserHistory(userID int64) ([]*models.Game, error) {
	games, err := ghc.db.GetUserGameHistory(userID, ghc.maxRecords)
	if err != nil {
		return nil, err
	}
	
	// 更新缓存
	history := &UserGameHistory{
		UserID:    userID,
		Games:     games,
		UpdatedAt: time.Now(),
	}
	ghc.cache.Store(userID, history)
	
	// 返回副本
	gamesCopy := make([]*models.Game, len(games))
	copy(gamesCopy, games)
	
	return gamesCopy, nil
}

// startCleanupRoutine 启动定期清理任务
func (ghc *GameHistoryCache) startCleanupRoutine() {
	ticker := time.NewTicker(30 * time.Minute) // 每30分钟清理一次
	defer ticker.Stop()
	
	for range ticker.C {
		ghc.performCleanup()
	}
}

// performCleanup 执行清理操作
func (ghc *GameHistoryCache) performCleanup() {
	cleanedCount := 0
	totalCount := 0
	
	ghc.cache.Range(func(key, value interface{}) bool {
		userID := key.(int64)
		history := value.(*UserGameHistory)
		
		totalCount++
		
		// 检查缓存是否过期
		if time.Since(history.UpdatedAt) > ghc.cacheTTL*2 { // 超过2倍TTL时间清理
			ghc.cache.Delete(userID)
			cleanedCount++
		}
		
		return true
	})
	
	if cleanedCount > 0 {
		log.Printf("🧹 游戏历史缓存清理完成: 清理%d个过期缓存, 剩余%d个", 
			cleanedCount, totalCount-cleanedCount)
	}
}

// GetCacheStats 获取缓存统计信息
func (ghc *GameHistoryCache) GetCacheStats() map[string]interface{} {
	cacheSize := 0
	totalGames := 0
	
	ghc.cache.Range(func(key, value interface{}) bool {
		history := value.(*UserGameHistory)
		history.mutex.RLock()
		cacheSize++
		totalGames += len(history.Games)
		history.mutex.RUnlock()
		return true
	})
	
	return map[string]interface{}{
		"cached_users":    cacheSize,
		"total_games":     totalGames,
		"max_per_user":    ghc.maxRecords,
		"cache_ttl":       ghc.cacheTTL.String(),
		"system_status":   "active",
	}
}

// ClearUserCache 清除指定用户的缓存
func (ghc *GameHistoryCache) ClearUserCache(userID int64) {
	ghc.cache.Delete(userID)
	log.Printf("🗑️ 已清除用户%d的游戏历史缓存", userID)
}

// InvalidateCache 使所有缓存失效
func (ghc *GameHistoryCache) InvalidateCache() {
	count := 0
	ghc.cache.Range(func(key, value interface{}) bool {
		ghc.cache.Delete(key)
		count++
		return true
	})
	log.Printf("🔄 游戏历史缓存已全部清除 (%d个用户)", count)
}

// GetUserGameCount 获取用户当前游戏记录数量
func (ghc *GameHistoryCache) GetUserGameCount(userID int64) int {
	if cached, exists := ghc.cache.Load(userID); exists {
		history := cached.(*UserGameHistory)
		history.mutex.RLock()
		defer history.mutex.RUnlock()
		return len(history.Games)
	}
	return 0
}