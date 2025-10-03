package pool

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// WorkerPool 工作池，用于处理并发任务
type WorkerPool struct {
	workers    int
	jobQueue   chan Job
	workerPool chan chan Job
	quit       chan bool
	wg         sync.WaitGroup
}

// Job 工作任务接口
type Job interface {
	Execute() error
}

// MessageJob 消息处理任务
type MessageJob struct {
	Handler func() error
}

func (j *MessageJob) Execute() error {
	return j.Handler()
}

// NewWorkerPool 创建新的工作池
func NewWorkerPool(workers int, queueSize int) *WorkerPool {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	return &WorkerPool{
		workers:    workers,
		jobQueue:   make(chan Job, queueSize),
		workerPool: make(chan chan Job, workers),
		quit:       make(chan bool),
	}
}

// Start 启动工作池
func (p *WorkerPool) Start() {
	// 启动工作者
	for i := 0; i < p.workers; i++ {
		worker := NewWorker(p.workerPool, p.quit)
		worker.Start()
	}

	// 启动调度器
	go p.dispatch()
}

// Stop 停止工作池
func (p *WorkerPool) Stop() {
	close(p.quit)
	p.wg.Wait()
}

// Submit 提交任务
func (p *WorkerPool) Submit(job Job) {
	select {
	case p.jobQueue <- job:
	default:
		// 队列满时，直接执行（防止阻塞）
		go job.Execute()
	}
}

// dispatch 调度任务
func (p *WorkerPool) dispatch() {
	for {
		select {
		case job := <-p.jobQueue:
			// 获取可用的工作者
			select {
			case jobChannel := <-p.workerPool:
				jobChannel <- job
			case <-p.quit:
				return
			}
		case <-p.quit:
			return
		}
	}
}

// Worker 工作者
type Worker struct {
	workerPool chan chan Job
	jobChannel chan Job
	quit       chan bool
}

// NewWorker 创建新工作者
func NewWorker(workerPool chan chan Job, quit chan bool) *Worker {
	return &Worker{
		workerPool: workerPool,
		jobChannel: make(chan Job),
		quit:       quit,
	}
}

// Start 启动工作者
func (w *Worker) Start() {
	go func() {
		for {
			// 将工作者注册到工作池
			w.workerPool <- w.jobChannel

			select {
			case job := <-w.jobChannel:
				// 执行任务
				job.Execute()
			case <-w.quit:
				return
			}
		}
	}()
}

// RateLimiter 速率限制器
type RateLimiter struct {
	tokens   chan struct{}
	interval time.Duration
	quit     chan bool
}

// NewRateLimiter 创建速率限制器
func NewRateLimiter(rate int, interval time.Duration) *RateLimiter {
	rl := &RateLimiter{
		tokens:   make(chan struct{}, rate),
		interval: interval,
		quit:     make(chan bool),
	}

	// 填充初始令牌
	for i := 0; i < rate; i++ {
		rl.tokens <- struct{}{}
	}

	// 启动令牌补充
	go rl.refill(rate)

	return rl
}

// Allow 检查是否允许执行
func (rl *RateLimiter) Allow() bool {
	select {
	case <-rl.tokens:
		return true
	default:
		return false
	}
}

// Wait 等待直到可以执行
func (rl *RateLimiter) Wait(ctx context.Context) error {
	select {
	case <-rl.tokens:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// refill 补充令牌
func (rl *RateLimiter) refill(rate int) {
	ticker := time.NewTicker(rl.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// 尝试添加令牌
			for i := 0; i < rate; i++ {
				select {
				case rl.tokens <- struct{}{}:
				default:
					// 令牌桶已满
				}
			}
		case <-rl.quit:
			return
		}
	}
}

// Stop 停止速率限制器
func (rl *RateLimiter) Stop() {
	close(rl.quit)
}

// Cache 简单的内存缓存
type Cache struct {
	data map[string]CacheItem
	mu   sync.RWMutex
}

type CacheItem struct {
	Value      interface{}
	Expiration time.Time
}

// NewCache 创建新缓存
func NewCache() *Cache {
	cache := &Cache{
		data: make(map[string]CacheItem),
	}

	// 启动清理协程
	go cache.cleanup()

	return cache
}

// Set 设置缓存
func (c *Cache) Set(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[key] = CacheItem{
		Value:      value,
		Expiration: time.Now().Add(ttl),
	}
}

// Get 获取缓存
func (c *Cache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, exists := c.data[key]
	if !exists {
		return nil, false
	}

	if time.Now().After(item.Expiration) {
		return nil, false
	}

	return item.Value, true
}

// Delete 删除缓存
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.data, key)
}

// cleanup 清理过期缓存
func (c *Cache) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, item := range c.data {
			if now.After(item.Expiration) {
				delete(c.data, key)
			}
		}
		c.mu.Unlock()
	}
}

// ObjectPool 对象池，用于复用常用对象
type ObjectPool struct {
	messagePool   sync.Pool
	keyboardPool  sync.Pool
	stringBuilder sync.Pool
}

// NewObjectPool 创建对象池
func NewObjectPool() *ObjectPool {
	return &ObjectPool{
		messagePool: sync.Pool{
			New: func() interface{} {
				return &tgbotapi.MessageConfig{}
			},
		},
		keyboardPool: sync.Pool{
			New: func() interface{} {
				return &tgbotapi.InlineKeyboardMarkup{}
			},
		},
		stringBuilder: sync.Pool{
			New: func() interface{} {
				return &strings.Builder{}
			},
		},
	}
}

// GetMessage 获取消息对象
func (p *ObjectPool) GetMessage() *tgbotapi.MessageConfig {
	msg := p.messagePool.Get().(*tgbotapi.MessageConfig)
	// 重置消息对象
	*msg = tgbotapi.MessageConfig{}
	return msg
}

// PutMessage 归还消息对象
func (p *ObjectPool) PutMessage(msg *tgbotapi.MessageConfig) {
	p.messagePool.Put(msg)
}

// GetKeyboard 获取键盘对象
func (p *ObjectPool) GetKeyboard() *tgbotapi.InlineKeyboardMarkup {
	kb := p.keyboardPool.Get().(*tgbotapi.InlineKeyboardMarkup)
	// 重置键盘对象
	kb.InlineKeyboard = kb.InlineKeyboard[:0]
	return kb
}

// PutKeyboard 归还键盘对象
func (p *ObjectPool) PutKeyboard(kb *tgbotapi.InlineKeyboardMarkup) {
	p.keyboardPool.Put(kb)
}

// GetStringBuilder 获取字符串构建器
func (p *ObjectPool) GetStringBuilder() *strings.Builder {
	sb := p.stringBuilder.Get().(*strings.Builder)
	sb.Reset()
	return sb
}

// PutStringBuilder 归还字符串构建器
func (p *ObjectPool) PutStringBuilder(sb *strings.Builder) {
	p.stringBuilder.Put(sb)
}
