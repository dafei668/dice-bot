# 🔧 环境设置和依赖安装指南

## 📋 快速开始检查清单

在开始部署之前，请确保以下条件满足：

- [ ] **操作系统**: Linux (Ubuntu 20.04+, CentOS 8+, Debian 11+)
- [ ] **架构**: x86_64 (AMD64)
- [ ] **内存**: 最少 512MB (推荐 1GB+)
- [ ] **存储**: 最少 1GB 可用空间
- [ ] **网络**: 可访问 api.telegram.org
- [ ] **权限**: sudo 或 root 访问权限

---

## 🐧 按操作系统分类的安装指南

### Ubuntu/Debian 系统

#### 1. 系统更新
```bash
sudo apt update && sudo apt upgrade -y
```

#### 2. 安装基础工具
```bash
sudo apt install -y curl wget git build-essential
```

#### 3. 安装Go语言 (方法选择其一)

**方法A: 使用包管理器 (简单但可能版本较旧)**
```bash
sudo apt install -y golang-go
go version  # 检查版本是否 >= 1.21
```

**方法B: 手动安装最新版本 (推荐)**
```bash
# 下载Go 1.24.6
cd /tmp
wget https://go.dev/dl/go1.24.6.linux-amd64.tar.gz

# 安装Go
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.24.6.linux-amd64.tar.gz

# 设置环境变量
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
echo 'export GOPATH=$HOME/go' >> ~/.bashrc
echo 'export GOBIN=$GOPATH/bin' >> ~/.bashrc
source ~/.bashrc

# 验证安装
go version
```

#### 4. 安装SQLite
```bash
sudo apt install -y sqlite3 libsqlite3-dev
sqlite3 --version
```

---

### CentOS/RHEL/Rocky Linux 系统

#### 1. 系统更新
```bash
# CentOS 7
sudo yum update -y

# CentOS 8+ / Rocky Linux
sudo dnf update -y
```

#### 2. 安装基础工具
```bash
# CentOS 7
sudo yum groupinstall -y "Development Tools"
sudo yum install -y curl wget git

# CentOS 8+ / Rocky Linux
sudo dnf groupinstall -y "Development Tools"
sudo dnf install -y curl wget git
```

#### 3. 安装Go语言
```bash
# 下载Go 1.24.6
cd /tmp
wget https://go.dev/dl/go1.24.6.linux-amd64.tar.gz

# 安装Go
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.24.6.linux-amd64.tar.gz

# 设置环境变量
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
echo 'export GOPATH=$HOME/go' >> ~/.bashrc
source ~/.bashrc

# 验证安装
go version
```

#### 4. 安装SQLite
```bash
# CentOS 7
sudo yum install -y sqlite sqlite-devel

# CentOS 8+ / Rocky Linux
sudo dnf install -y sqlite sqlite-devel
```

---

### Arch Linux 系统

#### 1. 系统更新
```bash
sudo pacman -Syu
```

#### 2. 安装所需包
```bash
sudo pacman -S go git sqlite base-devel
```

#### 3. 验证安装
```bash
go version
sqlite3 --version
```

---

## 🔍 环境验证脚本

创建并运行以下脚本来验证环境是否正确设置：

```bash
cat > check_environment.sh << 'EOF'
#!/bin/bash

echo "🔍 检查系统环境..."
echo "=================================="

# 检查操作系统
echo "📋 操作系统信息:"
uname -a
echo ""

# 检查Go版本
echo "🐹 Go语言版本:"
if command -v go &> /dev/null; then
    GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
    echo "✅ Go版本: $GO_VERSION"
    
    # 检查版本是否满足要求 (>= 1.21)
    if [[ $(echo "$GO_VERSION 1.21" | tr " " "\n" | sort -V | head -n1) == "1.21" ]]; then
        echo "✅ Go版本满足要求 (>= 1.21)"
    else
        echo "❌ Go版本过低，需要 >= 1.21"
        exit 1
    fi
else
    echo "❌ Go未安装"
    exit 1
fi
echo ""

# 检查Go环境变量
echo "🔧 Go环境变量:"
echo "GOPATH: $GOPATH"
echo "GOROOT: $(go env GOROOT)"
echo "GOOS: $(go env GOOS)"
echo "GOARCH: $(go env GOARCH)"
echo ""

# 检查SQLite
echo "🗄️ SQLite版本:"
if command -v sqlite3 &> /dev/null; then
    echo "✅ SQLite版本: $(sqlite3 --version | awk '{print $1}')"
else
    echo "❌ SQLite未安装"
    exit 1
fi
echo ""

# 检查Git
echo "📚 Git版本:"
if command -v git &> /dev/null; then
    echo "✅ Git版本: $(git --version | awk '{print $3}')"
else
    echo "❌ Git未安装"
    exit 1
fi
echo ""

# 检查网络连接
echo "🌐 网络连接测试:"
if curl -s --connect-timeout 5 https://api.telegram.org > /dev/null; then
    echo "✅ 可以访问 Telegram API"
else
    echo "❌ 无法访问 Telegram API，请检查网络连接"
    exit 1
fi
echo ""

# 检查内存
echo "💾 系统资源:"
MEMORY_MB=$(free -m | awk 'NR==2{printf "%.0f", $2}')
echo "总内存: ${MEMORY_MB}MB"
if [ $MEMORY_MB -ge 512 ]; then
    echo "✅ 内存满足要求 (>= 512MB)"
else
    echo "⚠️ 内存可能不足，推荐至少512MB"
fi

# 检查磁盘空间
DISK_GB=$(df -BG . | awk 'NR==2 {print $4}' | sed 's/G//')
echo "可用磁盘空间: ${DISK_GB}GB"
if [ $DISK_GB -ge 1 ]; then
    echo "✅ 磁盘空间满足要求 (>= 1GB)"
else
    echo "❌ 磁盘空间不足，需要至少1GB"
    exit 1
fi
echo ""

echo "🎉 环境检查完成！所有要求都已满足。"
EOF

chmod +x check_environment.sh
./check_environment.sh
```

---

## 🔥 防火墙配置

### Ubuntu/Debian (UFW)
```bash
# 启用防火墙
sudo ufw enable

# 允许SSH (重要！)
sudo ufw allow ssh

# 允许机器人端口 (如果需要外部访问)
sudo ufw allow 8080

# 查看状态
sudo ufw status
```

### CentOS/RHEL (firewalld)
```bash
# 启动防火墙服务
sudo systemctl start firewalld
sudo systemctl enable firewalld

# 允许机器人端口
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --reload

# 查看状态
sudo firewall-cmd --list-all
```

---

## 🔐 安全配置建议

### 1. 创建专用用户
```bash
# 创建专用用户运行机器人
sudo useradd -r -s /bin/false -d /opt/telegram-dice-bot telegram-bot

# 设置目录权限
sudo mkdir -p /opt/telegram-dice-bot
sudo chown telegram-bot:telegram-bot /opt/telegram-dice-bot
```

### 2. 设置文件权限
```bash
# 设置配置文件权限 (只有所有者可读写)
chmod 600 .env

# 设置数据库文件权限
chmod 644 dice_bot.db

# 设置可执行文件权限
chmod 755 telegram-dice-bot
```

### 3. 系统限制配置
```bash
# 创建systemd服务限制
sudo tee /etc/systemd/system/telegram-dice-bot.service.d/limits.conf > /dev/null <<EOF
[Service]
# 内存限制 (256MB)
MemoryLimit=256M

# CPU限制 (50%)
CPUQuota=50%

# 文件描述符限制
LimitNOFILE=1024

# 禁止访问其他用户目录
ProtectHome=true

# 只读根文件系统
ProtectSystem=strict

# 允许写入的目录
ReadWritePaths=/opt/telegram-dice-bot
EOF
```

---

## 📊 性能优化建议

### 1. Go编译优化
```bash
# 使用优化编译
go build -ldflags="-s -w" -o telegram-dice-bot .

# 或者使用更激进的优化
CGO_ENABLED=0 GOOS=linux go build -a -ldflags="-s -w -extldflags '-static'" -o telegram-dice-bot .
```

### 2. 系统调优
```bash
# 增加文件描述符限制
echo "* soft nofile 65536" | sudo tee -a /etc/security/limits.conf
echo "* hard nofile 65536" | sudo tee -a /etc/security/limits.conf

# 优化网络参数
echo "net.core.somaxconn = 1024" | sudo tee -a /etc/sysctl.conf
echo "net.ipv4.tcp_max_syn_backlog = 1024" | sudo tee -a /etc/sysctl.conf
sudo sysctl -p
```

---

## 🚨 故障排除

### 常见问题解决方案

#### 1. Go版本问题
```bash
# 如果系统包管理器的Go版本过低
sudo apt remove golang-go  # Ubuntu
sudo yum remove go          # CentOS

# 然后手动安装最新版本 (参考上面的安装步骤)
```

#### 2. 权限问题
```bash
# 如果遇到权限错误
sudo chown -R $USER:$USER /opt/telegram-dice-bot
chmod -R 755 /opt/telegram-dice-bot
chmod 600 /opt/telegram-dice-bot/telegram-dice-bot/.env
```

#### 3. 网络连接问题
```bash
# 测试网络连接
curl -v https://api.telegram.org/bot<YOUR_TOKEN>/getMe

# 检查DNS解析
nslookup api.telegram.org

# 检查防火墙
sudo iptables -L
```

#### 4. 内存不足
```bash
# 创建交换文件 (临时解决方案)
sudo fallocate -l 1G /swapfile
sudo chmod 600 /swapfile
sudo mkswap /swapfile
sudo swapon /swapfile

# 永久启用
echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab
```

---

## ✅ 环境准备完成检查

完成环境设置后，请确认以下项目：

- [ ] Go版本 >= 1.21
- [ ] SQLite已安装
- [ ] Git已安装
- [ ] 网络可访问 api.telegram.org
- [ ] 防火墙配置正确
- [ ] 用户权限设置合适
- [ ] 系统资源充足 (内存 >= 512MB, 磁盘 >= 1GB)

**环境准备完成后，即可开始部署机器人！** 🚀