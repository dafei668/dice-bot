# Telegram Dice Bot 完整备份指南

## 📦 备份文件说明

### 1. 完整源码备份
- **文件**: `telegram-dice-bot-complete-YYYYMMDD-HHMMSS.tar.gz` (48KB)
- **内容**: 包含所有源码、配置文件、文档和测试文件
- **文件数量**: 41个文件
- **用途**: 用于在新服务器上完整部署

### 2. Git Bundle 备份
- **文件**: `telegram-dice-bot-git-YYYYMMDD-HHMMSS.bundle` (46KB)
- **内容**: 完整的Git仓库，包含版本历史
- **用途**: 保持版本控制历史，便于后续开发

## 📋 备份内容清单

### ✅ 包含的文件 (41个)
```
./                              # 项目根目录
├── .env                        # 环境配置文件
├── .env.example               # 环境配置模板
├── .gitignore                 # Git忽略文件
├── go.mod                     # Go模块文件
├── go.sum                     # Go依赖校验
├── main.go                    # 程序入口
├── Makefile                   # 构建脚本
├── migrate_db.sql             # 数据库迁移脚本
├── docker-compose.yml         # Docker编排文件
├── Dockerfile                 # Docker镜像构建文件
├── README.md                  # 项目说明
├── NETWORK_OPTIMIZATION.md    # 网络优化文档
├── DEPLOYMENT.md              # 部署指南
├── bin/                       # 二进制目录
├── data/                      # 数据目录
│   └── bot.db.backup         # 数据库备份
├── internal/                  # 内部包
│   ├── bot/
│   │   └── bot.go            # 机器人核心逻辑
│   ├── config/
│   │   └── config.go         # 配置管理
│   ├── database/
│   │   └── database.go       # 数据库操作
│   ├── game/
│   │   └── manager.go        # 游戏管理
│   ├── models/
│   │   └── models.go         # 数据模型
│   ├── network/              # 网络优化模块
│   │   ├── accelerator.go    # 网络加速器
│   │   ├── cache.go          # 响应缓存
│   │   ├── monitor.go        # 网络监控
│   │   ├── optimizer.go      # 网络优化器
│   │   └── retry.go          # 重试机制
│   ├── pool/
│   │   └── pool.go           # 连接池
│   └── utils/
│       └── utils.go          # 工具函数
└── test/                      # 测试文件
    ├── bot_test.go           # 机器人测试
    └── network_test.go       # 网络测试
```

### ❌ 排除的文件
- `telegram-dice-bot` (二进制文件)
- `bin/telegram-dice-bot` (二进制文件)
- `*.db` (数据库文件)
- `data/*.db` (运行时数据库)
- `.git/` (Git仓库文件，已单独打包)

## 🚀 在Mac上恢复部署

### 方法1: 使用tar.gz备份
```bash
# 1. 下载备份文件到Mac
scp root@your-server-ip:/root/telegram-dice-bot-complete-*.tar.gz ~/Downloads/

# 2. 解压到项目目录
cd ~/Projects
mkdir telegram-dice-bot
cd telegram-dice-bot
tar -xzf ~/Downloads/telegram-dice-bot-complete-*.tar.gz

# 3. 配置环境
cp .env.example .env
# 编辑 .env 文件，设置你的BOT_TOKEN

# 4. 安装Go依赖
go mod download

# 5. 构建项目
go build -o telegram-dice-bot .

# 6. 运行机器人
./telegram-dice-bot
```

### 方法2: 使用Git Bundle
```bash
# 1. 下载Git bundle到Mac
scp root@your-server-ip:/root/telegram-dice-bot-git-*.bundle ~/Downloads/

# 2. 从bundle克隆项目
cd ~/Projects
git clone ~/Downloads/telegram-dice-bot-git-*.bundle telegram-dice-bot
cd telegram-dice-bot

# 3. 后续步骤同方法1的3-6步
```

## 🔄 在新服务器上部署

### 使用tar.gz备份
```bash
# 1. 上传备份文件到新服务器
scp ~/Downloads/telegram-dice-bot-complete-*.tar.gz root@new-server-ip:/root/

# 2. 在新服务器上解压
ssh root@new-server-ip
cd /root
mkdir telegram-dice-bot
cd telegram-dice-bot
tar -xzf ../telegram-dice-bot-complete-*.tar.gz

# 3. 配置和部署
cp .env.example .env
# 编辑 .env 设置BOT_TOKEN
go mod download
go build -o telegram-dice-bot .
./telegram-dice-bot
```

## 📊 备份验证

### 验证备份完整性
```bash
# 检查文件数量
tar -tzf telegram-dice-bot-complete-*.tar.gz | wc -l
# 应该显示: 41

# 检查关键文件
tar -tzf telegram-dice-bot-complete-*.tar.gz | grep -E "(main.go|bot.go|go.mod)"
# 应该显示这些关键文件
```

### 验证Git Bundle
```bash
# 验证bundle文件
git bundle verify telegram-dice-bot-git-*.bundle
# 应该显示: The bundle contains these 2 refs
```

## 🛠️ 功能特性确认

部署后确认以下功能正常：

### 核心功能
- [x] 机器人启动 (`./telegram-dice-bot`)
- [x] 基本命令 (`/start`, `/help`)
- [x] 骰子游戏 (`/dice 100`)
- [x] 余额查询 (`/balance`)
- [x] 排行榜 (`/rank`)

### 网络优化功能
- [x] 网络加速器启动
- [x] 智能数据中心选择
- [x] 网络状态查询 (`/network`)
- [x] 延迟优化 (30-70%提升)
- [x] 智能重试机制
- [x] 响应缓存系统

## 🔒 安全注意事项

1. **保护敏感信息**
   - `.env` 文件包含在备份中，但需要手动设置BOT_TOKEN
   - 不要在公共场所存储包含真实Token的备份

2. **数据库安全**
   - 生产数据库文件已排除在备份外
   - 如需迁移用户数据，请单独备份 `data/bot.db`

3. **网络安全**
   - 确保新服务器的防火墙配置正确
   - 使用HTTPS连接Telegram API

## 📈 性能监控

部署后检查性能指标：
- 启动时间: < 5秒
- 内存使用: < 50MB
- 网络延迟: 通常 < 100ms
- 成功率: > 99.9%

## 🆘 故障排除

### 常见问题
1. **Go版本不兼容**: 确保Go版本 >= 1.19
2. **依赖下载失败**: 检查网络连接，使用 `go mod download -x` 查看详细信息
3. **权限问题**: 确保有执行权限 `chmod +x telegram-dice-bot`
4. **端口占用**: 检查8080端口是否被占用

### 日志分析
机器人启动时会显示详细状态：
```
🚀 网络加速器已启动
📍 最佳数据中心: DC1-Miami (Miami, US)
⚡ 延迟: 75ms
🔧 优化功能: HTTP/2, 连接池, 智能重试, 响应缓存, 质量监控
```

---

**备份创建时间**: $(date)
**备份文件位置**: `/root/telegram-dice-bot-complete-*.tar.gz` 和 `/root/telegram-dice-bot-git-*.bundle`
**项目版本**: 包含完整网络优化功能的生产版本