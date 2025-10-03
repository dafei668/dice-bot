# 🔒 安全注意事项

## ⚠️ 重要提醒

本项目包含Telegram机器人的完整源代码，在使用前请注意以下安全事项：

### 🔑 Bot Token配置
1. **不要使用默认Token**：`.env`文件中的`BOT_TOKEN`需要替换为您自己的Token
2. **获取Token**：通过[@BotFather](https://t.me/botfather)创建新机器人并获取Token
3. **保护Token**：永远不要将真实Token提交到公共仓库

### 📁 敏感文件
- `.env.backup` - 包含原始配置（仅供参考，请勿使用）
- `data/` - 数据库文件目录
- `telegram-dice-bot` - 编译后的可执行文件

### 🛡️ 安全建议
1. 在生产环境中使用环境变量而不是`.env`文件
2. 定期更换Bot Token
3. 限制机器人的访问权限
4. 监控机器人的使用情况

### 🚀 快速开始
1. 复制`.env.example`到`.env`
2. 在`.env`中设置您的Bot Token
3. 运行`make build && ./telegram-dice-bot`

---
**注意**：本备份已移除真实的Bot Token以确保安全性。
