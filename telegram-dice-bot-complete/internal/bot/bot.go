package bot

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"telegram-dice-bot/internal/config"
	"telegram-dice-bot/internal/database"
	"telegram-dice-bot/internal/game"
	"telegram-dice-bot/internal/models"
	"telegram-dice-bot/internal/network"
	"telegram-dice-bot/internal/pool"
)

// Bot Telegram机器人结构
type Bot struct {
	api           *tgbotapi.BotAPI
	db            *database.DB
	gameManager   *game.Manager
	config        *config.Config
	workerPool    *pool.WorkerPool
	rateLimiter   *pool.RateLimiter
	cache         *pool.Cache
	objectPool    *pool.ObjectPool
	accelerator   *network.NetworkAccelerator // 网络加速器
	userMutex     sync.Map                    // 用户级别的互斥锁，防止同一用户并发操作
	gameStates    sync.Map                    // 用户游戏状态，防止游戏进行中的重复操作
	activeGames   sync.Map                    // 群组活跃游戏状态，记录哪些群正在进行游戏 chatID -> gameID
	gameQueue     sync.Map                    // 群组游戏队列，记录等待中的游戏 chatID -> []GameRequest
	queueMutex    sync.Map                    // 队列操作互斥锁 chatID -> *sync.Mutex

	// 清理机制相关
	lastActivity  sync.Map     // 记录每个群组的最后活动时间 chatID -> time.Time
	cleanupTicker *time.Ticker // 定期清理定时器
}

// GameRequest 游戏请求结构
type GameRequest struct {
	UserID   int64
	Username string
	Amount   float64
	ChatID   int64
	Time     time.Time
}

// createUserCallbackData 创建带用户ID的回调数据
func (b *Bot) createUserCallbackData(action string, userID int64) string {
	return fmt.Sprintf("%s_%d", action, userID)
}

// needsUserValidation 检查回调数据是否需要用户验证
func (b *Bot) needsUserValidation(callbackData string) bool {
	// 立即应战按钮不需要验证，任何人都可以点击
	if strings.HasPrefix(callbackData, "join_") {
		return false
	}

	// 固定指令（/start, /help等）不需要验证，即使带用户ID
	if strings.HasPrefix(callbackData, "start_") || strings.HasPrefix(callbackData, "help_") ||
		strings.HasPrefix(callbackData, "balance_") || strings.HasPrefix(callbackData, "games_") ||
		strings.HasPrefix(callbackData, "quick_game_") || strings.HasPrefix(callbackData, "custom_amount_") ||
		strings.HasPrefix(callbackData, "show_amount_options_") || strings.HasPrefix(callbackData, "dice_") {
		return false
	}

	// 其他所有带用户ID的操作都需要验证
	return strings.Contains(callbackData, "_")
}

// validateUserCallback 验证回调是否来自正确的用户
// validateUserCallback 验证回调是否来自正确的用户
func (b *Bot) validateUserCallback(callbackData string, userID int64) bool {
	// 如果回调数据不包含用户ID，则不需要验证（通用按钮）
	if !strings.Contains(callbackData, "_") {
		return true
	}

	// 解析回调数据中的用户ID
	parts := strings.Split(callbackData, "_")
	if len(parts) < 2 {
		return true // 没有用户ID信息，允许操作
	}

	// 获取最后一部分作为用户ID
	lastPart := parts[len(parts)-1]
	expectedUserID, err := strconv.ParseInt(lastPart, 10, 64)
	if err != nil {
		return true // 解析失败，可能不是用户ID，允许操作
	}

	return expectedUserID == userID
}

// getUsernameDisplay 获取用户显示名称
func (b *Bot) getUsernameDisplay(user *tgbotapi.User) string {
	// 优先显示用户的真实姓名（昵称）
	if user.FirstName != "" {
		displayName := user.FirstName
		if user.LastName != "" {
			displayName += " " + user.LastName
		}
		return displayName
	}
	// 如果没有昵称，才使用用户名（不带@符号）
	if user.UserName != "" {
		return user.UserName
	}
	// 最后的备选方案
	return fmt.Sprintf("用户%d", user.ID)
}

// NewBot 创建新的机器人实例
func NewBot(cfg *config.Config, db *database.DB, gameManager *game.Manager) (*Bot, error) {
	// 创建网络加速器
	accelerator := network.NewNetworkAccelerator()

	// 创建BotAPI实例
	api, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		return nil, fmt.Errorf("创建机器人API失败: %v", err)
	}

	// 初始化网络加速器
	err = accelerator.InitializeWithBot(api)
	if err != nil {
		return nil, fmt.Errorf("初始化网络加速器失败: %v", err)
	}

	// 创建工作池（CPU核心数的2倍工作者，队列大小1000）
	workerPool := pool.NewWorkerPool(0, 1000)

	// 创建速率限制器（每秒30个请求，符合Telegram API限制）
	rateLimiter := pool.NewRateLimiter(30, time.Second)

	// 创建缓存
	cache := pool.NewCache()

	// 创建对象池
	objectPool := pool.NewObjectPool()

	bot := &Bot{
		api:           api,
		db:            db,
		gameManager:   gameManager,
		config:        cfg,
		workerPool:    workerPool,
		rateLimiter:   rateLimiter,
		cache:         cache,
		objectPool:    objectPool,
		accelerator:   accelerator,
		cleanupTicker: time.NewTicker(30 * time.Minute), // 每30分钟清理一次
	}

	// 启动清理协程
	go bot.startCleanupRoutine()

	return bot, nil
}

// startCleanupRoutine 启动定期清理协程
func (b *Bot) startCleanupRoutine() {
	for range b.cleanupTicker.C {
		b.performCleanup()
	}
}

// performCleanup 执行清理操作
func (b *Bot) performCleanup() {
	now := time.Now()
	inactiveThreshold := 2 * time.Hour // 2小时无活动视为不活跃

	// 清理不活跃群组的资源
	b.lastActivity.Range(func(key, value interface{}) bool {
		chatID := key.(int64)
		lastActivity := value.(time.Time)

		if now.Sub(lastActivity) > inactiveThreshold {
			// 清理该群组的所有资源
			b.gameQueue.Delete(chatID)
			b.queueMutex.Delete(chatID)
			b.activeGames.Delete(chatID)
			b.lastActivity.Delete(chatID)
			log.Printf("清理不活跃群组资源: %d", chatID)
		}
		return true
	})
}

// updateActivity 更新群组活动时间
func (b *Bot) updateActivity(chatID int64) {
	b.lastActivity.Store(chatID, time.Now())
}

// handleGameError 处理游戏错误的统一方法
func (b *Bot) handleGameError(chatID int64, gameID string, errorType string, err error) {
	log.Printf("游戏错误 (GameID: %s, ChatID: %d, Type: %s): %v", gameID, chatID, errorType, err)

	// 发送用户友好的错误消息
	var userMessage string
	switch errorType {
	case "骰子投掷失败":
		userMessage = "❌ 骰子投掷失败，游戏取消。请稍后重试。"
	case "游戏结算失败":
		userMessage = "❌ 游戏结算失败，请联系管理员。"
	default:
		userMessage = "❌ 游戏出现错误，已自动取消。"
	}

	b.sendMessage(chatID, userMessage)

	// 记录错误以便后续分析和处理
	log.Printf("游戏错误详情 - GameID: %s, ChatID: %d, Error: %v", gameID, chatID, err)
}

// OnGameExpired 处理游戏超时通知
func (b *Bot) OnGameExpired(gameID string, chatID int64) {
	message := `⏰ 对决超时提醒

🆔 对决编号：%s
⌛ 等待时间已超过60秒，对决已自动关闭
💰 对决金额已自动退还

💡 温馨提示：发起对决后请及时分享给朋友，或在群内@其他成员参与激战！`

	b.sendMessage(chatID, fmt.Sprintf(message, gameID))
}

// Start 启动机器人
func (b *Bot) Start() error {
	log.Printf("机器人启动中...")

	// 启动工作池
	b.workerPool.Start()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	log.Printf("机器人已启动，用户名: %s", b.api.Self.UserName)

	for update := range updates {
		// 使用工作池处理更新，避免阻塞
		job := &pool.MessageJob{
			Handler: func() error {
				return b.handleUpdate(update)
			},
		}
		b.workerPool.Submit(job)
	}

	return nil
}

// Stop 停止机器人
func (b *Bot) Stop() {
	log.Printf("机器人停止中...")
	b.api.StopReceivingUpdates()
	b.workerPool.Stop()
	b.rateLimiter.Stop()

	// 停止清理定时器以防止内存泄漏
	if b.cleanupTicker != nil {
		b.cleanupTicker.Stop()
	}

	log.Printf("机器人已停止")
}

// handleUpdate 处理更新
func (b *Bot) handleUpdate(update tgbotapi.Update) error {
	var chatID int64

	if update.Message != nil {
		chatID = update.Message.Chat.ID
		b.updateActivity(chatID)
		return b.handleMessage(update.Message)
	} else if update.CallbackQuery != nil {
		chatID = update.CallbackQuery.Message.Chat.ID
		b.updateActivity(chatID)
		b.handleCallbackQuery(update.CallbackQuery)
	}
	return nil
}

// handleMessage 处理消息
func (b *Bot) handleMessage(message *tgbotapi.Message) error {
	// 获取用户级别的互斥锁
	userID := message.From.ID
	mutex, _ := b.userMutex.LoadOrStore(userID, &sync.Mutex{})
	userMutex := mutex.(*sync.Mutex)

	userMutex.Lock()
	defer userMutex.Unlock()

	// 确保用户存在
	if err := b.ensureUserExists(message.From); err != nil {
		log.Printf("确保用户存在失败: %v", err)
		return err
	}

	// 只在群组中处理消息
	if !message.Chat.IsGroup() && !message.Chat.IsSuperGroup() {
		b.sendMessage(message.Chat.ID, "🎲 骰子机器人只能在群组中使用！请将我添加到群组中。")
		return nil
	}

	// 检查群组是否有活跃游戏，如果有则删除非命令消息
	if _, hasActiveGame := b.activeGames.Load(message.Chat.ID); hasActiveGame {
		if !message.IsCommand() {
			// 删除用户在游戏进行中发送的消息
			deleteMsg := tgbotapi.NewDeleteMessage(message.Chat.ID, message.MessageID)
			if _, err := b.api.Request(deleteMsg); err != nil {
				// 如果删除失败，记录日志但不影响程序运行
				log.Printf("删除消息失败 (ChatID: %d, MessageID: %d): %v", message.Chat.ID, message.MessageID, err)
				// 可能是权限不足，发送提示消息给管理员
				if strings.Contains(err.Error(), "not enough rights") || strings.Contains(err.Error(), "CHAT_ADMIN_REQUIRED") {
					b.sendMessage(message.Chat.ID, "⚠️ 机器人需要删除消息权限才能在游戏中保持环境整洁。请将机器人设为管理员并给予删除消息权限。")
				}
			}
			return nil
		}
	}

	if message.IsCommand() {
		return b.handleCommand(message)
	}

	return nil
}

// handleCommand 处理命令
func (b *Bot) handleCommand(message *tgbotapi.Message) error {
	command := message.Command()
	args := message.CommandArguments()

	switch command {
	case "start":
		b.handleStart(message)
	case "help":
		b.handleHelp(message)
	case "balance":
		b.handleBalance(message)
	case "dice":
		b.handleDice(message, args)
	case "join":
		b.handleJoin(message, args)
	case "games":
		b.handleGames(message)
	case "checkperm":
		b.handleCheckPermissions(message)
	case "network":
		b.handleNetworkStatus(message)
	default:
		b.sendMessage(message.Chat.ID, "❓ 未知命令。使用 /help 查看可用命令。")
	}

	return nil
}

// handleCheckPermissions 处理权限检查命令
func (b *Bot) handleCheckPermissions(message *tgbotapi.Message) {
	if err := b.checkBotPermissions(message.Chat.ID); err != nil {
		errorMsg := fmt.Sprintf(`❌ 权限检查失败！

🔍 检查结果：%s

📋 所需权限：
• 机器人必须是群组管理员
• 必须有删除消息权限

🛠️ 解决方法：
1. 在群组设置中将机器人设为管理员
2. 确保勾选"删除消息"权限
3. 重新运行 /checkperm 验证

⚠️ 没有足够权限时，游戏中的消息清理功能将无法正常工作。`, err.Error())

		b.sendMessage(message.Chat.ID, errorMsg)
	} else {
		successMsg := `✅ 权限检查通过！

🎉 机器人权限状态：
• ✅ 管理员权限：已获得
• ✅ 删除消息权限：已获得

🎮 所有功能已就绪：
• 游戏中自动清理干扰消息
• 按钮操作限制功能
• 完整的游戏体验

🚀 现在可以开始畅快游戏了！`

		b.sendMessage(message.Chat.ID, successMsg)
	}
}

func (b *Bot) handleStart(message *tgbotapi.Message) {
	userDisplay := b.getUsernameDisplay(message.From)
	text := fmt.Sprintf(`🎰 欢迎进入终极财富战场！🎰

💥 这里是勇者的天堂，懦夫的地狱！💥

👤 当前玩家：%s

🔥 游戏特色：
• ⚡ 肾上腺素爆表的1v1生死对决！
• 💎 每一次投掷都可能改变你的命运！
• 🚀 瞬间暴富，一夜成名的机会就在眼前！
• 🎊 简单粗暴，3秒上手，一生上瘾！

💰 财富密码已解锁：
选择下方金额，开启你的逆袭之路！
每一次点击都可能是你人生的转折点！

⚠️ 警告：此游戏极度上瘾，请做好暴富准备！

🎁 新手特权：注册即送1000金币！立即开始你的财富传奇！`, userDisplay)

	// 创建内联键盘，包含用户ID
	userID := message.From.ID
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💀 生死对决 (10)", b.createUserCallbackData("dice_10", userID)),
			tgbotapi.NewInlineKeyboardButtonData("💀 生死对决 (50)", b.createUserCallbackData("dice_50", userID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💀 生死对决 (100)", b.createUserCallbackData("dice_100", userID)),
			tgbotapi.NewInlineKeyboardButtonData("💀 生死对决 (500)", b.createUserCallbackData("dice_500", userID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔥 自定义赌注", b.createUserCallbackData("custom_amount", userID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💎 财富查询", b.createUserCallbackData("balance", userID)),
			tgbotapi.NewInlineKeyboardButtonData("⚔️ 寻找猎物", "games"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❓ 战斗指南", b.createUserCallbackData("help", userID)),
		),
	)

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ReplyMarkup = keyboard
	b.api.Send(msg)
}

func (b *Bot) handleHelp(message *tgbotapi.Message) {
	text := `📚 游戏攻略与规则

🎯 游戏玩法：
/dice <金额> - 发起激动人心的对决
  示例：/dice 100

/join <游戏ID> - 勇敢接受挑战
  示例：/join GAME123

/balance - 查看当前余额

/games - 查看群组中等待的游戏

/checkperm - 检查机器人权限状态

/network - 查看网络优化状态

🎯 游戏流程：
1. 玩家A使用 /dice 100 发起游戏
2. 玩家B使用 /join GAME123 加入游戏
3. 双方自动投掷骰子
4. 点数大的获胜，平台收取10%手续费

💡 提示：
• 最小下注金额：1
• 最大下注金额：10000
• 平台服务费：10%
• 骰子结果完全随机且可验证

⚠️ 重要：机器人需要管理员权限才能在游戏中清理干扰消息`

	// 创建帮助菜单内联键盘
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⚡ 血战到底", "show_amount_options"),
			tgbotapi.NewInlineKeyboardButtonData("💎 财富查询", "balance"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⚔️ 寻找猎物", "games"),
			tgbotapi.NewInlineKeyboardButtonData("🏰 回到大厅", "start"),
		),
	)

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ReplyMarkup = keyboard
	b.api.Send(msg)
}

func (b *Bot) handleBalance(message *tgbotapi.Message) {
	user, err := b.db.GetUser(message.From.ID)
	if err != nil {
		b.sendMessage(message.Chat.ID, "❌ 查询余额失败")
		return
	}

	text := fmt.Sprintf("💰 %s 的余额：%d",
		b.getUserDisplayName(message.From), user.Balance)

	// 创建余额操作内联键盘
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💀 生死对决 (10)", "dice_10"),
			tgbotapi.NewInlineKeyboardButtonData("💀 生死对决 (50)", "dice_50"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💀 生死对决 (100)", "dice_100"),
			tgbotapi.NewInlineKeyboardButtonData("💀 生死对决 (500)", "dice_500"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💀 生死对决 (1000)", "dice_1000"),
			tgbotapi.NewInlineKeyboardButtonData("🔥 自定义赌注", "custom_amount"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⚔️ 寻找猎物", "games"),
			tgbotapi.NewInlineKeyboardButtonData("🏰 回到大厅", "start"),
		),
	)

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ReplyMarkup = keyboard
	b.api.Send(msg)
}

func (b *Bot) handleDice(message *tgbotapi.Message, args string) {
	if args == "" {
		b.sendMessage(message.Chat.ID, "❌ 请指定下注金额，例：/dice 100")
		return
	}

	amount, err := strconv.ParseInt(args, 10, 64)
	if err != nil || amount <= 0 {
		b.sendMessage(message.Chat.ID, "❌ 请输入有效的金额")
		return
	}

	// 检查金额限制
	if amount < 1 {
		b.sendMessage(message.Chat.ID, "❌ 最小下注金额为 1")
		return
	}
	if amount > 10000 {
		b.sendMessage(message.Chat.ID, "❌ 最大下注金额为 10000")
		return
	}

	// 检查用户余额
	user, err := b.db.GetUser(message.From.ID)
	if err != nil {
		b.sendMessage(message.Chat.ID, "❌ 查询用户信息失败")
		return
	}

	if user.Balance < amount {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("❌ 余额不足！当前余额：%d，需要：%d", user.Balance, amount))
		return
	}

	// 检查群组是否有正在进行的游戏
	chatID := message.Chat.ID
	if _, hasActiveGame := b.activeGames.Load(chatID); hasActiveGame {
		// 将游戏请求加入队列
		b.addToGameQueue(chatID, GameRequest{
			UserID:   message.From.ID,
			Username: b.getUserDisplayName(message.From),
			Amount:   float64(amount),
			ChatID:   chatID,
			Time:     time.Now(),
		})

		// 获取队列长度
		queueLength := b.getQueueLength(chatID)

		b.sendMessage(chatID, fmt.Sprintf("⏳ 当前群组有游戏正在进行中，您的游戏请求已加入队列\n📍 队列位置：第 %d 位\n💰 下注金额：%d\n⏰ 请耐心等待...", queueLength, amount))
		return
	}

	// 创建游戏
	gameID, err := b.gameManager.CreateGame(message.From.ID, message.Chat.ID, amount)
	if err != nil {
		b.sendMessage(message.Chat.ID, "❌ 创建游戏失败")
		return
	}

	// 标记群组有活跃游戏
	b.activeGames.Store(chatID, gameID)

	text := fmt.Sprintf(`🎲 新游戏创建成功！

🆔 游戏ID：%s
👤 发起者：%s
💰 下注金额：%d
🎯 等待对手加入...

其他玩家可点击下方按钮加入游戏`,
		gameID,
		b.getUserDisplayName(message.From),
		amount)

	// 创建加入游戏的内联键盘
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⚡ 立即应战", "join_"+gameID),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⚔️ 寻找猎物", "games"),
			tgbotapi.NewInlineKeyboardButtonData("🏰 回到大厅", "start"),
		),
	)

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ReplyMarkup = keyboard
	b.api.Send(msg)
}

func (b *Bot) handleJoin(message *tgbotapi.Message, args string) {
	if args == "" {
		b.sendMessage(message.Chat.ID, "❌ 请指定游戏ID，例：/join GAME123")
		return
	}

	gameID := strings.TrimSpace(args)

	// 验证gameID格式
	if len(gameID) < 3 {
		b.sendMessage(message.Chat.ID, "❌ 无效的游戏ID格式")
		return
	}

	result, err := b.gameManager.JoinGame(gameID, message.From.ID)
	if err != nil {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("❌ 加入游戏失败：%s", err.Error()))
		return
	}

	// 发送游戏结果
	b.sendGameResult(result, message.Chat.ID)
}

func (b *Bot) handleGames(message *tgbotapi.Message) {
	games, err := b.db.GetWaitingGames(message.Chat.ID)
	if err != nil {
		b.sendMessage(message.Chat.ID, "❌ 查询游戏列表失败")
		return
	}

	if len(games) == 0 {
		b.sendMessage(message.Chat.ID, "📋 当前没有等待中的游戏")
		return
	}

	text := "📋 等待中的游戏：\n\n"
	for _, game := range games {
		player1, _ := b.db.GetUser(game.Player1ID)
		player1Name := "未知用户"
		if player1 != nil {
			player1Name = b.getUserDisplayNameFromUser(player1)
		}

		text += fmt.Sprintf("🎮 %s\n👤 %s\n💰 %d\n\n",
			game.ID, player1Name, game.BetAmount)
	}

	b.sendMessage(message.Chat.ID, text)
}

func (b *Bot) handleCallbackQuery(query *tgbotapi.CallbackQuery) {
	// 获取用户级别的互斥锁
	userID := query.From.ID
	mutex, _ := b.userMutex.LoadOrStore(userID, &sync.Mutex{})
	userMutex := mutex.(*sync.Mutex)

	userMutex.Lock()
	defer userMutex.Unlock()

	// 确保用户存在
	if err := b.ensureUserExists(query.From); err != nil {
		log.Printf("确保用户存在失败: %v", err)
		return
	}

	// 解析回调数据，检查是否需要验证用户身份
	data := query.Data
	needsValidation := b.needsUserValidation(data)

	if needsValidation && !b.validateUserCallback(data, userID) {
		// 对于需要验证但验证失败的操作，显示权限不足提示
		callback := tgbotapi.NewCallbackWithAlert(query.ID, "⚠️ 这不是您的菜单，请使用 /start 创建自己的菜单")
		b.api.Request(callback)
		return
	}

	// 检查群组是否有活跃游戏，如果有则阻止按钮操作
	if query.Message != nil && query.Message.Chat != nil {
		if _, hasActiveGame := b.activeGames.Load(query.Message.Chat.ID); hasActiveGame {
			// 发送游戏中指令无效的通知（只对点击者可见）
			callback := tgbotapi.NewCallbackWithAlert(query.ID, "游戏中指令无效")
			b.api.Request(callback)
			return
		}
	}

	// 解析回调数据
	// data := query.Data (已在上面定义)

	switch {
	case strings.HasPrefix(data, "join_"):
		gameID := strings.TrimPrefix(data, "join_")
		b.handleJoinGame(query, gameID)
	case strings.HasPrefix(data, "dice_"):
		amountStr := strings.TrimPrefix(data, "dice_")
		// 移除用户ID后缀（如果存在）
		if strings.Contains(amountStr, "_") {
			parts := strings.Split(amountStr, "_")
			if len(parts) >= 2 {
				amountStr = parts[0]
			}
		}
		b.handleDiceCallback(query, amountStr)
	case strings.HasPrefix(data, "custom_amount_"):
		b.handleCustomAmountCallback(query)
	case strings.HasPrefix(data, "show_amount_options_"):
		b.handleShowAmountOptionsCallback(query)
	case strings.HasPrefix(data, "balance_"):
		b.handleBalanceCallback(query)
	case strings.HasPrefix(data, "games_"):
		b.handleGamesCallback(query)
	case strings.HasPrefix(data, "help_"):
		b.handleHelpCallback(query)
	case strings.HasPrefix(data, "start_"):
		b.handleStartCallback(query)
	case data == "quick_game" || data == "custom_amount" || data == "show_amount_options" ||
		data == "balance" || data == "games" || data == "help" || data == "start":
		// 处理不带用户ID的通用按钮
		switch data {
		case "quick_game":
			b.handleQuickGameCallback(query)
		case "custom_amount":
			b.handleCustomAmountCallback(query)
		case "show_amount_options":
			b.handleShowAmountOptionsCallback(query)
		case "balance":
			b.handleBalanceCallback(query)
		case "games":
			b.handleGamesCallback(query)
		case "help":
			b.handleHelpCallback(query)
		case "start":
			b.handleStartCallback(query)
		}
	default:
		// 处理未知的回调数据
		log.Printf("未知的回调数据: %s", data)
		msg := tgbotapi.NewMessage(query.Message.Chat.ID, "❌ 操作无效，请重新选择")
		b.api.Send(msg)
	}

	// 回应回调查询
	callback := tgbotapi.NewCallback(query.ID, "")
	b.api.Request(callback)
}

func (b *Bot) handleDiceCallback(query *tgbotapi.CallbackQuery, amountStr string) {
	userID := query.From.ID

	// 检查用户是否已经在游戏中
	if _, inGame := b.gameStates.Load(userID); inGame {
		msg := tgbotapi.NewMessage(query.Message.Chat.ID, "⚠️ 您正在进行游戏中，请等待当前游戏结束")
		b.api.Send(msg)
		return
	}

	amount, err := strconv.Atoi(amountStr)
	if err != nil {
		msg := tgbotapi.NewMessage(query.Message.Chat.ID, "❌ 无效的金额")
		b.api.Send(msg)
		return
	}

	// 验证金额范围
	if amount < 1 || amount > 10000 {
		msg := tgbotapi.NewMessage(query.Message.Chat.ID, "❌ 金额必须在1-10000之间")
		b.api.Send(msg)
		return
	}

	// 检查用户余额
	user, err := b.db.GetUser(query.From.ID)
	if err != nil {
		msg := tgbotapi.NewMessage(query.Message.Chat.ID, "❌ 查询用户信息失败")
		b.api.Send(msg)
		return
	}

	if user.Balance < int64(amount) {
		msg := tgbotapi.NewMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 余额不足！当前余额：%d，需要：%d", user.Balance, amount))
		b.api.Send(msg)
		return
	}

	// 创建游戏
	gameID, err := b.gameManager.CreateGame(query.From.ID, query.Message.Chat.ID, int64(amount))
	if err != nil {
		msg := tgbotapi.NewMessage(query.Message.Chat.ID, "❌ 创建游戏失败")
		if _, sendErr := b.api.Send(msg); sendErr != nil {
			log.Printf("发送错误消息失败: %v", sendErr)
		}
		return
	}

	text := fmt.Sprintf(`🎲 新游戏创建成功！

🆔 游戏ID：%s
👤 发起者：%s
💰 下注金额：%d
🎯 等待对手加入...

其他玩家可点击下方按钮加入游戏`,
		gameID,
		b.getUserDisplayName(query.From),
		amount)

	// 创建加入游戏的内联键盘
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⚡ 立即应战", "join_"+gameID),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⚔️ 寻找猎物", "games"),
			tgbotapi.NewInlineKeyboardButtonData("🏰 回到大厅", "start"),
		),
	)

	msg := tgbotapi.NewMessage(query.Message.Chat.ID, text)
	msg.ReplyMarkup = keyboard
	b.api.Send(msg)
}

func (b *Bot) handleJoinGame(query *tgbotapi.CallbackQuery, gameID string) {
	userID := query.From.ID

	// 验证gameID格式
	if gameID == "" {
		msg := tgbotapi.NewMessage(query.Message.Chat.ID, "❌ 无效的游戏ID")
		if _, err := b.api.Send(msg); err != nil {
			log.Printf("发送错误消息失败: %v", err)
		}
		return
	}

	// 使用原子操作检查并设置用户游戏状态，避免竞态条件
	if _, loaded := b.gameStates.LoadOrStore(userID, true); loaded {
		msg := tgbotapi.NewMessage(query.Message.Chat.ID, "⚠️ 您正在进行游戏中，请等待当前游戏结束")
		b.api.Send(msg)
		return
	}
	defer b.gameStates.Delete(userID) // 游戏结束后清除状态

	result, err := b.gameManager.JoinGame(gameID, query.From.ID)
	if err != nil {
		msg := tgbotapi.NewMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 接受挑战失败：%s", err.Error()))
		b.api.Send(msg)
		return
	}

	// 发送游戏开始消息
	b.sendGameResult(result, query.Message.Chat.ID)
}

func (b *Bot) sendGameResult(result *game.GameResult, chatID int64) {
	// 设置群组为活跃游戏状态
	b.activeGames.Store(chatID, result.GameID)

	// 使用defer确保游戏状态总是被清理
	defer func() {
		b.activeGames.Delete(chatID)
		go b.processNextGameInQueue(chatID)
	}()

	// 检查机器人权限，如果没有删除消息权限则发出警告
	if err := b.checkBotPermissions(chatID); err != nil {
		log.Printf("机器人权限检查失败 (ChatID: %d): %v", chatID, err)
		// 发送权限警告但不阻止游戏进行
		b.sendMessage(chatID, "⚠️ 提醒：机器人需要管理员权限和删除消息权限才能在游戏中保持环境整洁。请将机器人设为管理员。")
	}

	player1Name := "玩家1"
	player2Name := "玩家2"

	if result.Player1 != nil {
		player1Name = b.getUserDisplayNameFromUser(result.Player1)
	}
	if result.Player2 != nil {
		player2Name = b.getUserDisplayNameFromUser(result.Player2)
	}

	// 先发送游戏开始消息
	startMsg := fmt.Sprintf(`⚡ 终极对决！生死时刻！⚡

🆔 战场编号：%s
💀 %s VS %s 💀
💎 生死赌注：%d 金币

🎲 命运之骰即将决定一切！
⚠️ 心脏病患者请勿观看！`,
		result.GameID,
		player1Name,
		player2Name,
		result.BetAmount)

	b.sendMessage(chatID, startMsg)

	// 发送可验证的骰子动画 - 玩家轮流投掷骰子
	b.sendMessage(chatID, fmt.Sprintf("💥 %s 握紧命运之骰！第一击！", player1Name))
	time.Sleep(1 * time.Second) // API速率限制保护

	// 玩家1投掷第一个骰子 - 带重试机制
	p1dice1Msg, err := b.sendDiceWithRetry(chatID, 3)
	if err != nil {
		log.Printf("玩家1第一个骰子投掷失败: %v", err)
		b.handleGameError(chatID, result.GameID, "骰子投掷失败", err)
		return
	}
	time.Sleep(3 * time.Second) // 增加间隔避免API限制

	// 玩家2投掷第一个骰子
	b.sendMessage(chatID, fmt.Sprintf("🔥 %s 反击！命运的较量！", player2Name))
	time.Sleep(1 * time.Second)
	p2dice1Msg, err := b.sendDiceWithRetry(chatID, 3)
	if err != nil {
		log.Printf("玩家2第一个骰子投掷失败: %v", err)
		b.handleGameError(chatID, result.GameID, "骰子投掷失败", err)
		return
	}
	time.Sleep(3 * time.Second)

	// 玩家1投掷第二个骰子
	b.sendMessage(chatID, fmt.Sprintf("⚡ %s 第二击！势不可挡！", player1Name))
	time.Sleep(1 * time.Second)
	p1dice2Msg, err := b.sendDiceWithRetry(chatID, 3)
	if err != nil {
		log.Printf("玩家1第二个骰子投掷失败: %v", err)
		b.handleGameError(chatID, result.GameID, "骰子投掷失败", err)
		return
	}
	time.Sleep(3 * time.Second)

	// 玩家2投掷第二个骰子
	b.sendMessage(chatID, fmt.Sprintf("💀 %s 绝地反击！生死一线！", player2Name))
	time.Sleep(1 * time.Second)
	p2dice2Msg, err := b.sendDiceWithRetry(chatID, 3)
	if err != nil {
		log.Printf("玩家2第二个骰子投掷失败: %v", err)
		b.handleGameError(chatID, result.GameID, "骰子投掷失败", err)
		return
	}
	time.Sleep(3 * time.Second)

	// 玩家1投掷第三个骰子
	b.sendMessage(chatID, fmt.Sprintf("🚀 %s 最后一击！决定命运！", player1Name))
	time.Sleep(1 * time.Second)
	p1dice3Msg, err := b.sendDiceWithRetry(chatID, 3)
	if err != nil {
		log.Printf("玩家1第三个骰子投掷失败: %v", err)
		b.handleGameError(chatID, result.GameID, "骰子投掷失败", err)
		return
	}
	time.Sleep(3 * time.Second)

	// 玩家2投掷第三个骰子
	b.sendMessage(chatID, fmt.Sprintf("💎 %s 终极一投！胜负已定！", player2Name))
	time.Sleep(1 * time.Second)
	p2dice3Msg, err := b.sendDiceWithRetry(chatID, 3)
	if err != nil {
		log.Printf("玩家2第三个骰子投掷失败: %v", err)
		b.handleGameError(chatID, result.GameID, "骰子投掷失败", err)
		return
	}

	// 等待动画完成后使用实际骰子结果完成游戏
	time.Sleep(4 * time.Second)

	// 使用TG骰子的实际结果完成游戏
	finalResult, err := b.gameManager.PlayGameWithDiceResults(result.GameID,
		p1dice1Msg.Dice.Value, p1dice2Msg.Dice.Value, p1dice3Msg.Dice.Value,
		p2dice1Msg.Dice.Value, p2dice2Msg.Dice.Value, p2dice3Msg.Dice.Value)
	if err != nil {
		b.handleGameError(chatID, result.GameID, "游戏结算失败", err)
		return
	}

	winnerText := "💥 史诗级平局！天神也震惊！💥"
	if finalResult.Winner != nil {
		winnerText = fmt.Sprintf("👑 %s 称霸全场！传奇诞生！👑", b.getUserDisplayNameFromUser(finalResult.Winner))
	}

	resultText := fmt.Sprintf(`🎆 终极对决结果震撼揭晓！🎆

🆔 战场编号：%s
⚔️ %s：🎲 %d + %d + %d = %d 点
⚔️ %s：🎲 %d + %d + %d = %d 点

%s
💰 战利品：%d 金币

🔥 这就是命运的力量！下一个传奇就是你！`,
		finalResult.GameID,
		player1Name, finalResult.Player1Dice1, finalResult.Player1Dice2, finalResult.Player1Dice3, finalResult.Player1Total,
		player2Name, finalResult.Player2Dice1, finalResult.Player2Dice2, finalResult.Player2Dice3, finalResult.Player2Total,
		winnerText,
		finalResult.WinAmount)

	// 创建游戏结束后的操作按钮
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🚀 血战到底！", "show_amount_options"),
			tgbotapi.NewInlineKeyboardButtonData("💎 财富查询", "balance"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⚔️ 寻找猎物", "games"),
			tgbotapi.NewInlineKeyboardButtonData("🏰 回到大厅", "start"),
		),
	)

	msg := tgbotapi.NewMessage(chatID, resultText)
	msg.ReplyMarkup = keyboard
	b.api.Send(msg)

	// 游戏成功完成，defer中的清理会自动执行
}

// checkBotPermissions 检查机器人在群组中的权限
func (b *Bot) checkBotPermissions(chatID int64) error {
	// 获取机器人信息
	me, err := b.api.GetMe()
	if err != nil {
		return fmt.Errorf("获取机器人信息失败: %v", err)
	}

	// 获取机器人在群组中的成员信息
	chatMemberConfig := tgbotapi.GetChatMemberConfig{
		ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
			ChatID: chatID,
			UserID: me.ID,
		},
	}

	chatMember, err := b.api.GetChatMember(chatMemberConfig)
	if err != nil {
		return fmt.Errorf("获取机器人权限信息失败: %v", err)
	}

	// 检查是否为管理员
	if chatMember.Status != "administrator" && chatMember.Status != "creator" {
		return fmt.Errorf("机器人不是管理员")
	}

	// 检查删除消息权限
	if !chatMember.CanDeleteMessages {
		return fmt.Errorf("机器人没有删除消息权限")
	}

	return nil
}

func (b *Bot) ensureUserExists(from *tgbotapi.User) error {
	user, err := b.db.GetUser(from.ID)
	if err != nil {
		return err
	}

	if user == nil {
		// 创建新用户
		newUser := &models.User{
			ID:        from.ID,
			Username:  from.UserName,
			FirstName: from.FirstName,
			LastName:  from.LastName,
			Balance:   1000, // 新用户赠送1000初始余额
		}
		return b.db.CreateUser(newUser)
	}

	return nil
}

func (b *Bot) getUserDisplayName(user *tgbotapi.User) string {
	// 优先显示用户的真实姓名（昵称）
	name := user.FirstName
	if user.LastName != "" {
		name += " " + user.LastName
	}
	if name != "" {
		return name
	}
	// 如果没有昵称，才使用用户名（不带@符号）
	if user.UserName != "" {
		return user.UserName
	}
	// 最后的备选方案
	return fmt.Sprintf("用户%d", user.ID)
}

func (b *Bot) getUserDisplayNameFromUser(user *models.User) string {
	// 优先显示用户的真实姓名（昵称）
	name := user.FirstName
	if user.LastName != "" {
		name += " " + user.LastName
	}
	if name != "" {
		return name
	}
	// 如果没有昵称，才使用用户名（不带@符号）
	if user.Username != "" {
		return user.Username
	}
	// 最后的备选方案
	return fmt.Sprintf("用户%d", user.ID)
}

// 回调处理函数
func (b *Bot) handleBalanceCallback(query *tgbotapi.CallbackQuery) {
	user, err := b.db.GetUser(query.From.ID)
	if err != nil {
		msg := tgbotapi.NewMessage(query.Message.Chat.ID, "❌ 查询余额失败")
		b.api.Send(msg)
		return
	}

	userDisplay := b.getUsernameDisplay(query.From)
	userID := query.From.ID
	text := fmt.Sprintf("💰 %s 的余额：%d", userDisplay, user.Balance)

	// 创建余额操作内联键盘
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💀 生死对决 (10)", b.createUserCallbackData("dice_10", userID)),
			tgbotapi.NewInlineKeyboardButtonData("💀 生死对决 (50)", b.createUserCallbackData("dice_50", userID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💀 生死对决 (100)", b.createUserCallbackData("dice_100", userID)),
			tgbotapi.NewInlineKeyboardButtonData("💀 生死对决 (500)", b.createUserCallbackData("dice_500", userID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💀 生死对决 (1000)", b.createUserCallbackData("dice_1000", userID)),
			tgbotapi.NewInlineKeyboardButtonData("🔥 自定义赌注", b.createUserCallbackData("custom_amount", userID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⚔️ 寻找猎物", "games"),
			tgbotapi.NewInlineKeyboardButtonData("🏰 回到大厅", b.createUserCallbackData("start", userID)),
		),
	)

	msg := tgbotapi.NewMessage(query.Message.Chat.ID, text)
	msg.ReplyMarkup = keyboard
	b.api.Send(msg)
}

func (b *Bot) handleGamesCallback(query *tgbotapi.CallbackQuery) {
	games, err := b.db.GetWaitingGames(query.Message.Chat.ID)
	if err != nil {
		msg := tgbotapi.NewMessage(query.Message.Chat.ID, "❌ 查询游戏列表失败")
		b.api.Send(msg)
		return
	}

	if len(games) == 0 {
		userDisplay := b.getUsernameDisplay(query.From)
		userID := query.From.ID
		text := fmt.Sprintf("📋 当前没有等待中的游戏\n\n👤 当前玩家：%s\n\n点击下方按钮发起新游戏！", userDisplay)

		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("💀 生死对决 (10)", b.createUserCallbackData("dice_10", userID)),
				tgbotapi.NewInlineKeyboardButtonData("💀 生死对决 (50)", b.createUserCallbackData("dice_50", userID)),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("💀 生死对决 (100)", b.createUserCallbackData("dice_100", userID)),
				tgbotapi.NewInlineKeyboardButtonData("💀 生死对决 (500)", b.createUserCallbackData("dice_500", userID)),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("💀 生死对决 (1000)", b.createUserCallbackData("dice_1000", userID)),
				tgbotapi.NewInlineKeyboardButtonData("🔥 自定义赌注", b.createUserCallbackData("custom_amount", userID)),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🏰 回到大厅", b.createUserCallbackData("start", userID)),
			),
		)

		msg := tgbotapi.NewMessage(query.Message.Chat.ID, text)
		msg.ReplyMarkup = keyboard
		b.api.Send(msg)
		return
	}

	userDisplay := b.getUsernameDisplay(query.From)
	userID := query.From.ID
	text := fmt.Sprintf("📋 等待中的游戏：\n\n👤 当前玩家：%s\n\n", userDisplay)
	var buttons [][]tgbotapi.InlineKeyboardButton

	for _, game := range games {
		player1, _ := b.db.GetUser(game.Player1ID)
		player1Name := "未知用户"
		if player1 != nil {
			player1Name = b.getUserDisplayNameFromUser(player1)
		}

		text += fmt.Sprintf("� %s\n👤 %s\n💰 %d\n\n",
			game.ID, player1Name, game.BetAmount)

		// 为每个游戏添加加入按钮
		buttons = append(buttons, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("⚡ 立即应战 %s (💰%d)", game.ID, game.BetAmount), "join_"+game.ID),
		))
	}

	// 添加底部操作按钮
	buttons = append(buttons, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔄 刷新战场", "games"),
		tgbotapi.NewInlineKeyboardButtonData("🏰 回到大厅", b.createUserCallbackData("start", userID)),
	))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(buttons...)
	msg := tgbotapi.NewMessage(query.Message.Chat.ID, text)
	msg.ReplyMarkup = keyboard
	b.api.Send(msg)
}

func (b *Bot) handleHelpCallback(query *tgbotapi.CallbackQuery) {
	userID := query.From.ID
	text := `🎲 骰子机器人帮助

📋 命令说明：
/dice <金额> - 发起新的骰子游戏
  示例：/dice 100

/join <游戏ID> - 加入等待中的游戏
  示例：/join GAME123

/balance - 查看当前余额

/games - 查看群组中等待的游戏

🎯 游戏流程：
1. 玩家A使用 /dice 100 发起游戏
2. 玩家B使用 /join GAME123 加入游戏
3. 双方自动投掷骰子
4. 点数大的获胜，平台收取10%手续费

💡 提示：
• 最小下注金额：1
• 最大下注金额：10000
• 平台服务费：10%
• 骰子结果完全随机且可验证`

	// 创建帮助菜单内联键盘
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⚡ 闪电对决", b.createUserCallbackData("quick_game", userID)),
			tgbotapi.NewInlineKeyboardButtonData("💎 财富查询", b.createUserCallbackData("balance", userID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⚔️ 寻找猎物", "games"),
			tgbotapi.NewInlineKeyboardButtonData("🏰 回到大厅", b.createUserCallbackData("start", userID)),
		),
	)

	msg := tgbotapi.NewMessage(query.Message.Chat.ID, text)
	msg.ReplyMarkup = keyboard
	b.api.Send(msg)
}

func (b *Bot) handleStartCallback(query *tgbotapi.CallbackQuery) {
	text := `✨ 欢迎来到极致骰子世界！✨

🎯 游戏特色：
• 🔥 刺激1v1骰子对决，谁与争锋
• 💎 公平透明，Telegram官方骰子保证
• ⚡ 即时结算，秒速到账
• 🎊 简单易懂，一键开启财富之门

🚀 立即体验：
选择下方金额，开启你的幸运之旅！

💡 温馨提示：点击"✨ 自定义金额"可设置任意金额！

🎁 新手福利：注册即送1000金币！`

	// 创建内联键盘
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💀 生死对决 (10)", "dice_10"),
			tgbotapi.NewInlineKeyboardButtonData("💀 生死对决 (50)", "dice_50"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💀 生死对决 (100)", "dice_100"),
			tgbotapi.NewInlineKeyboardButtonData("💀 生死对决 (500)", "dice_500"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔥 自定义赌注", "custom_amount"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💎 财富查询", "balance"),
			tgbotapi.NewInlineKeyboardButtonData("⚔️ 寻找猎物", "games"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❓ 战斗指南", "help"),
		),
	)

	msg := tgbotapi.NewMessage(query.Message.Chat.ID, text)
	msg.ReplyMarkup = keyboard
	b.api.Send(msg)
}

// handleShowAmountOptionsCallback 显示金额选择选项
func (b *Bot) handleShowAmountOptionsCallback(query *tgbotapi.CallbackQuery) {
	text := `💎 选择对决金额

请选择您想要的对决金额：`

	// 创建金额选择键盘
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💀 10金币", "dice_10"),
			tgbotapi.NewInlineKeyboardButtonData("💀 50金币", "dice_50"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💀 100金币", "dice_100"),
			tgbotapi.NewInlineKeyboardButtonData("💀 500金币", "dice_500"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔥 自定义赌注", "custom_amount"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🏰 回到大厅", "start"),
		),
	)

	msg := tgbotapi.NewMessage(query.Message.Chat.ID, text)
	msg.ReplyMarkup = keyboard
	b.api.Send(msg)
}

// handleCustomAmountCallback 处理自定义金额回调
func (b *Bot) handleCustomAmountCallback(query *tgbotapi.CallbackQuery) {
	text := `💎 自定义对决金额

请输入您想要的对决金额：

� 温馨提示：
• 最小金额：1 金币
• 最大金额：10,000 金币
• 请直接输入数字，例如：888

� 输入格式：/dice <金额>
例如：/dice 888`

	msg := tgbotapi.NewMessage(query.Message.Chat.ID, text)

	// 创建返回按钮
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🏰 回到大厅", "start"),
		),
	)
	msg.ReplyMarkup = keyboard

	b.api.Send(msg)
}

// handleQuickGameCallback 处理快速游戏回调，显示金额确认界面
func (b *Bot) handleQuickGameCallback(query *tgbotapi.CallbackQuery) {
	userDisplay := b.getUsernameDisplay(query.From)
	text := fmt.Sprintf(`💎 快速对决确认

👤 当前玩家：%s

请选择下注金额：`, userDisplay)

	userID := query.From.ID
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💀 100金币", b.createUserCallbackData("dice_100", userID)),
			tgbotapi.NewInlineKeyboardButtonData("💀 500金币", b.createUserCallbackData("dice_500", userID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💀 1000金币", b.createUserCallbackData("dice_1000", userID)),
			tgbotapi.NewInlineKeyboardButtonData("🔥 自定义赌注", b.createUserCallbackData("custom_amount", userID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔙 返回", b.createUserCallbackData("help", userID)),
			tgbotapi.NewInlineKeyboardButtonData("🏰 主菜单", b.createUserCallbackData("start", userID)),
		),
	)

	msg := tgbotapi.NewMessage(query.Message.Chat.ID, text)
	msg.ReplyMarkup = keyboard
	b.api.Send(msg)
}

// sendMessage 优化的消息发送方法，使用对象池
func (b *Bot) sendMessage(chatID int64, text string) {
	msg := b.objectPool.GetMessage()
	defer b.objectPool.PutMessage(msg)

	msg.ChatID = chatID
	msg.Text = text
	msg.ReplyMarkup = nil

	if _, err := b.api.Send(*msg); err != nil {
		log.Printf("发送消息失败 (ChatID: %d): %v", chatID, err)
	}
}

// sendMessageWithKeyboard 优化的带键盘消息发送方法
func (b *Bot) sendMessageWithKeyboard(chatID int64, text string, keyboard tgbotapi.InlineKeyboardMarkup) {
	msg := b.objectPool.GetMessage()
	defer b.objectPool.PutMessage(msg)

	msg.ChatID = chatID
	msg.Text = text
	msg.ReplyMarkup = keyboard

	if _, err := b.api.Send(*msg); err != nil {
		log.Printf("发送带键盘消息失败 (ChatID: %d): %v", chatID, err)
	}
}

// buildGameListText 优化的游戏列表文本构建
func (b *Bot) buildGameListText(games []*models.Game) string {
	if len(games) == 0 {
		return "🔥 战场空无一人！🔥\n\n成为第一个发起挑战的勇士吧！"
	}

	sb := b.objectPool.GetStringBuilder()
	defer b.objectPool.PutStringBuilder(sb)

	sb.WriteString("🔥 激战正在等待你！🔥\n\n")

	for _, game := range games {
		player1, _ := b.db.GetUser(game.Player1ID)
		player1Name := "神秘挑战者"
		if player1 != nil {
			player1Name = b.getUserDisplayNameFromUser(player1)
		}

		sb.WriteString(fmt.Sprintf("🆔 %s | 👤 %s | 💰 %d金币\n", game.ID, player1Name, game.BetAmount))
	}

	return sb.String()
}

// sendDiceWithRetry 带重试机制的骰子投掷函数
func (b *Bot) sendDiceWithRetry(chatID int64, maxRetries int) (*tgbotapi.Message, error) {
	for i := 0; i < maxRetries; i++ {
		dice := tgbotapi.NewDice(chatID)
		msg, err := b.api.Send(dice)

		if err == nil && msg.Dice != nil {
			return &msg, nil
		}

		// 记录错误并等待重试
		log.Printf("骰子投掷失败 (尝试 %d/%d): %v", i+1, maxRetries, err)

		if i < maxRetries-1 {
			// 检查是否是速率限制错误，如果是则使用更长的等待时间
			waitTime := time.Duration(1<<uint(i)) * time.Second
			if err != nil && strings.Contains(err.Error(), "Too Many Requests") {
				// 对于速率限制错误，使用更长的等待时间
				waitTime = time.Duration(20+i*10) * time.Second
			}
			log.Printf("等待 %v 后重试...", waitTime)
			time.Sleep(waitTime)
		}
	}

	return nil, fmt.Errorf("骰子投掷失败，已重试 %d 次", maxRetries)
}

// addToGameQueue 将游戏请求添加到队列
func (b *Bot) addToGameQueue(chatID int64, request GameRequest) {
	// 获取或创建队列互斥锁
	mutexInterface, _ := b.queueMutex.LoadOrStore(chatID, &sync.Mutex{})
	mutex := mutexInterface.(*sync.Mutex)

	mutex.Lock()
	defer mutex.Unlock()

	// 获取当前队列
	queueInterface, _ := b.gameQueue.LoadOrStore(chatID, []GameRequest{})
	queue := queueInterface.([]GameRequest)

	// 添加新请求到队列
	queue = append(queue, request)
	b.gameQueue.Store(chatID, queue)
}

// getQueueLength 获取队列长度
func (b *Bot) getQueueLength(chatID int64) int {
	queueInterface, exists := b.gameQueue.Load(chatID)
	if !exists {
		return 0
	}
	queue := queueInterface.([]GameRequest)
	return len(queue)
}

// processNextGameInQueue 处理队列中的下一个游戏
func (b *Bot) processNextGameInQueue(chatID int64) {
	// 获取队列互斥锁
	mutexInterface, exists := b.queueMutex.Load(chatID)
	if !exists {
		return
	}
	mutex := mutexInterface.(*sync.Mutex)

	mutex.Lock()
	defer mutex.Unlock()

	// 获取队列
	queueInterface, exists := b.gameQueue.Load(chatID)
	if !exists {
		return
	}
	queue := queueInterface.([]GameRequest)

	if len(queue) == 0 {
		return
	}

	// 取出第一个请求
	nextRequest := queue[0]
	queue = queue[1:]
	b.gameQueue.Store(chatID, queue)

	// 处理下一个游戏请求
	go b.createGameFromQueue(nextRequest)
}

// createGameFromQueue 从队列创建游戏
func (b *Bot) createGameFromQueue(request GameRequest) {
	// 等待一段时间确保前一个游戏完全结束
	time.Sleep(2 * time.Second)

	// 检查用户余额是否仍然足够
	user, err := b.db.GetUser(request.UserID)
	if err != nil {
		b.sendMessage(request.ChatID, fmt.Sprintf("❌ 队列游戏处理失败：查询用户 %s 信息失败", request.Username))
		b.processNextGameInQueue(request.ChatID)
		return
	}

	if user.Balance < int64(request.Amount) {
		b.sendMessage(request.ChatID, fmt.Sprintf("❌ 队列游戏取消：用户 %s 余额不足（当前：%d，需要：%.0f）", request.Username, user.Balance, request.Amount))
		b.processNextGameInQueue(request.ChatID)
		return
	}

	// 创建游戏
	gameID, err := b.gameManager.CreateGame(request.UserID, request.ChatID, int64(request.Amount))
	if err != nil {
		b.sendMessage(request.ChatID, fmt.Sprintf("❌ 队列游戏创建失败：%s", request.Username))
		b.processNextGameInQueue(request.ChatID)
		return
	}

	// 标记群组有活跃游戏
	b.activeGames.Store(request.ChatID, gameID)

	text := fmt.Sprintf(`🎯 队列游戏开始！

🆔 战场编号：%s
⚔️ 挑战者：%s
💎 赌注：%.0f 金币
🔥 生死一战，谁敢应战？！

⚠️ 警告：只有真正的勇士才敢接受这个挑战！
💀 败者将失去一切，胜者独享荣耀！

👇 其他玩家，你们敢吗？`,
		gameID,
		request.Username,
		request.Amount)

	// 创建加入游戏的内联键盘
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⚡ 立即应战", "join_"+gameID),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⚔️ 寻找猎物", "games"),
			tgbotapi.NewInlineKeyboardButtonData("🏰 回到大厅", "start"),
		),
	)

	msg := tgbotapi.NewMessage(request.ChatID, text)
	msg.ReplyMarkup = keyboard
	b.api.Send(msg)
}

// GetNetworkStatus 获取网络状态信息
func (b *Bot) GetNetworkStatus() string {
	if b.accelerator == nil {
		return "❌ 网络加速器未初始化"
	}

	currentDC := b.accelerator.GetCurrentDatacenter()
	if currentDC == nil {
		return "❌ 无法获取数据中心信息"
	}

	status := fmt.Sprintf("🚀 网络加速器状态:\n")
	status += fmt.Sprintf("📍 当前数据中心: %s (%s)\n", currentDC.Name, currentDC.Location)
	status += fmt.Sprintf("⚡ 延迟: %v\n", currentDC.Latency)
	status += fmt.Sprintf("💾 缓存条目: %d\n", b.accelerator.GetCacheSize())

	stats := b.accelerator.GetNetworkStats()
	if len(stats) > 0 {
		status += "📊 网络统计:\n"
		for endpoint, stat := range stats {
			status += fmt.Sprintf("  %s: 请求%d次, 错误率%.1f%%, 平均延迟%v\n", 
				endpoint, stat.RequestCount, stat.ErrorRate*100, stat.Latency)
		}
	}

	status += "🔧 启用的优化:\n"
	status += "  ✅ HTTP/2 支持\n"
	status += "  ✅ 连接池优化\n"
	status += "  ✅ 智能重试机制\n"
	status += "  ✅ 响应缓存\n"
	status += "  ✅ 网络质量监控\n"
	status += "  ✅ 自动数据中心切换\n"

	return status
}

// handleNetworkStatus 处理网络状态命令
func (b *Bot) handleNetworkStatus(message *tgbotapi.Message) {
	status := b.GetNetworkStatus()
	b.sendMessage(message.Chat.ID, status)
}

// ...
