# 🚀 Telegram骰子机器人 - 新服务器部署完整指南

## 📋 目录
1. [系统环境要求](#系统环境要求)
2. [环境准备](#环境准备)
3. [备份文件部署](#备份文件部署)
4. [配置设置](#配置设置)
5. [启动和测试](#启动和测试)
6. [故障排除](#故障排除)
7. [维护和监控](#维护和监控)

---

## 🖥️ 系统环境要求

### 最低系统要求
- **操作系统**: Ubuntu 20.04+ / CentOS 8+ / Debian 11+
- **架构**: x86_64 (AMD64)
- **内存**: 最少 512MB RAM (推荐 1GB+)
- **存储**: 最少 1GB 可用空间
- **网络**: 稳定的互联网连接

### 必需软件版本
- **Go语言**: 1.21+ (推荐 1.24.6)
- **SQLite**: 3.x (通常系统自带)
- **Git**: 2.x+ (用于版本控制)

---

## 🔧 环境准备

### 1. 更新系统包
```bash
# Ubuntu/Debian
sudo apt update && sudo apt upgrade -y

# CentOS/RHEL
sudo yum update -y
# 或者 (CentOS 8+)
sudo dnf update -y
```

### 2. 安装Go语言环境

#### Ubuntu/Debian 安装方式
```bash
# 方法1: 使用官方包管理器 (可能版本较旧)
sudo apt install golang-go

# 方法2: 安装最新版本 (推荐)
wget https://go.dev/dl/go1.24.6.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.24.6.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

#### CentOS/RHEL 安装方式
```bash
# 安装最新版本
wget https://go.dev/dl/go1.24.6.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.24.6.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

### 3. 验证Go安装
```bash
go version
# 应该显示: go version go1.24.6 linux/amd64
```

### 4. 安装必要工具
```bash
# Ubuntu/Debian
sudo apt install -y git sqlite3 curl wget

# CentOS/RHEL
sudo yum install -y git sqlite curl wget
# 或者
sudo dnf install -y git sqlite curl wget
```

---

## 📦 备份文件部署

### 1. 创建项目目录
```bash
# 创建应用目录
sudo mkdir -p /opt/telegram-dice-bot
sudo chown $USER:$USER /opt/telegram-dice-bot
cd /opt/telegram-dice-bot
```

### 2. 解压备份文件
```bash
# 上传备份文件到服务器 (使用scp、rsync或其他方式)
# 假设备份文件已上传到 /tmp/

# 解压主备份文件
tar -xzf /tmp/telegram-dice-bot-COMPLETE-*.tar.gz
cd telegram-dice-bot

# 验证文件完整性
ls -la
# 应该看到: main.go, internal/, .env, dice_bot.db 等文件
```

### 3. 恢复Git仓库 (可选)
```bash
# 如果需要版本控制，恢复Git仓库
git clone /tmp/telegram-dice-bot-COMPLETE-git-*.bundle .git-restored
cd .git-restored
# 或者直接在项目目录初始化
git init
git remote add origin /tmp/telegram-dice-bot-COMPLETE-git-*.bundle
git fetch origin
git checkout master
```

---

## ⚙️ 配置设置

### 1. 配置环境变量
```bash
# 编辑 .env 文件
nano .env

# 必须修改的配置项:
BOT_TOKEN=YOUR_NEW_BOT_TOKEN_HERE

# 可选配置项 (根据需要调整):
DATABASE_URL=./dice_bot.db
PORT=8080
COMMISSION_RATE=0.1
MIN_BET=1
MAX_BET=10000
```

### 2. 获取Telegram Bot Token
1. 在Telegram中找到 @BotFather
2. 发送 `/newbot` 创建新机器人
3. 按提示设置机器人名称和用户名
4. 复制获得的Token到 `.env` 文件中

### 3. 设置数据库权限
```bash
# 确保数据库文件权限正确
chmod 644 dice_bot.db
chown $USER:$USER dice_bot.db

# 验证数据库完整性
sqlite3 dice_bot.db "PRAGMA integrity_check;"
# 应该返回: ok
```

---

## 🚀 启动和测试

### 1. 下载依赖并编译
```bash
# 下载Go模块依赖
go mod download

# 验证模块完整性
go mod verify

# 编译项目
go build -o telegram-dice-bot .

# 验证编译结果
ls -la telegram-dice-bot
# 应该看到可执行文件
```

### 2. 运行测试
```bash
# 运行测试套件
go test ./... -v

# 测试配置加载
echo 'package main
import (
    "fmt"
    "telegram-dice-bot/internal/config"
)
func main() {
    cfg, err := config.Load()
    if err != nil {
        fmt.Printf("配置加载失败: %v\n", err)
        return
    }
    fmt.Printf("✅ Bot Token: %s\n", cfg.BotToken[:10]+"...")
    fmt.Printf("✅ 数据库: %s\n", cfg.DatabaseURL)
    fmt.Printf("✅ 端口: %s\n", cfg.Port)
}' > test_config.go

go run test_config.go
rm test_config.go
```

### 3. 启动机器人
```bash
# 前台运行 (测试用)
./telegram-dice-bot

# 后台运行 (生产环境)
nohup ./telegram-dice-bot > bot.log 2>&1 &

# 查看运行状态
ps aux | grep telegram-dice-bot
```

### 4. 验证功能
1. 在Telegram中找到你的机器人
2. 发送 `/start` 命令
3. 测试骰子游戏功能
4. 检查日志文件确认无错误

---

## 🔧 故障排除

### 常见问题及解决方案

#### 1. Go版本过低
```bash
# 错误: go: directive requires go 1.21 or later
# 解决: 升级Go版本到1.21+
```

#### 2. Bot Token无效
```bash
# 错误: 401 Unauthorized
# 解决: 检查.env文件中的BOT_TOKEN是否正确
```

#### 3. 数据库权限问题
```bash
# 错误: database is locked
# 解决:
sudo chown $USER:$USER dice_bot.db
chmod 644 dice_bot.db
```

#### 4. 端口被占用
```bash
# 错误: bind: address already in use
# 解决: 修改.env中的PORT或停止占用端口的进程
sudo netstat -tlnp | grep :8080
sudo kill -9 <PID>
```

#### 5. 网络连接问题
```bash
# 测试网络连接
curl -I https://api.telegram.org
# 如果失败，检查防火墙和网络设置
```

---

## 📊 维护和监控

### 1. 系统服务配置 (推荐)
```bash
# 创建systemd服务文件
sudo tee /etc/systemd/system/telegram-dice-bot.service > /dev/null <<EOF
[Unit]
Description=Telegram Dice Bot
After=network.target

[Service]
Type=simple
User=$USER
WorkingDirectory=/opt/telegram-dice-bot/telegram-dice-bot
ExecStart=/opt/telegram-dice-bot/telegram-dice-bot/telegram-dice-bot
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

# 启用并启动服务
sudo systemctl daemon-reload
sudo systemctl enable telegram-dice-bot
sudo systemctl start telegram-dice-bot

# 查看服务状态
sudo systemctl status telegram-dice-bot
```

### 2. 日志监控
```bash
# 查看实时日志
sudo journalctl -u telegram-dice-bot -f

# 查看历史日志
sudo journalctl -u telegram-dice-bot --since "1 hour ago"
```

### 3. 数据库备份
```bash
# 创建定期备份脚本
cat > backup_db.sh << 'EOF'
#!/bin/bash
BACKUP_DIR="/opt/backups/telegram-dice-bot"
DATE=$(date +%Y%m%d_%H%M%S)
mkdir -p $BACKUP_DIR
cp /opt/telegram-dice-bot/telegram-dice-bot/dice_bot.db $BACKUP_DIR/dice_bot_$DATE.db
# 保留最近7天的备份
find $BACKUP_DIR -name "dice_bot_*.db" -mtime +7 -delete
EOF

chmod +x backup_db.sh

# 添加到crontab (每天凌晨2点备份)
echo "0 2 * * * /opt/telegram-dice-bot/backup_db.sh" | crontab -
```

### 4. 性能监控
```bash
# 监控进程资源使用
top -p $(pgrep telegram-dice-bot)

# 监控数据库大小
ls -lh dice_bot.db

# 检查网络连接
ss -tlnp | grep telegram-dice-bot
```

---

## ✅ 部署检查清单

- [ ] 系统环境满足要求
- [ ] Go语言环境安装完成 (1.21+)
- [ ] 备份文件解压成功
- [ ] .env文件配置正确
- [ ] Bot Token已更新
- [ ] 数据库权限设置正确
- [ ] 依赖下载和编译成功
- [ ] 测试套件运行通过
- [ ] 机器人启动成功
- [ ] Telegram功能测试正常
- [ ] 系统服务配置完成
- [ ] 日志监控设置完成
- [ ] 数据库备份策略实施

---

## 📞 技术支持

如果在部署过程中遇到问题:

1. 检查系统日志: `sudo journalctl -u telegram-dice-bot`
2. 验证配置文件: 确保.env文件格式正确
3. 测试网络连接: `curl -I https://api.telegram.org`
4. 检查Go环境: `go version` 和 `go env`
5. 验证文件权限: `ls -la dice_bot.db`

---

**部署完成后，你的Telegram骰子机器人将与原服务器完全相同地运行！** 🎉