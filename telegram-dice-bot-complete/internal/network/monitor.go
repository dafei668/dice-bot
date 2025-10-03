package network

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// NetworkStats 网络统计信息
type NetworkStats struct {
	Latency       time.Duration
	PacketLoss    float64
	Throughput    float64
	ErrorRate     float64
	LastUpdate    time.Time
	RequestCount  int64
	ErrorCount    int64
	TotalLatency  time.Duration
}

// UpdateStats 更新统计信息
func (ns *NetworkStats) UpdateStats(latency time.Duration, isError bool) {
	ns.RequestCount++
	ns.TotalLatency += latency
	ns.Latency = ns.TotalLatency / time.Duration(ns.RequestCount)
	
	if isError {
		ns.ErrorCount++
	}
	
	ns.ErrorRate = float64(ns.ErrorCount) / float64(ns.RequestCount)
	ns.LastUpdate = time.Now()
}

// NetworkMonitor 网络质量监控器
type NetworkMonitor struct {
	optimizer    *NetworkOptimizer
	stats        map[string]*NetworkStats
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
	
	// 监控配置
	checkInterval    time.Duration
	switchThreshold  float64  // 错误率阈值
	latencyThreshold time.Duration // 延迟阈值
}

// NewNetworkMonitor 创建网络监控器
func NewNetworkMonitor(optimizer *NetworkOptimizer) *NetworkMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	
	monitor := &NetworkMonitor{
		optimizer:        optimizer,
		stats:           make(map[string]*NetworkStats),
		ctx:             ctx,
		cancel:          cancel,
		checkInterval:   30 * time.Second,
		switchThreshold: 0.3, // 30%错误率
		latencyThreshold: 200 * time.Millisecond,
	}
	
	// 启动监控协程
	go monitor.startMonitoring()
	
	return monitor
}

// RecordRequest 记录请求统计
func (nm *NetworkMonitor) RecordRequest(endpoint string, latency time.Duration, isError bool) {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	
	stats, exists := nm.stats[endpoint]
	if !exists {
		stats = &NetworkStats{
			LastUpdate: time.Now(),
		}
		nm.stats[endpoint] = stats
	}
	
	stats.UpdateStats(latency, isError)
}

// GetStats 获取统计信息
func (nm *NetworkMonitor) GetStats(endpoint string) *NetworkStats {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	
	if stats, exists := nm.stats[endpoint]; exists {
		// 返回副本以避免并发问题
		return &NetworkStats{
			Latency:      stats.Latency,
			PacketLoss:   stats.PacketLoss,
			Throughput:   stats.Throughput,
			ErrorRate:    stats.ErrorRate,
			LastUpdate:   stats.LastUpdate,
			RequestCount: stats.RequestCount,
			ErrorCount:   stats.ErrorCount,
			TotalLatency: stats.TotalLatency,
		}
	}
	
	return nil
}

// GetAllStats 获取所有统计信息
func (nm *NetworkMonitor) GetAllStats() map[string]*NetworkStats {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	
	result := make(map[string]*NetworkStats)
	for endpoint, stats := range nm.stats {
		result[endpoint] = &NetworkStats{
			Latency:      stats.Latency,
			PacketLoss:   stats.PacketLoss,
			Throughput:   stats.Throughput,
			ErrorRate:    stats.ErrorRate,
			LastUpdate:   stats.LastUpdate,
			RequestCount: stats.RequestCount,
			ErrorCount:   stats.ErrorCount,
			TotalLatency: stats.TotalLatency,
		}
	}
	
	return result
}

// startMonitoring 启动监控
func (nm *NetworkMonitor) startMonitoring() {
	ticker := time.NewTicker(nm.checkInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-nm.ctx.Done():
			return
		case <-ticker.C:
			nm.checkAndSwitch()
		}
	}
}

// checkAndSwitch 检查网络质量并自动切换
func (nm *NetworkMonitor) checkAndSwitch() {
	currentDC := nm.optimizer.GetBestDatacenter()
	if currentDC == nil {
		return
	}
	
	currentStats := nm.GetStats(currentDC.Endpoint)
	if currentStats == nil {
		return
	}
	
	// 检查是否需要切换
	needSwitch := false
	reason := ""
	
	if currentStats.ErrorRate > nm.switchThreshold {
		needSwitch = true
		reason = fmt.Sprintf("错误率过高: %.2f%%", currentStats.ErrorRate*100)
	} else if currentStats.Latency > nm.latencyThreshold {
		needSwitch = true
		reason = fmt.Sprintf("延迟过高: %v", currentStats.Latency)
	}
	
	if needSwitch {
		log.Printf("检测到网络质量问题: %s，尝试切换数据中心", reason)
		
		// 重新测试所有数据中心
		newDC, err := nm.optimizer.FindBestDatacenter(nm.ctx)
		if err != nil {
			log.Printf("切换数据中心失败: %v", err)
			return
		}
		
		if newDC.Endpoint != currentDC.Endpoint {
			log.Printf("已切换到新的数据中心: %s -> %s (延迟: %v)", 
				currentDC.Name, newDC.Name, newDC.Latency)
		}
	}
}

// Stop 停止监控
func (nm *NetworkMonitor) Stop() {
	nm.cancel()
}

// SetThresholds 设置阈值
func (nm *NetworkMonitor) SetThresholds(errorRate float64, latency time.Duration) {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	
	nm.switchThreshold = errorRate
	nm.latencyThreshold = latency
}

// ResetStats 重置统计信息
func (nm *NetworkMonitor) ResetStats() {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	
	nm.stats = make(map[string]*NetworkStats)
}