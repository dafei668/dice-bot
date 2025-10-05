# Telegram 骰子机器人

一个功能完整的 Telegram 骰子游戏机器人，支持 1v1 比大小游戏，具有可验证的随机性和完整的用户管理系统。

## 🎯 功能特性

- **1v1 骰子游戏**: 玩家可以发起游戏，其他玩家可以加入
- **可验证随机性**: 使用加密种子生成可验证的骰子结果
- **动画骰子**: 使用 Telegram 原生骰子动画
- **用户管理**: 完整的用户注册、余额管理系统
- **交易记录**: 详细的游戏和交易历史
- **并发优化**: 工作池、速率限制、缓存等性能优化
- **Docker 支持**: 完整的容器化部署方案

## 🎮 游戏规则

1. 玩家使用 `/dice <金额>` 发起游戏
2. 其他玩家使用 `/join <游戏ID>` 加入游戏
3. 双方自动投掷骰子，点数大的获胜
4. 平台收取 10% 手续费
5. 新用户自动获得 10 初始余额

## 📋 命令列表

- `/start` - 显示欢迎信息和游戏规则
- `/help` - 显示帮助信息
- `/dice <金额>` - 发起骰子游戏
- `/join <游戏ID>` - 加入游戏
- `/balance` - 查看余额
- `/games` - 查看等待中的游戏

## 🚀 快速开始

### 环境要求

- Go 1.19+
- SQLite3
- Docker (可选)

### 本地运行

1. **克隆项目**
   ```bash
   git clone <repository-url>
   cd telegram-dice-bot
   ```

2. **安装依赖**
   ```bash
   go mod tidy
   ```

3. **配置环境变量**
   ```bash
   cp .env.example .env
   # 编辑 .env 文件，设置你的 BOT_TOKEN
   ```

4. **构建并运行**
   ```bash
   make build
   make run
   ```

### Docker 部署

1. **构建镜像**
   ```bash
   make docker-build
   ```

2. **启动服务**
   ```bash
   make docker-run
   ```

3. **查看日志**
   ```bash
   make docker-logs
   ```

## 🔧 配置说明

### 环境变量

| 变量名 | 描述 | 默认值 |
|--------|------|--------|
| `BOT_TOKEN` | Telegram Bot Token | 必填 |
| `DATABASE_URL` | 数据库文件路径 | `./data/bot.db` |
| `PORT` | 服务端口 | `8080` |
| `COMMISSION_RATE` | 平台手续费率 | `0.1` (10%) |
| `MIN_BET` | 最小下注金额 | `1` |
| `MAX_BET` | 最大下注金额 | `100` |

### 获取 Bot Token

1. 在 Telegram 中找到 [@BotFather](https://t.me/botfather)
2. 发送 `/newbot` 创建新机器人
3. 按提示设置机器人名称和用户名
4. 获取 Bot Token 并设置到环境变量中

## 使用示例

```
# 发起游戏
/dice 100

# 加入游戏
/join GAME1672531200001

# 查看余额
/balance

# 查看等待的游戏
/games
```

## 技术特点

- **高性能**: 使用 Go 语言开发，支持高并发
- **低资源**: 内存占用小，CPU 使用率低
- **安全随机**: 使用加密安全的随机数生成器
- **数据持久**: SQLite 数据库存储游戏数据
- **并发安全**: 使用互斥锁保证数据一致性

## 项目结构

```
telegram-dice-bot/
├── main.go                 # 主程序入口
├── internal/
│   ├── bot/                # Telegram Bot 逻辑
│   ├── config/             # 配置管理
│   ├── database/           # 数据库操作
│   ├── game/               # 游戏逻辑
│   ├── models/             # 数据模型
│   └── utils/              # 工具函数
├── .env.example            # 环境变量示例
└── README.md               # 项目说明
```

## 开发说明

### 数据库表结构

- `users`: 用户信息和余额
- `games`: 游戏记录
- `transactions`: 交易记录

### 游戏流程

1. 用户发起游戏 -> 扣除下注金额 -> 创建游戏记录
2. 其他用户加入 -> 扣除下注金额 -> 开始游戏
3. 投掷骰子 -> 计算结果 -> 分配奖金 -> 记录交易

## 许可证

MIT License

## 贡献

欢迎提交 Issue 和 Pull Request！