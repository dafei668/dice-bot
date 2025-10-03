package network

import (
	"crypto/md5"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// CacheEntry 缓存条目
type CacheEntry struct {
	Response  *http.Response
	Body      []byte
	Timestamp time.Time
	TTL       time.Duration
}

// IsExpired 检查缓存是否过期
func (ce *CacheEntry) IsExpired() bool {
	return time.Since(ce.Timestamp) > ce.TTL
}

// ResponseCache 响应缓存
type ResponseCache struct {
	cache map[string]*CacheEntry
	mu    sync.RWMutex
	ttl   time.Duration
}

// NewResponseCache 创建响应缓存
func NewResponseCache(defaultTTL time.Duration) *ResponseCache {
	cache := &ResponseCache{
		cache: make(map[string]*CacheEntry),
		ttl:   defaultTTL,
	}
	
	// 启动清理协程
	go cache.cleanup()
	
	return cache
}

// generateKey 生成缓存键
func (rc *ResponseCache) generateKey(method, url string, headers http.Header) string {
	h := md5.New()
	h.Write([]byte(method))
	h.Write([]byte(url))
	
	// 包含重要的请求头
	for key, values := range headers {
		switch key {
		case "Authorization", "Content-Type", "Accept":
			for _, value := range values {
				h.Write([]byte(key))
				h.Write([]byte(value))
			}
		}
	}
	
	return fmt.Sprintf("%x", h.Sum(nil))
}

// Get 获取缓存的响应
func (rc *ResponseCache) Get(method, url string, headers http.Header) (*CacheEntry, bool) {
	key := rc.generateKey(method, url, headers)
	
	rc.mu.RLock()
	entry, exists := rc.cache[key]
	rc.mu.RUnlock()
	
	if !exists || entry.IsExpired() {
		if exists {
			// 删除过期条目
			rc.mu.Lock()
			delete(rc.cache, key)
			rc.mu.Unlock()
		}
		return nil, false
	}
	
	return entry, true
}

// Set 设置缓存响应
func (rc *ResponseCache) Set(method, url string, headers http.Header, response *http.Response, body []byte, ttl time.Duration) {
	key := rc.generateKey(method, url, headers)
	
	if ttl == 0 {
		ttl = rc.ttl
	}
	
	entry := &CacheEntry{
		Response:  response,
		Body:      body,
		Timestamp: time.Now(),
		TTL:       ttl,
	}
	
	rc.mu.Lock()
	rc.cache[key] = entry
	rc.mu.Unlock()
}

// Delete 删除缓存条目
func (rc *ResponseCache) Delete(method, url string, headers http.Header) {
	key := rc.generateKey(method, url, headers)
	
	rc.mu.Lock()
	delete(rc.cache, key)
	rc.mu.Unlock()
}

// Clear 清空所有缓存
func (rc *ResponseCache) Clear() {
	rc.mu.Lock()
	rc.cache = make(map[string]*CacheEntry)
	rc.mu.Unlock()
}

// Size 获取缓存大小
func (rc *ResponseCache) Size() int {
	rc.mu.RLock()
	size := len(rc.cache)
	rc.mu.RUnlock()
	return size
}

// cleanup 定期清理过期缓存
func (rc *ResponseCache) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		rc.mu.Lock()
		for key, entry := range rc.cache {
			if entry.IsExpired() {
				delete(rc.cache, key)
			}
		}
		rc.mu.Unlock()
	}
}

// IsCacheable 判断响应是否可缓存
func IsCacheable(method string, statusCode int) bool {
	// 只缓存GET请求
	if method != "GET" {
		return false
	}
	
	// 只缓存成功的响应
	switch statusCode {
	case http.StatusOK,
		http.StatusCreated,
		http.StatusAccepted,
		http.StatusNonAuthoritativeInfo,
		http.StatusNoContent,
		http.StatusResetContent,
		http.StatusPartialContent:
		return true
	default:
		return false
	}
}