package network

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/http"
	"syscall"
	"time"
)

// RetryConfig 重试配置
type RetryConfig struct {
	MaxRetries      int           // 最大重试次数
	BaseDelay       time.Duration // 基础延迟
	MaxDelay        time.Duration // 最大延迟
	BackoffFactor   float64       // 退避因子
	JitterFactor    float64       // 抖动因子
	RetryableErrors []error       // 可重试的错误类型
}

// DefaultRetryConfig 默认重试配置
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxRetries:    5,
		BaseDelay:     100 * time.Millisecond,
		MaxDelay:      30 * time.Second,
		BackoffFactor: 2.0,
		JitterFactor:  0.1,
		RetryableErrors: []error{
			syscall.ECONNREFUSED,
			syscall.ECONNRESET,
			syscall.ETIMEDOUT,
			context.DeadlineExceeded,
		},
	}
}

// RetryableHTTPClient 可重试的HTTP客户端
type RetryableHTTPClient struct {
	client *http.Client
	config *RetryConfig
}

// NewRetryableHTTPClient 创建可重试的HTTP客户端
func NewRetryableHTTPClient(client *http.Client, config *RetryConfig) *RetryableHTTPClient {
	if config == nil {
		config = DefaultRetryConfig()
	}
	return &RetryableHTTPClient{
		client: client,
		config: config,
	}
}

// IsRetryableError 判断错误是否可重试
func (r *RetryableHTTPClient) IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// 检查网络错误
	if netErr, ok := err.(net.Error); ok {
		if netErr.Timeout() || netErr.Temporary() {
			return true
		}
	}

	// 检查系统调用错误
	if opErr, ok := err.(*net.OpError); ok {
		if syscallErr, ok := opErr.Err.(*syscall.Errno); ok {
			switch *syscallErr {
			case syscall.ECONNREFUSED, syscall.ECONNRESET, syscall.ETIMEDOUT:
				return true
			}
		}
	}

	// 检查上下文错误
	if err == context.DeadlineExceeded || err == context.Canceled {
		return true
	}

	return false
}

// IsRetryableStatusCode 判断HTTP状态码是否可重试
func (r *RetryableHTTPClient) IsRetryableStatusCode(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests,     // 429
		http.StatusInternalServerError,   // 500
		http.StatusBadGateway,           // 502
		http.StatusServiceUnavailable,   // 503
		http.StatusGatewayTimeout:       // 504
		return true
	default:
		return false
	}
}

// CalculateDelay 计算重试延迟（指数退避 + 抖动）
func (r *RetryableHTTPClient) CalculateDelay(attempt int) time.Duration {
	// 指数退避
	delay := float64(r.config.BaseDelay) * math.Pow(r.config.BackoffFactor, float64(attempt))
	
	// 添加抖动以避免惊群效应
	jitter := delay * r.config.JitterFactor * (rand.Float64()*2 - 1)
	delay += jitter
	
	// 限制最大延迟
	if delay > float64(r.config.MaxDelay) {
		delay = float64(r.config.MaxDelay)
	}
	
	return time.Duration(delay)
}

// DoWithRetry 执行带重试的HTTP请求
func (r *RetryableHTTPClient) DoWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	var lastErr error
	
	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		// 如果不是第一次尝试，等待重试延迟
		if attempt > 0 {
			delay := r.CalculateDelay(attempt - 1)
			
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
				// 继续重试
			}
		}
		
		// 克隆请求以避免重复使用问题
		reqClone := req.Clone(ctx)
		
		// 执行请求
		resp, err := r.client.Do(reqClone)
		
		// 如果成功或不可重试，直接返回
		if err == nil {
			if !r.IsRetryableStatusCode(resp.StatusCode) {
				return resp, nil
			}
			// 状态码可重试，关闭响应体并继续
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
		} else {
			lastErr = err
			// 如果错误不可重试，直接返回
			if !r.IsRetryableError(err) {
				return nil, err
			}
		}
		
		// 如果是最后一次尝试，不再重试
		if attempt == r.config.MaxRetries {
			break
		}
	}
	
	return nil, fmt.Errorf("请求失败，已重试%d次: %v", r.config.MaxRetries, lastErr)
}

// GetWithRetry 执行带重试的GET请求
func (r *RetryableHTTPClient) GetWithRetry(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	return r.DoWithRetry(ctx, req)
}

// PostWithRetry 执行带重试的POST请求
func (r *RetryableHTTPClient) PostWithRetry(ctx context.Context, url, contentType string, body interface{}) (*http.Response, error) {
	var bodyReader *bytes.Reader
	if body != nil {
		if data, ok := body.([]byte); ok {
			bodyReader = bytes.NewReader(data)
		} else {
			return nil, fmt.Errorf("不支持的body类型")
		}
	}
	
	req, err := http.NewRequestWithContext(ctx, "POST", url, bodyReader)
	if err != nil {
		return nil, err
	}
	
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	
	return r.DoWithRetry(ctx, req)
}