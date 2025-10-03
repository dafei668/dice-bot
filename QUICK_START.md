# ⚡ 快速开始指南

> 🎯 **目标**: 在5分钟内完成Telegram骰子机器人的部署

---

## 🚀 一键部署脚本

### Ubuntu/Debian 系统
```bash
#!/bin/bash
# 一键部署脚本 - Ubuntu/Debian

set -e  # 遇到错误立即退出

echo "🚀 开始一键部署 Telegram 骰子机器人..."

# 1. 更新系统
echo "📦 更新系统包..."
sudo apt update && sudo apt upgrade -y

# 2. 安装依赖
echo "🔧 安装依赖..."
sudo apt install -y curl wget git build-essential sqlite3 libsqlite3-dev

# 3. 安装Go (如果未安装)
if ! command -v go &> /dev/null; then
    echo "🐹 安装Go语言..."
    cd /tmp
    wget -q https://go.dev/dl/go1.24.6.linux-amd64.tar.gz
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf go1.24.6.linux-amd64.tar.gz
    echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
    export PATH=$PATH:/usr/local/go/bin
fi

# 4. 验证环境
echo "✅ 验证环境..."
go version
sqlite3 --version

# 5. 解压备份文件 (假设在当前目录)
echo "📂 解压备份文件..."
tar -xzf telegram-dice-bot-COMPLETE-*.tar.gz
cd telegram-dice-bot

# 6. 配置环境变量
echo "⚙️ 配置环境变量..."
echo "请输入您的Telegram Bot Token:"
read -s BOT_TOKEN
sed -i "s/BOT_TOKEN=.*/BOT_TOKEN=$BOT_TOKEN/" .env

# 7. 编译和启动
echo "🔨 编译项目..."
go mod download
go build -o telegram-dice-bot .

echo "🎉 部署完成！启动机器人..."
./telegram-dice-bot

echo "✅ 机器人已启动！请在Telegram中测试 /start 命令"
```

### CentOS/RHEL 系统
```bash
#!/bin/bash
# 一键部署脚本 - CentOS/RHEL

set -e

echo "🚀 开始一键部署 Telegram 骰子机器人..."

# 1. 更新系统
echo "📦 更新系统包..."
sudo dnf update -y

# 2. 安装依赖
echo "🔧 安装依赖..."
sudo dnf groupinstall -y "Development Tools"
sudo dnf install -y curl wget git sqlite sqlite-devel

# 3. 安装Go
if ! command -v go &> /dev/null; then
    echo "🐹 安装Go语言..."
    cd /tmp
    wget -q https://go.dev/dl/go1.24.6.linux-amd64.tar.gz
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf go1.24.6.linux-amd64.tar.gz
    echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
    export PATH=$PATH:/usr/local/go/bin
fi

# 4. 验证环境
echo "✅ 验证环境..."
go version
sqlite3 --version

# 5. 解压备份文件
echo "📂 解压备份文件..."
tar -xzf telegram-dice-bot-COMPLETE-*.tar.gz
cd telegram-dice-bot

# 6. 配置环境变量
echo "⚙️ 配置环境变量..."
echo "请输入您的Telegram Bot Token:"
read -s BOT_TOKEN
sed -i "s/BOT_TOKEN=.*/BOT_TOKEN=$BOT_TOKEN/" .env

# 7. 编译和启动
echo "🔨 编译项目..."
go mod download
go build -o telegram-dice-bot .

echo "🎉 部署完成！启动机器人..."
./telegram-dice-bot
```

---

## 📋 手动部署 (3步骤)

### 步骤 1: 环境准备
```bash
# Ubuntu/Debian
sudo apt update && sudo apt install -y golang-go git sqlite3

# CentOS/RHEL
sudo dnf install -y go git sqlite
```

### 步骤 2: 部署代码
```bash
# 解压备份
tar -xzf telegram-dice-bot-COMPLETE-*.tar.gz
cd telegram-dice-bot

# 配置Token
nano .env  # 修改 BOT_TOKEN=your_token_here
```

### 步骤 3: 启动服务
```bash
# 编译
go build -o telegram-dice-bot .

# 启动
./telegram-dice-bot
```

---

## 🔧 Docker 部署 (推荐)

### 创建 Dockerfile
```dockerfile
FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY . .
RUN go mod download
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o telegram-dice-bot .

FROM alpine:latest
RUN apk --no-cache add ca-certificates sqlite
WORKDIR /root/
COPY --from=builder /app/telegram-dice-bot .
COPY --from=builder /app/.env .
COPY --from=builder /app/dice_bot.db .

EXPOSE 8080
CMD ["./telegram-dice-bot"]
```

### 构建和运行
```bash
# 构建镜像
docker build -t telegram-dice-bot .

# 运行容器
docker run -d \
  --name dice-bot \
  -p 8080:8080 \
  -e BOT_TOKEN=your_token_here \
  -v $(pwd)/dice_bot.db:/root/dice_bot.db \
  telegram-dice-bot
```

---

## 🎯 验证部署

### 1. 检查服务状态
```bash
# 检查进程
ps aux | grep telegram-dice-bot

# 检查端口
netstat -tlnp | grep 8080

# 检查日志
tail -f /var/log/telegram-dice-bot.log
```

### 2. 测试机器人功能
在Telegram中发送以下命令：
- `/start` - 开始使用机器人
- `/help` - 查看帮助信息
- `/balance` - 查看余额
- `/dice 10` - 投掷骰子，下注10积分

### 3. 健康检查
```bash
# API健康检查
curl http://localhost:8080/health

# 数据库检查
sqlite3 dice_bot.db "SELECT COUNT(*) FROM users;"
```

---

## 🚨 常见问题快速解决

### 问题1: "command not found: go"
```bash
# 解决方案
export PATH=$PATH:/usr/local/go/bin
source ~/.bashrc
```

### 问题2: "permission denied"
```bash
# 解决方案
chmod +x telegram-dice-bot
sudo chown $USER:$USER telegram-dice-bot
```

### 问题3: "database locked"
```bash
# 解决方案
pkill telegram-dice-bot
rm -f dice_bot.db-wal dice_bot.db-shm
./telegram-dice-bot
```

### 问题4: "network unreachable"
```bash
# 解决方案
curl -I https://api.telegram.org  # 测试网络
sudo ufw allow out 443            # 允许HTTPS出站
```

---

## 📞 获取帮助

- **文档**: 查看 `NEW_SERVER_DEPLOYMENT.md` 获取详细部署指南
- **环境**: 查看 `ENVIRONMENT_SETUP.md` 获取环境配置帮助
- **问题**: 检查日志文件 `/var/log/telegram-dice-bot.log`

---

## ✅ 部署成功标志

当您看到以下输出时，表示部署成功：

```
🎲 Telegram Dice Bot Starting...
✅ Configuration loaded successfully
✅ Database connected: dice_bot.db
✅ Bot initialized successfully
🚀 Server starting on port 8080
✅ Bot is running! Send /start to begin
```

**恭喜！您的Telegram骰子机器人已成功部署！** 🎉