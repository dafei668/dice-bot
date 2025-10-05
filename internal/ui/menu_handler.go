package ui

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// MenuHandler 菜单处理器
type MenuHandler struct {
	menuSystem     *HybridMenuSystem
	userMenuStates map[int64]MenuType // 用户当前所处的菜单状态
}

// NewMenuHandler 创建菜单处理器
func NewMenuHandler() *MenuHandler {
	return &MenuHandler{
		menuSystem:     NewHybridMenuSystem(),
		userMenuStates: make(map[int64]MenuType),
	}
}

// HandleMessage 处理消息并根据内容选择菜单
func (h *MenuHandler) HandleMessage(msg *tgbotapi.Message, bot *tgbotapi.BotAPI, userID int64, isAdmin bool) (tgbotapi.Chattable, error) {
	// 检查消息文本
	if msg.Text == "" {
		return nil, nil
	}

	// 初始化用户状态
	if _, exists := h.userMenuStates[userID]; !exists {
		h.userMenuStates[userID] = MenuTypeMain
	}

	// 处理菜单导航
	var newMenuType MenuType
	var shouldSendMenu bool

	switch msg.Text {
	case "🎮 游戏中心":
		newMenuType = MenuTypeGameCenter
		shouldSendMenu = true

	case "💰 财务管理":
		newMenuType = MenuTypeFinance
		shouldSendMenu = true

	case "⚡ 更多":
		newMenuType = MenuTypeMore
		shouldSendMenu = true

	case "↩️ 返回主菜单":
		newMenuType = MenuTypeMain
		shouldSendMenu = true

	// 游戏中心子菜单选项
	case "🎲 开始游戏":
		// 处理开始游戏的逻辑
		return h.handleStartGame(userID, bot)

	case "🔍 胜率查询":
		// 处理胜率查询的逻辑
		return h.handleWinRateQuery(userID, bot)

	case "📊 游戏历史":
		newMenuType = MenuTypeHistory
		shouldSendMenu = true

	case "🏆 排行榜":
		newMenuType = MenuTypeRank
		shouldSendMenu = true

	// 财务管理子菜单选项
	case "💎 余额查询":
		// 处理余额查询的逻辑
		return h.handleBalanceQuery(userID, bot)

	case "💳 快速充值":
		newMenuType = MenuTypeRecharge
		shouldSendMenu = true

	case "💸 提现申请":
		newMenuType = MenuTypeWithdraw
		shouldSendMenu = true

	case "📈 收支明细":
		newMenuType = MenuTypeTransaction
		shouldSendMenu = true

	// 更多子菜单选项
	case "📖 新手教程":
		newMenuType = MenuTypeGuide
		shouldSendMenu = true

	case "🎯 游戏规则":
		newMenuType = MenuTypeRules
		shouldSendMenu = true

	case "🛠️ 联系客服":
		newMenuType = MenuTypeSupport
		shouldSendMenu = true

	case "⚙️ 设置":
		newMenuType = MenuTypeSettings
		shouldSendMenu = true

	default:
		// 其他文本消息处理
		return nil, nil
	}

	if shouldSendMenu {
		h.userMenuStates[userID] = newMenuType
		return h.sendMenuForType(userID, newMenuType, isAdmin, msg.Chat.ID)
	}

	return nil, nil
}

// HandleCallbackQuery 处理回调查询
func (h *MenuHandler) HandleCallbackQuery(query *tgbotapi.CallbackQuery, bot *tgbotapi.BotAPI, userID int64, isAdmin bool) (tgbotapi.Chattable, error) {
	// 处理内联键盘按钮点击
	data := query.Data
	chatID := query.Message.Chat.ID

	var response tgbotapi.Chattable

	switch data {
	case "create_game":
		response = tgbotapi.NewMessage(chatID, "🎮 请选择游戏模式和下注金额：")
		// 添加游戏设置的内联键盘...

	case "random_match":
		response = tgbotapi.NewMessage(chatID, "🔄 正在为您寻找对手...")
		// 添加随机匹配的逻辑...

	case "quick_game_10":
		response = tgbotapi.NewMessage(chatID, "🎲 已创建10💎的游戏，等待对手加入...")
		// 处理快速游戏10的逻辑...

	case "quick_game_50":
		response = tgbotapi.NewMessage(chatID, "🎲 已创建50💎的游戏，等待对手加入...")
		// 处理快速游戏50的逻辑...

	case "recharge":
		response = tgbotapi.NewMessage(chatID, "💳 请选择充值金额：")
		// 添加充值选项的内联键盘...

	case "recharge_10", "recharge_50", "recharge_100":
		// 处理不同金额的充值...
		amount := ""
		switch data {
		case "recharge_10":
			amount = "10"
		case "recharge_50":
			amount = "50"
		case "recharge_100":
			amount = "100"
		}
		response = tgbotapi.NewMessage(chatID, "💰 已为您生成"+amount+"💎的充值订单，请按照以下指引完成支付...")

	case "withdraw":
		response = tgbotapi.NewMessage(chatID, "💸 请输入提现金额和您的收款地址：")
		// 后续处理提现逻辑...

	case "balance":
		// 处理余额查询...

	case "game_history":
		// 处理游戏历史查询...

	case "stats":
		// 处理统计数据查询...

	case "my_rank":
		// 处理排名查询...

	default:
		return nil, nil
	}

	// 回应回调查询
	callback := tgbotapi.NewCallback(query.ID, "")
	bot.Request(callback)

	return response, nil
}

// sendMenuForType 发送特定类型的菜单
func (h *MenuHandler) sendMenuForType(userID int64, menuType MenuType, isAdmin bool, chatID int64) (tgbotapi.Chattable, error) {
	var replyKeyboard *tgbotapi.ReplyKeyboardMarkup
	var inlineKeyboard *tgbotapi.InlineKeyboardMarkup

	// 根据菜单类型选择对应的菜单
	switch menuType {
	case MenuTypeMain:
		replyKeyboard, inlineKeyboard = h.menuSystem.CreateMainMenu(userID, isAdmin)
	case MenuTypeGameCenter:
		replyKeyboard, inlineKeyboard = h.menuSystem.CreateGameCenterMenu(userID)
	case MenuTypeFinance:
		replyKeyboard, inlineKeyboard = h.menuSystem.CreateFinanceMenu(userID)
	case MenuTypeMore:
		replyKeyboard, inlineKeyboard = h.menuSystem.CreateMoreMenu(userID)
	default:
		replyKeyboard, inlineKeyboard = h.menuSystem.CreateMainMenu(userID, isAdmin)
	}

	// 准备消息文本
	text := "请选择操作："
	switch menuType {
	case MenuTypeMain:
		text = "🎮 欢迎使用骰子游戏机器人！\n\n请选择以下功能："
	case MenuTypeGameCenter:
		text = "🎲 游戏中心\n\n在这里您可以开始游戏、查看战绩和排行榜："
	case MenuTypeFinance:
		text = "💰 财务管理\n\n在这里您可以查询余额、充值和提现："
	case MenuTypeMore:
		text = "⚡ 更多功能\n\n在这里您可以查看教程、规则和联系客服："
	}

	msg := tgbotapi.NewMessage(chatID, text)

	// 设置回复键盘（底部固定菜单）
	if replyKeyboard != nil {
		msg.ReplyMarkup = replyKeyboard
	}

	// 设置内联键盘（需要单独发送一条消息）
	if inlineKeyboard != nil {
		// 对于内联键盘，我们需要单独发送一条消息
		// 但是在这里我们只返回回复键盘的消息，
		// 实际发送内联键盘的工作交给 Bot 的主函数处理
		// 这里我们通过返回具有回复键盘的消息，确保底部菜单正常显示
	}

	return msg, nil
}

// GetCurrentMenuType 获取用户当前的菜单类型
func (h *MenuHandler) GetCurrentMenuType(userID int64) MenuType {
	if menuType, exists := h.userMenuStates[userID]; exists {
		return menuType
	}
	return MenuTypeMain // 默认返回主菜单类型
}

// SetMenuType 设置用户的菜单类型
func (h *MenuHandler) SetMenuType(userID int64, menuType MenuType) {
	h.userMenuStates[userID] = menuType
}

// 处理各种菜单选项的辅助函数

func (h *MenuHandler) handleStartGame(userID int64, bot *tgbotapi.BotAPI) (tgbotapi.Chattable, error) {
	// 这里实现游戏开始逻辑
	msg := tgbotapi.NewMessage(userID, "🎲 请选择游戏模式和下注金额：")

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🎮 创建游戏", "create_game"),
			tgbotapi.NewInlineKeyboardButtonData("🔄 随机匹配", "random_match"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("10💎", "bet_10"),
			tgbotapi.NewInlineKeyboardButtonData("50💎", "bet_50"),
			tgbotapi.NewInlineKeyboardButtonData("100💎", "bet_100"),
		),
	)

	msg.ReplyMarkup = keyboard
	return msg, nil
}

func (h *MenuHandler) handleWinRateQuery(userID int64, bot *tgbotapi.BotAPI) (tgbotapi.Chattable, error) {
	// 这里实现胜率查询逻辑
	msg := tgbotapi.NewMessage(userID, "🔍 您的游戏胜率统计：\n\n总场次：0\n胜利：0\n失败：0\n胜率：0%\n\n暂无游戏记录，开始游戏吧！")
	return msg, nil
}

func (h *MenuHandler) handleBalanceQuery(userID int64, bot *tgbotapi.BotAPI) (tgbotapi.Chattable, error) {
	// 这里实现余额查询逻辑
	msg := tgbotapi.NewMessage(userID, "💰 您的账户余额：\n\n当前余额：0💎\n\n可通过\"财务管理\"菜单进行充值和提现操作。")

	// 添加快速充值按钮
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💲 充值", "recharge"),
		),
	)

	msg.ReplyMarkup = keyboard
	return msg, nil
}
