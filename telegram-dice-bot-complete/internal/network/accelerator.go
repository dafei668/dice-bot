package network

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// NetworkAccelerator 网络加速器
type NetworkAccelerator struct {
	optimizer     *NetworkOptimizer
	retryClient   *RetryableHTTPClient
	cache         *ResponseCache
	monitor       *NetworkMonitor
	originalAPI   *tgbotapi.BotAPI
}

// NewNetworkAccelerator 创建网络加速器
func NewNetworkAccelerator() *NetworkAccelerator {
	// 创建网络优化器
	optimizer := NewNetworkOptimizer()
	
	// 创建可重试的HTTP客户端
	retryClient := NewRetryableHTTPClient(optimizer.GetOptimizedClient(), DefaultRetryConfig())
	
	// 创建响应缓存
	cache := NewResponseCache(5 * time.Minute)
	
	// 创建网络监控器
	monitor := NewNetworkMonitor(optimizer)
	
	return &NetworkAccelerator{
		optimizer:   optimizer,
		retryClient: retryClient,
		cache:       cache,
		monitor:     monitor,
	}
}

// InitializeWithBot 使用机器人初始化加速器
func (na *NetworkAccelerator) InitializeWithBot(api *tgbotapi.BotAPI) error {
	na.originalAPI = api
	
	// 查找最佳数据中心
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	bestDC, err := na.optimizer.FindBestDatacenter(ctx)
	if err != nil {
		return fmt.Errorf("初始化网络加速器失败: %v", err)
	}
	
	fmt.Printf("🚀 网络加速器已启动\n")
	fmt.Printf("📍 最佳数据中心: %s (%s)\n", bestDC.Name, bestDC.Location)
	fmt.Printf("⚡ 延迟: %v\n", bestDC.Latency)
	fmt.Printf("🔧 优化功能: HTTP/2, 连接池, 智能重试, 响应缓存, 质量监控\n")
	
	// 替换API的HTTP客户端
	api.Client = na.optimizer.GetOptimizedClient()
	
	return nil
}

// MakeRequest 执行优化的HTTP请求
func (na *NetworkAccelerator) MakeRequest(ctx context.Context, method, url string, body []byte) (*http.Response, error) {
	start := time.Now()
	
	// 检查缓存
	if method == "GET" {
		if entry, found := na.cache.Get(method, url, nil); found {
			// 创建缓存响应
			resp := &http.Response{
				StatusCode: entry.Response.StatusCode,
				Header:     entry.Response.Header,
				Body:       io.NopCloser(bytes.NewReader(entry.Body)),
			}
			return resp, nil
		}
	}
	
	// 创建请求
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	
	// 设置请求头
	req.Header.Set("User-Agent", "TelegramBot/1.0 (Optimized)")
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	
	// 执行带重试的请求
	resp, err := na.retryClient.DoWithRetry(ctx, req)
	
	// 记录统计信息
	latency := time.Since(start)
	endpoint := na.optimizer.GetBestDatacenter().Endpoint
	na.monitor.RecordRequest(endpoint, latency, err != nil)
	
	if err != nil {
		return nil, err
	}
	
	// 缓存GET请求的响应
	if method == "GET" && IsCacheable(method, resp.StatusCode) {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err == nil {
			resp.Body.Close()
			na.cache.Set(method, url, req.Header, resp, bodyBytes, 5*time.Minute)
			resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}
	}
	
	return resp, nil
}

// GetNetworkStats 获取网络统计信息
func (na *NetworkAccelerator) GetNetworkStats() map[string]*NetworkStats {
	return na.monitor.GetAllStats()
}

// GetDatacenterInfo 获取数据中心信息
func (na *NetworkAccelerator) GetDatacenterInfo() []TelegramDC {
	return na.optimizer.GetDatacenterStats()
}

// GetCurrentDatacenter 获取当前数据中心
func (na *NetworkAccelerator) GetCurrentDatacenter() *TelegramDC {
	return na.optimizer.GetBestDatacenter()
}

// RefreshDatacenters 刷新数据中心测试
func (na *NetworkAccelerator) RefreshDatacenters() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	_, err := na.optimizer.FindBestDatacenter(ctx)
	return err
}

// ClearCache 清空缓存
func (na *NetworkAccelerator) ClearCache() {
	na.cache.Clear()
}

// GetCacheSize 获取缓存大小
func (na *NetworkAccelerator) GetCacheSize() int {
	return na.cache.Size()
}

// Stop 停止加速器
func (na *NetworkAccelerator) Stop() {
	na.monitor.Stop()
}

// PrintStatus 打印状态信息
func (na *NetworkAccelerator) PrintStatus() {
	currentDC := na.GetCurrentDatacenter()
	if currentDC == nil {
		fmt.Println("❌ 网络加速器未初始化")
		return
	}
	
	fmt.Println("\n🚀 网络加速器状态:")
	fmt.Printf("📍 当前数据中心: %s (%s)\n", currentDC.Name, currentDC.Location)
	fmt.Printf("⚡ 延迟: %v\n", currentDC.Latency)
	fmt.Printf("💾 缓存条目: %d\n", na.GetCacheSize())
	
	stats := na.GetNetworkStats()
	if len(stats) > 0 {
		fmt.Println("📊 网络统计:")
		for endpoint, stat := range stats {
			fmt.Printf("  %s: 请求%d次, 错误率%.1f%%, 平均延迟%v\n", 
				endpoint, stat.RequestCount, stat.ErrorRate*100, stat.Latency)
		}
	}
	
	fmt.Println("🔧 启用的优化:")
	fmt.Println("  ✅ HTTP/2 支持")
	fmt.Println("  ✅ 连接池优化")
	fmt.Println("  ✅ 智能重试机制")
	fmt.Println("  ✅ 响应缓存")
	fmt.Println("  ✅ 网络质量监控")
	fmt.Println("  ✅ 自动数据中心切换")
}