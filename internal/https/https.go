package https

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"path/filepath"

	"golang.org/x/crypto/acme/autocert"
)

// Config HTTPS配置
type Config struct {
	Domain     string // 域名
	CacheDir   string // 证书缓存目录
	Email      string // Let's Encrypt邮箱
	HTTPPort   string // HTTP端口（用于重定向）
	HTTPSPort  string // HTTPS端口
	Production bool   // 是否生产环境
}

// Manager HTTPS管理器
type Manager struct {
	config     *Config
	certManager *autocert.Manager
}

// NewManager 创建HTTPS管理器
func NewManager(config *Config) *Manager {
	// 创建证书缓存目录
	cacheDir := config.CacheDir
	if cacheDir == "" {
		cacheDir = "./certs"
	}

	// 配置autocert管理器
	certManager := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(config.Domain),
		Cache:      autocert.DirCache(cacheDir),
		Email:      config.Email,
	}

	return &Manager{
		config:      config,
		certManager: certManager,
	}
}

// GetTLSConfig 获取TLS配置
func (m *Manager) GetTLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: m.certManager.GetCertificate,
		NextProtos:     []string{"h2", "http/1.1"},
		MinVersion:     tls.VersionTLS12,
	}
}

// StartHTTPRedirectServer 启动HTTP重定向服务器
func (m *Manager) StartHTTPRedirectServer() {
	// HTTP重定向服务器固定使用端口80
	httpPort := ":80"

	// HTTP重定向到HTTPS
	redirectHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := "https://" + r.Host + r.URL.Path
		if len(r.URL.RawQuery) > 0 {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	})

	// 处理ACME挑战
	handler := m.certManager.HTTPHandler(redirectHandler)

	server := &http.Server{
		Addr:    httpPort,
		Handler: handler,
	}

	log.Printf("🔄 HTTP重定向服务器启动在端口 %s", httpPort)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP重定向服务器错误: %v", err)
		}
	}()
}

// StartHTTPSServer 启动HTTPS服务器
func (m *Manager) StartHTTPSServer(handler http.Handler) error {
	httpsPort := m.config.HTTPSPort
	if httpsPort == "" {
		httpsPort = ":443"
	}

	server := &http.Server{
		Addr:      httpsPort,
		Handler:   handler,
		TLSConfig: m.GetTLSConfig(),
	}

	log.Printf("🔒 HTTPS服务器启动在端口 %s", httpsPort)
	log.Printf("🌐 域名: %s", m.config.Domain)
	
	return server.ListenAndServeTLS("", "")
}

// ValidateDomain 验证域名配置
func (m *Manager) ValidateDomain() error {
	if m.config.Domain == "" {
		return fmt.Errorf("域名不能为空")
	}
	
	// 检查域名格式
	if len(m.config.Domain) < 3 || !contains(m.config.Domain, ".") {
		return fmt.Errorf("域名格式无效: %s", m.config.Domain)
	}
	
	return nil
}

// GetCertPath 获取证书文件路径
func (m *Manager) GetCertPath() string {
	return filepath.Join(m.config.CacheDir, m.config.Domain)
}

// contains 检查字符串是否包含子字符串
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}