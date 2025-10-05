package ui

import (
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// HybridMenuSystem 混合式菜单系统
type HybridMenuSystem struct {
	// 配置选项
	useInlineForActions   bool // 操作按钮使用InlineKeyboard
	useReplyForNavigation bool // 导航使用ReplyKeyboard
	adaptiveMode          bool // 自适应模式
}

// MenuType 菜单类型
type MenuType int

const (
	MenuTypeMain        MenuType = iota
	MenuTypeGameCenter           // 游戏中心
	MenuTypeFinance              // 财务管理
	MenuTypeMore                 // 更多选项
	MenuTypeBalance              // 余额查询
	MenuTypeHistory              // 游戏历史
	MenuTypeRank                 // 排行榜
	MenuTypeRecharge             // 充值
	MenuTypeWithdraw             // 提现
	MenuTypeTransaction          // 交易记录
	MenuTypeGuide                // 新手教程
	MenuTypeRules                // 游戏规则
	MenuTypeSupport              // 联系客服
	MenuTypeSettings             // 设置
	MenuTypeAdmin                // 管理面板
)

// NewHybridMenuSystem 创建新的混合式菜单系统
func NewHybridMenuSystem() *HybridMenuSystem {
	return &HybridMenuSystem{
		useInlineForActions:   true, // 操作按钮使用内联键盘
		useReplyForNavigation: true, // 导航使用回复键盘
		adaptiveMode:          true, // 启用自适应模式
	}
}

// CreateMainMenu 创建主菜单
func (hms *HybridMenuSystem) CreateMainMenu(userID int64, isAdmin bool) (*tgbotapi.ReplyKeyboardMarkup, *tgbotapi.InlineKeyboardMarkup) {
	// 导航键盘 (ReplyKeyboard)
	var replyKeyboard *tgbotapi.ReplyKeyboardMarkup
	if hms.useReplyForNavigation {
		keyboard := tgbotapi.NewReplyKeyboard(
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("🎮 游戏中心"),
				tgbotapi.NewKeyboardButton("💰 财务管理"),
			),
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("⚙️ 设置"),
				tgbotapi.NewKeyboardButton("⚡ 更多"),
			),
		)

		// 管理员额外按钮
		if isAdmin {
			keyboard.Keyboard = append(keyboard.Keyboard,
				tgbotapi.NewKeyboardButtonRow(
					tgbotapi.NewKeyboardButton("👑 管理面板"),
				),
			)
		}

		keyboard.ResizeKeyboard = true
		keyboard.OneTimeKeyboard = false
		replyKeyboard = &keyboard
	}

	// 快速操作键盘 (InlineKeyboard)
	var inlineKeyboard *tgbotapi.InlineKeyboardMarkup
	if hms.useInlineForActions {
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🚀 快速游戏 (10💎)", "quick_game_10"),
				tgbotapi.NewInlineKeyboardButtonData("💎 快速游戏 (50💎)", "quick_game_50"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("💳 快速充值", "recharge"),
				tgbotapi.NewInlineKeyboardButtonData("💸 提现", "withdraw"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("💰 查询余额", "check_balance"),
				tgbotapi.NewInlineKeyboardButtonData("📜 游戏记录", "game_history"),
			),
		)
		inlineKeyboard = &keyboard
	}

	return replyKeyboard, inlineKeyboard
}

// CreateGameCenterMenu 创建游戏中心菜单
func (hms *HybridMenuSystem) CreateGameCenterMenu(userID int64) (*tgbotapi.ReplyKeyboardMarkup, *tgbotapi.InlineKeyboardMarkup) {
	// 导航键盘 (ReplyKeyboard)
	var replyKeyboard *tgbotapi.ReplyKeyboardMarkup
	if hms.useReplyForNavigation {
		keyboard := tgbotapi.NewReplyKeyboard(
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("🎲 开始游戏"),
				tgbotapi.NewKeyboardButton("🔍 胜率查询"),
			),
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("📊 游戏历史"),
				tgbotapi.NewKeyboardButton("🏆 排行榜"),
			),
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("↩️ 返回主菜单"),
			),
		)

		keyboard.ResizeKeyboard = true
		keyboard.OneTimeKeyboard = false
		replyKeyboard = &keyboard
	}

	// 快速操作键盘 (InlineKeyboard)
	var inlineKeyboard *tgbotapi.InlineKeyboardMarkup
	if hms.useInlineForActions {
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🎲 投掷 10💎", "play_10"),
				tgbotapi.NewInlineKeyboardButtonData("🎲 投掷 50💎", "play_50"),
				tgbotapi.NewInlineKeyboardButtonData("🎲 投掷 100💎", "play_100"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("📊 我的统计", "my_stats"),
				tgbotapi.NewInlineKeyboardButtonData("👥 邀请朋友", "invite_friends"),
			),
		)
		inlineKeyboard = &keyboard
	}

	return replyKeyboard, inlineKeyboard
}

// CreateFinanceMenu 创建财务管理菜单
func (hms *HybridMenuSystem) CreateFinanceMenu(userID int64) (*tgbotapi.ReplyKeyboardMarkup, *tgbotapi.InlineKeyboardMarkup) {
	// 导航键盘 (ReplyKeyboard)
	var replyKeyboard *tgbotapi.ReplyKeyboardMarkup
	if hms.useReplyForNavigation {
		keyboard := tgbotapi.NewReplyKeyboard(
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("💵 充值"),
				tgbotapi.NewKeyboardButton("💸 提现"),
			),
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("💹 交易记录"),
				tgbotapi.NewKeyboardButton("👤 账户"),
			),
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("↩️ 返回主菜单"),
			),
		)

		keyboard.ResizeKeyboard = true
		keyboard.OneTimeKeyboard = false
		replyKeyboard = &keyboard
	}

	// 快速操作键盘 (InlineKeyboard)
	var inlineKeyboard *tgbotapi.InlineKeyboardMarkup
	if hms.useInlineForActions {
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("💲 充值 50", "recharge_50"),
				tgbotapi.NewInlineKeyboardButtonData("💲 充值 100", "recharge_100"),
				tgbotapi.NewInlineKeyboardButtonData("💲 充值 500", "recharge_500"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("💸 提现", "withdraw_funds"),
				tgbotapi.NewInlineKeyboardButtonData("💰 查询余额", "check_balance"),
			),
		)
		inlineKeyboard = &keyboard
	}

	return replyKeyboard, inlineKeyboard
}

// CreateMoreMenu 创建更多选项菜单
func (hms *HybridMenuSystem) CreateMoreMenu(userID int64) (*tgbotapi.ReplyKeyboardMarkup, *tgbotapi.InlineKeyboardMarkup) {
	// 导航键盘 (ReplyKeyboard)
	var replyKeyboard *tgbotapi.ReplyKeyboardMarkup
	if hms.useReplyForNavigation {
		keyboard := tgbotapi.NewReplyKeyboard(
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("📜 游戏规则"),
				tgbotapi.NewKeyboardButton("📚 新手教程"),
			),
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("📞 联系客服"),
				tgbotapi.NewKeyboardButton("ℹ️ 关于我们"),
			),
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("↩️ 返回主菜单"),
			),
		)

		keyboard.ResizeKeyboard = true
		keyboard.OneTimeKeyboard = false
		replyKeyboard = &keyboard
	}

	// 快速操作键盘 (InlineKeyboard)
	var inlineKeyboard *tgbotapi.InlineKeyboardMarkup
	if hms.useInlineForActions {
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("📖 游戏指南", "game_guide"),
				tgbotapi.NewInlineKeyboardButtonData("❓ 常见问题", "faq"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonURL("🌐 官网", "https://example.com"),
				tgbotapi.NewInlineKeyboardButtonURL("🔊 官方频道", "https://t.me/examplechannel"),
			),
		)
		inlineKeyboard = &keyboard
	}

	return replyKeyboard, inlineKeyboard
}

// CreateSettingsMenu 创建设置菜单
func (hms *HybridMenuSystem) CreateSettingsMenu(userID int64) (*tgbotapi.ReplyKeyboardMarkup, *tgbotapi.InlineKeyboardMarkup) {
	// 导航键盘 (ReplyKeyboard)
	var replyKeyboard *tgbotapi.ReplyKeyboardMarkup
	if hms.useReplyForNavigation {
		keyboard := tgbotapi.NewReplyKeyboard(
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("🌐 语言设置"),
				tgbotapi.NewKeyboardButton("🔔 通知设置"),
			),
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("🛡️ 隐私设置"),
				tgbotapi.NewKeyboardButton("🧩 界面设置"),
			),
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("↩️ 返回主菜单"),
			),
		)

		keyboard.ResizeKeyboard = true
		keyboard.OneTimeKeyboard = false
		replyKeyboard = &keyboard
	}

	// 快速操作键盘 (InlineKeyboard)
	var inlineKeyboard *tgbotapi.InlineKeyboardMarkup
	if hms.useInlineForActions {
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔒 账户安全", "account_security"),
				tgbotapi.NewInlineKeyboardButtonData("🔑 修改密码", "change_password"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔕 关闭通知", "disable_notifications"),
				tgbotapi.NewInlineKeyboardButtonData("🔔 开启通知", "enable_notifications"),
			),
		)
		inlineKeyboard = &keyboard
	}

	return replyKeyboard, inlineKeyboard
}

// CreateAdminMenu 创建管理员菜单
func (hms *HybridMenuSystem) CreateAdminMenu(userID int64) (*tgbotapi.ReplyKeyboardMarkup, *tgbotapi.InlineKeyboardMarkup) {
	// 导航键盘 (ReplyKeyboard)
	var replyKeyboard *tgbotapi.ReplyKeyboardMarkup
	if hms.useReplyForNavigation {
		keyboard := tgbotapi.NewReplyKeyboard(
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("📊 系统状态"),
				tgbotapi.NewKeyboardButton("👥 用户管理"),
			),
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("🎮 游戏设置"),
				tgbotapi.NewKeyboardButton("💰 财务管理"),
			),
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButton("↩️ 返回主菜单"),
			),
		)

		keyboard.ResizeKeyboard = true
		keyboard.OneTimeKeyboard = false
		replyKeyboard = &keyboard
	}

	// 快速操作键盘 (InlineKeyboard)
	var inlineKeyboard *tgbotapi.InlineKeyboardMarkup
	if hms.useInlineForActions {
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("⚠️ 系统告警", "system_alerts"),
				tgbotapi.NewInlineKeyboardButtonData("📝 操作日志", "operation_logs"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("👤 查找用户", "find_user"),
				tgbotapi.NewInlineKeyboardButtonData("🔒 封禁用户", "ban_user"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("⚙️ 全局设置", "global_settings"),
				tgbotapi.NewInlineKeyboardButtonData("💹 数据统计", "statistics"),
			),
		)
		inlineKeyboard = &keyboard
	}

	return replyKeyboard, inlineKeyboard
}

// GetMenuByMenuType 根据菜单类型获取对应的菜单
func (hms *HybridMenuSystem) GetMenuByMenuType(menuType MenuType, userID int64, isAdmin bool) (*tgbotapi.ReplyKeyboardMarkup, *tgbotapi.InlineKeyboardMarkup) {
	switch menuType {
	case MenuTypeMain:
		return hms.CreateMainMenu(userID, isAdmin)
	case MenuTypeGameCenter:
		return hms.CreateGameCenterMenu(userID)
	case MenuTypeFinance:
		return hms.CreateFinanceMenu(userID)
	case MenuTypeMore:
		return hms.CreateMoreMenu(userID)
	case MenuTypeSettings:
		return hms.CreateSettingsMenu(userID)
	case MenuTypeAdmin:
		if isAdmin {
			return hms.CreateAdminMenu(userID)
		}
		return hms.CreateMainMenu(userID, isAdmin)
	default:
		return hms.CreateMainMenu(userID, isAdmin)
	}
}

// SetAdaptiveMode 设置自适应模式
func (hms *HybridMenuSystem) SetAdaptiveMode(enabled bool) {
	hms.adaptiveMode = enabled
	log.Printf("🔧 混合菜单系统自适应模式: %v", enabled)
}
