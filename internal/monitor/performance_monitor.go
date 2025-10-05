package monitor

import (
	"fmt"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// PerformanceMonitor 性能监控系统
type PerformanceMonitor struct {
	// 计数器
	gameCount      int64 // 游戏总数
	userCount      int64 // 用户总数
	messageCount   int64 // 消息总数
	errorCount     int64 // 错误总数
	cacheHitCount  int64 // 缓存命中数
	cacheMissCount int64 // 缓存未命中数

	// 性能指标
	avgResponseTime int64 // 平均响应时间(毫秒)
	maxResponseTime int64 // 最大响应时间(毫秒)
	minResponseTime int64 // 最小响应时间(毫秒)

	// 系统信息
	startTime      time.Time
	lastReportTime time.Time

	// 实时统计
	recentRequests []RequestMetric
	requestMutex   sync.RWMutex

	// 配置
	reportInterval time.Duration
	maxRecentCount int

	// 停止信号
	stopChan chan struct{}
	running  bool
	mutex    sync.RWMutex
}

// RequestMetric 请求指标
type RequestMetric struct {
	Timestamp    time.Time
	ResponseTime time.Duration
	Success      bool
	RequestType  string
}

// NewPerformanceMonitor 创建新的性能监控器
func NewPerformanceMonitor() *PerformanceMonitor {
	pm := &PerformanceMonitor{
		startTime:       time.Now(),
		lastReportTime:  time.Now(),
		reportInterval:  5 * time.Minute, // 每5分钟报告一次
		maxRecentCount:  1000,            // 保留最近1000个请求记录
		recentRequests:  make([]RequestMetric, 0, 1000),
		stopChan:        make(chan struct{}),
		minResponseTime: int64(^uint64(0) >> 1), // 初始化为最大值
	}

	return pm
}

// Start 启动性能监控
func (pm *PerformanceMonitor) Start() {
	pm.mutex.Lock()
	if pm.running {
		pm.mutex.Unlock()
		return
	}
	pm.running = true
	pm.mutex.Unlock()

	log.Printf("🚀 性能监控系统已启动 (报告间隔: %v)", pm.reportInterval)

	// 启动定期报告协程
	go pm.reportLoop()
}

// Stop 停止性能监控
func (pm *PerformanceMonitor) Stop() {
	pm.mutex.Lock()
	if !pm.running {
		pm.mutex.Unlock()
		return
	}
	pm.running = false
	pm.mutex.Unlock()

	close(pm.stopChan)
	log.Printf("⏹️ 性能监控系统已停止")
}

// RecordRequest 记录请求
func (pm *PerformanceMonitor) RecordRequest(requestType string, responseTime time.Duration, success bool) {
	atomic.AddInt64(&pm.messageCount, 1)

	if !success {
		atomic.AddInt64(&pm.errorCount, 1)
	}

	// 更新响应时间统计
	responseTimeMs := responseTime.Milliseconds()
	pm.updateResponseTime(responseTimeMs)

	// 记录详细请求信息
	pm.requestMutex.Lock()
	metric := RequestMetric{
		Timestamp:    time.Now(),
		ResponseTime: responseTime,
		Success:      success,
		RequestType:  requestType,
	}

	pm.recentRequests = append(pm.recentRequests, metric)

	// 限制记录数量
	if len(pm.recentRequests) > pm.maxRecentCount {
		pm.recentRequests = pm.recentRequests[len(pm.recentRequests)-pm.maxRecentCount:]
	}
	pm.requestMutex.Unlock()
}

// updateResponseTime 更新响应时间统计
func (pm *PerformanceMonitor) updateResponseTime(responseTimeMs int64) {
	// 更新平均响应时间 (简单移动平均)
	currentAvg := atomic.LoadInt64(&pm.avgResponseTime)
	newAvg := (currentAvg + responseTimeMs) / 2
	atomic.StoreInt64(&pm.avgResponseTime, newAvg)

	// 更新最大响应时间
	for {
		currentMax := atomic.LoadInt64(&pm.maxResponseTime)
		if responseTimeMs <= currentMax {
			break
		}
		if atomic.CompareAndSwapInt64(&pm.maxResponseTime, currentMax, responseTimeMs) {
			break
		}
	}

	// 更新最小响应时间
	for {
		currentMin := atomic.LoadInt64(&pm.minResponseTime)
		if responseTimeMs >= currentMin {
			break
		}
		if atomic.CompareAndSwapInt64(&pm.minResponseTime, currentMin, responseTimeMs) {
			break
		}
	}
}

// IncrementGameCount 增加游戏计数
func (pm *PerformanceMonitor) IncrementGameCount() {
	atomic.AddInt64(&pm.gameCount, 1)
}

// IncrementUserCount 增加用户计数
func (pm *PerformanceMonitor) IncrementUserCount() {
	atomic.AddInt64(&pm.userCount, 1)
}

// RecordCacheHit 记录缓存命中
func (pm *PerformanceMonitor) RecordCacheHit() {
	atomic.AddInt64(&pm.cacheHitCount, 1)
}

// RecordCacheMiss 记录缓存未命中
func (pm *PerformanceMonitor) RecordCacheMiss() {
	atomic.AddInt64(&pm.cacheMissCount, 1)
}

// reportLoop 定期报告循环
func (pm *PerformanceMonitor) reportLoop() {
	ticker := time.NewTicker(pm.reportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pm.generateReport()
		case <-pm.stopChan:
			return
		}
	}
}

// generateReport 生成性能报告
func (pm *PerformanceMonitor) generateReport() {
	now := time.Now()
	uptime := now.Sub(pm.startTime)
	intervalDuration := now.Sub(pm.lastReportTime)

	// 获取系统内存信息
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// 计算缓存命中率
	totalCacheRequests := atomic.LoadInt64(&pm.cacheHitCount) + atomic.LoadInt64(&pm.cacheMissCount)
	cacheHitRate := float64(0)
	if totalCacheRequests > 0 {
		cacheHitRate = float64(atomic.LoadInt64(&pm.cacheHitCount)) / float64(totalCacheRequests) * 100
	}

	// 计算错误率
	totalMessages := atomic.LoadInt64(&pm.messageCount)
	errorRate := float64(0)
	if totalMessages > 0 {
		errorRate = float64(atomic.LoadInt64(&pm.errorCount)) / float64(totalMessages) * 100
	}

	// 计算最近请求的统计信息
	recentStats := pm.calculateRecentStats(intervalDuration)

	// 生成报告
	report := fmt.Sprintf(`
📊 ===== 性能监控报告 =====
⏰ 报告时间: %s
🕐 系统运行时间: %v
📈 报告间隔: %v

🎯 业务指标:
  🎲 游戏总数: %d
  👥 用户总数: %d
  📨 消息总数: %d
  ❌ 错误总数: %d (错误率: %.2f%%)

⚡ 性能指标:
  📊 平均响应时间: %dms
  ⚡ 最小响应时间: %dms
  🔥 最大响应时间: %dms
  📈 最近%v平均响应: %dms

💾 缓存统计:
  ✅ 缓存命中: %d
  ❌ 缓存未命中: %d
  📊 命中率: %.2f%%

🖥️ 系统资源:
  💾 内存使用: %.2f MB
  🗑️ GC次数: %d
  🔄 协程数: %d

📊 最近活动:
  📨 请求数: %d
  ✅ 成功率: %.2f%%
  ⚡ QPS: %.2f
=============================`,
		now.Format("2006-01-02 15:04:05"),
		uptime.Round(time.Second),
		intervalDuration.Round(time.Second),
		atomic.LoadInt64(&pm.gameCount),
		atomic.LoadInt64(&pm.userCount),
		atomic.LoadInt64(&pm.messageCount),
		atomic.LoadInt64(&pm.errorCount),
		errorRate,
		atomic.LoadInt64(&pm.avgResponseTime),
		atomic.LoadInt64(&pm.minResponseTime),
		atomic.LoadInt64(&pm.maxResponseTime),
		intervalDuration.Round(time.Second),
		recentStats.avgResponseTime,
		atomic.LoadInt64(&pm.cacheHitCount),
		atomic.LoadInt64(&pm.cacheMissCount),
		cacheHitRate,
		float64(memStats.Alloc)/1024/1024,
		memStats.NumGC,
		runtime.NumGoroutine(),
		recentStats.requestCount,
		recentStats.successRate,
		recentStats.qps,
	)

	log.Printf("%s", report)
	pm.lastReportTime = now
}

// RecentStats 最近统计信息
type RecentStats struct {
	requestCount    int
	successRate     float64
	avgResponseTime int64
	qps             float64
}

// calculateRecentStats 计算最近的统计信息
func (pm *PerformanceMonitor) calculateRecentStats(interval time.Duration) RecentStats {
	pm.requestMutex.RLock()
	defer pm.requestMutex.RUnlock()

	cutoff := time.Now().Add(-interval)
	recentCount := 0
	successCount := 0
	totalResponseTime := int64(0)

	for _, req := range pm.recentRequests {
		if req.Timestamp.After(cutoff) {
			recentCount++
			if req.Success {
				successCount++
			}
			totalResponseTime += req.ResponseTime.Milliseconds()
		}
	}

	stats := RecentStats{
		requestCount: recentCount,
	}

	if recentCount > 0 {
		stats.successRate = float64(successCount) / float64(recentCount) * 100
		stats.avgResponseTime = totalResponseTime / int64(recentCount)
		stats.qps = float64(recentCount) / interval.Seconds()
	}

	return stats
}

// GetCurrentStats 获取当前统计信息
func (pm *PerformanceMonitor) GetCurrentStats() map[string]interface{} {
	uptime := time.Since(pm.startTime)

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	totalCacheRequests := atomic.LoadInt64(&pm.cacheHitCount) + atomic.LoadInt64(&pm.cacheMissCount)
	cacheHitRate := float64(0)
	if totalCacheRequests > 0 {
		cacheHitRate = float64(atomic.LoadInt64(&pm.cacheHitCount)) / float64(totalCacheRequests) * 100
	}

	return map[string]interface{}{
		"uptime_seconds":  int64(uptime.Seconds()),
		"game_count":      atomic.LoadInt64(&pm.gameCount),
		"user_count":      atomic.LoadInt64(&pm.userCount),
		"message_count":   atomic.LoadInt64(&pm.messageCount),
		"error_count":     atomic.LoadInt64(&pm.errorCount),
		"avg_response_ms": atomic.LoadInt64(&pm.avgResponseTime),
		"max_response_ms": atomic.LoadInt64(&pm.maxResponseTime),
		"min_response_ms": atomic.LoadInt64(&pm.minResponseTime),
		"cache_hit_rate":  cacheHitRate,
		"memory_mb":       float64(memStats.Alloc) / 1024 / 1024,
		"goroutine_count": runtime.NumGoroutine(),
		"gc_count":        memStats.NumGC,
		"system_status":   "healthy",
	}
}

// SetReportInterval 设置报告间隔
func (pm *PerformanceMonitor) SetReportInterval(interval time.Duration) {
	pm.reportInterval = interval
	log.Printf("🔧 性能监控报告间隔已设置为: %v", interval)
}
