package network

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// TelegramDC 表示Telegram数据中心信息
type TelegramDC struct {
	ID       int
	Name     string
	Endpoint string
	Location string
	Latency  time.Duration
	Success  bool
}

// NetworkOptimizer 网络优化器
type NetworkOptimizer struct {
	datacenters []TelegramDC
	bestDC      *TelegramDC
	client      *http.Client
	mu          sync.RWMutex
}

// NewNetworkOptimizer 创建网络优化器
func NewNetworkOptimizer() *NetworkOptimizer {
	// 定义Telegram的主要数据中心
	datacenters := []TelegramDC{
		{ID: 1, Name: "DC1-Miami", Endpoint: "149.154.175.53", Location: "Miami, US", Latency: 0, Success: false},
		{ID: 2, Name: "DC2-Amsterdam", Endpoint: "149.154.167.51", Location: "Amsterdam, NL", Latency: 0, Success: false},
		{ID: 3, Name: "DC3-Miami", Endpoint: "149.154.175.100", Location: "Miami, US", Latency: 0, Success: false},
		{ID: 4, Name: "DC4-Amsterdam", Endpoint: "149.154.167.91", Location: "Amsterdam, NL", Latency: 0, Success: false},
		{ID: 5, Name: "DC5-Singapore", Endpoint: "91.108.56.130", Location: "Singapore, SG", Latency: 0, Success: false},
	}

	// 创建优化的HTTP客户端
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,  // 连接超时
			KeepAlive: 60 * time.Second,  // 增加保活时间，复用连接
		}).DialContext,
		ForceAttemptHTTP2:     false,     // 禁用HTTP/2，使用HTTP/1.1更稳定
		MaxIdleConns:          100,       // 增加最大空闲连接池
		MaxIdleConnsPerHost:   20,        // 增加每个主机的连接数
		IdleConnTimeout:       90 * time.Second,  // 增加空闲连接超时
		TLSHandshakeTimeout:   10 * time.Second,  // TLS握手超时
		ExpectContinueTimeout: 2 * time.Second,   // 期望继续超时
		ResponseHeaderTimeout: 45 * time.Second,  // 响应头超时
		DisableCompression:    false,
		DisableKeepAlives:     false,     // 启用Keep-Alive
	}

	// 不启用HTTP/2，使用更稳定的HTTP/1.1
	// http2.ConfigureTransport(transport)

	client := &http.Client{
		Transport: transport,
		Timeout:   60 * time.Second,  // 进一步增加总体超时时间以适应高延迟网络
	}

	return &NetworkOptimizer{
		datacenters: datacenters,
		client:      client,
	}
}

// TestDatacenterLatency 测试数据中心延迟
func (no *NetworkOptimizer) TestDatacenterLatency(ctx context.Context, dc *TelegramDC) error {
	start := time.Now()
	
	// 使用TCP连接测试延迟
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:443", dc.Endpoint), 10*time.Second)
	if err != nil {
		dc.Success = false
		dc.Latency = time.Hour // 设置一个很大的延迟表示失败
		return err
	}
	defer conn.Close()

	dc.Latency = time.Since(start)
	dc.Success = true
	return nil
}

// FindBestDatacenter 查找最佳数据中心
func (no *NetworkOptimizer) FindBestDatacenter(ctx context.Context) (*TelegramDC, error) {
	var wg sync.WaitGroup
	
	// 并发测试所有数据中心
	for i := range no.datacenters {
		wg.Add(1)
		go func(dc *TelegramDC) {
			defer wg.Done()
			no.TestDatacenterLatency(ctx, dc)
		}(&no.datacenters[i])
	}
	
	wg.Wait()

	// 找到延迟最低且连接成功的数据中心
	var bestDC *TelegramDC
	minLatency := time.Hour

	for i := range no.datacenters {
		dc := &no.datacenters[i]
		if dc.Success && dc.Latency < minLatency {
			minLatency = dc.Latency
			bestDC = dc
		}
	}

	if bestDC == nil {
		return nil, fmt.Errorf("无法连接到任何Telegram数据中心")
	}

	no.mu.Lock()
	no.bestDC = bestDC
	no.mu.Unlock()

	return bestDC, nil
}

// GetBestDatacenter 获取最佳数据中心
func (no *NetworkOptimizer) GetBestDatacenter() *TelegramDC {
	no.mu.RLock()
	defer no.mu.RUnlock()
	return no.bestDC
}

// GetOptimizedClient 获取优化的HTTP客户端
func (no *NetworkOptimizer) GetOptimizedClient() *http.Client {
	return no.client
}

// GetDatacenterStats 获取所有数据中心统计信息
func (no *NetworkOptimizer) GetDatacenterStats() []TelegramDC {
	no.mu.RLock()
	defer no.mu.RUnlock()
	
	stats := make([]TelegramDC, len(no.datacenters))
	copy(stats, no.datacenters)
	return stats
}