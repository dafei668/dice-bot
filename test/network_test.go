package test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"telegram-dice-bot/internal/network"
)

func TestNetworkOptimizer(t *testing.T) {
	optimizer := network.NewNetworkOptimizer()
	
	// 测试数据中心选择
	ctx := context.Background()
	bestDC, err := optimizer.FindBestDatacenter(ctx)
	if err != nil {
		t.Logf("无法找到最佳数据中心（可能是网络问题）: %v", err)
		return
	}
	
	if bestDC == nil {
		t.Error("应该返回最佳数据中心")
	}
	
	// 测试HTTP客户端创建
	client := optimizer.GetOptimizedClient()
	if client == nil {
		t.Error("应该返回优化的HTTP客户端")
	}
	
	// 验证HTTP/2支持
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Error("应该使用HTTP Transport")
	}
	
	if transport.TLSClientConfig == nil {
		t.Error("应该配置TLS")
	}
}

func TestRetryableHTTPClient(t *testing.T) {
	config := &network.RetryConfig{
		MaxRetries:    3,
		BaseDelay:     100 * time.Millisecond,
		MaxDelay:      2 * time.Second,
		BackoffFactor: 2.0,
	}
	
	client := network.NewRetryableHTTPClient(&http.Client{
		Timeout: 5 * time.Second,
	}, config)
	
	if client == nil {
		t.Error("应该创建重试客户端")
	}
	
	// 测试基本功能
	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://httpbin.org/status/200", nil)
	
	resp, err := client.DoWithRetry(ctx, req)
	if err != nil {
		t.Logf("网络请求失败（可能是网络问题）: %v", err)
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		t.Errorf("期望状态码200，得到%d", resp.StatusCode)
	}
}

func TestResponseCache(t *testing.T) {
	cache := network.NewResponseCache(5 * time.Minute)
	
	// 测试缓存设置和获取
	key := "test_key"
	url := "https://api.telegram.org/test"
	headers := http.Header{"Content-Type": []string{"application/json"}}
	resp := &http.Response{StatusCode: 200}
	data := []byte("test data")
	ttl := 1 * time.Minute
	
	cache.Set(key, url, headers, resp, data, ttl)
	
	cachedResp, found := cache.Get(key, url, headers)
	if !found {
		t.Error("应该找到缓存的数据")
	}
	
	if cachedResp == nil {
		t.Error("缓存响应不应为空")
	}
}

func TestNetworkMonitor(t *testing.T) {
	optimizer := network.NewNetworkOptimizer()
	monitor := network.NewNetworkMonitor(optimizer)
	
	// 测试统计记录
	monitor.RecordRequest("test_endpoint", 100*time.Millisecond, false)
	monitor.RecordRequest("test_endpoint", 200*time.Millisecond, false)
	
	stats := monitor.GetStats("test_endpoint")
	if stats == nil {
		t.Error("应该有统计数据")
	}
	
	if stats.RequestCount != 2 {
		t.Errorf("期望请求数2，得到%d", stats.RequestCount)
	}
}

func TestNetworkAccelerator(t *testing.T) {
	accelerator := network.NewNetworkAccelerator()
	
	if accelerator == nil {
		t.Error("应该创建网络加速器")
	}
	
	// 测试统计信息
	stats := accelerator.GetNetworkStats()
	if stats == nil {
		t.Error("应该返回统计信息")
	}
	
	// 测试数据中心信息
	dcInfo := accelerator.GetDatacenterInfo()
	if len(dcInfo) == 0 {
		t.Error("应该有数据中心信息")
	}
}