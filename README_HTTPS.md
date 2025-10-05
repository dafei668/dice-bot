# HTTPS 配置指南

本项目支持自动 HTTPS 证书申请和管理，使用 Let's Encrypt 提供免费的 SSL 证书。

## 配置步骤

### 1. 环境变量配置

在 `.env` 文件中添加以下配置：

```bash
# 启用 HTTPS
ENABLE_HTTPS=true

# 你的域名
DOMAIN=xn--9kq831b.live

# HTTPS 端口（默认 443）
HTTPS_PORT=443

# 证书缓存目录
CERT_CACHE_DIR=./certs

# 管理员邮箱（用于 Let's Encrypt 注册）
ADMIN_EMAIL=admin@xn--9kq831b.live
```

### 2. 域名解析

确保你的域名 `xn--9kq831b.live` 已正确解析到服务器 IP 地址：

```bash
# 检查域名解析
nslookup xn--9kq831b.live
```

### 3. 防火墙配置

确保服务器防火墙允许 80 和 443 端口：

```bash
# Ubuntu/Debian
sudo ufw allow 80
sudo ufw allow 443

# CentOS/RHEL
sudo firewall-cmd --permanent --add-port=80/tcp
sudo firewall-cmd --permanent --add-port=443/tcp
sudo firewall-cmd --reload
```

### 4. 启动服务

```bash
./telegram-dice-bot
```

## 工作原理

1. **自动证书申请**: 首次启动时，系统会自动向 Let's Encrypt 申请 SSL 证书
2. **HTTP 重定向**: 所有 HTTP 请求会自动重定向到 HTTPS
3. **证书续期**: 证书会在到期前自动续期
4. **证书缓存**: 证书保存在指定目录，重启后无需重新申请

## 访问地址

启用 HTTPS 后，管理后台地址为：
- HTTPS: https://xn--9kq831b.live/admin
- HTTP 会自动重定向到 HTTPS

## 故障排除

### 证书申请失败

1. 检查域名解析是否正确
2. 确保 80 端口可访问（Let's Encrypt 验证需要）
3. 检查防火墙设置
4. 查看日志输出的错误信息

### 证书目录权限

确保证书目录有正确的权限：

```bash
mkdir -p ./certs
chmod 755 ./certs
```

## 注意事项

1. Let's Encrypt 有速率限制，请勿频繁申请证书
2. 生产环境建议使用专用的证书目录
3. 定期备份证书文件
4. 监控证书到期时间