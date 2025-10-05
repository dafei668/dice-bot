package admin

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strconv"

	"telegram-dice-bot/internal/auth"
	"telegram-dice-bot/internal/bot"
	"telegram-dice-bot/internal/database"
	"telegram-dice-bot/internal/game"
	"telegram-dice-bot/internal/models"

	"github.com/gorilla/mux"
)

type AdminHandler struct {
	db          *database.DB
	gameManager *game.Manager
	bot         *bot.Bot
	templates   *template.Template
}

func NewAdminHandler(db *database.DB, gameManager *game.Manager, bot *bot.Bot) *AdminHandler {
	// 解析模板
	templates := template.Must(template.ParseGlob("web/admin/templates/*.html"))

	return &AdminHandler{
		db:          db,
		gameManager: gameManager,
		bot:         bot,
		templates:   templates,
	}
}

// Dashboard 仪表板页面
func (h *AdminHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	log.Printf("Dashboard handler called for path: %s", r.URL.Path)

	// 获取统计数据
	totalUsers, _ := h.db.GetTotalUsersCount()
	activeUsers, _ := h.db.GetActiveUsersCount()
	todayGames, _ := h.db.GetTodayGamesCount()
	totalRecharge, _ := h.db.GetTotalRechargeAmount()

	data := map[string]interface{}{
		"Title": "仪表板",
		"Stats": map[string]interface{}{
			"total_users":    totalUsers,
			"active_users":   activeUsers,
			"total_games":    todayGames,
			"total_recharge": float64(totalRecharge) / 100, // 转换为元
			"update_time":    "刚刚",
		},
	}

	log.Printf("Dashboard data: %+v", data)

	err := h.templates.ExecuteTemplate(w, "layout", data)
	if err != nil {
		log.Printf("Dashboard template error: %v", err)
		http.Error(w, "模板渲染失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

// Users 用户管理页面
func (h *AdminHandler) Users(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	limit := 20
	offset := (page - 1) * limit

	users, err := h.db.GetUsersWithPagination(offset, limit)
	if err != nil {
		http.Error(w, "获取用户数据失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	totalUsers, _ := h.db.GetTotalUsersCount()
	totalPages := (totalUsers + limit - 1) / limit

	data := map[string]interface{}{
		"Title":      "用户管理",
		"Users":      users,
		"Page":       page,
		"TotalPages": totalPages,
		"HasPrev":    page > 1,
		"HasNext":    page < totalPages,
		"PrevPage":   page - 1,
		"NextPage":   page + 1,
	}

	err = h.templates.ExecuteTemplate(w, "users.html", data)
	if err != nil {
		http.Error(w, "模板渲染失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

// Games 游戏记录页面
func (h *AdminHandler) Games(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	limit := 20
	offset := (page - 1) * limit

	games, err := h.db.GetGamesWithPagination(offset, limit)
	if err != nil {
		http.Error(w, "获取游戏数据失败", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Title": "游戏记录",
		"Games": games,
		"Page":  page,
	}

	h.templates.ExecuteTemplate(w, "games.html", data)
}

// Recharges 充值记录页面
func (h *AdminHandler) Recharges(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	limit := 20
	offset := (page - 1) * limit

	recharges, err := h.db.GetRechargesWithPagination(offset, limit)
	if err != nil {
		http.Error(w, "获取充值数据失败", http.StatusInternalServerError)
		return
	}

	totalRecharges, _ := h.db.GetTotalRechargesCount()
	totalPages := (totalRecharges + limit - 1) / limit

	data := map[string]interface{}{
		"Title":      "充值记录",
		"Recharges":  recharges,
		"Page":       page,
		"TotalPages": totalPages,
		"HasPrev":    page > 1,
		"HasNext":    page < totalPages,
		"PrevPage":   page - 1,
		"NextPage":   page + 1,
	}

	h.templates.ExecuteTemplate(w, "recharges.html", data)
}

// API接口

// APIStats 获取统计数据
func (h *AdminHandler) APIStats(w http.ResponseWriter, r *http.Request) {
	totalUsers, _ := h.db.GetTotalUsersCount()
	activeUsers, _ := h.db.GetActiveUsersCount()
	todayGames, _ := h.db.GetTodayGamesCount()
	totalRecharge, _ := h.db.GetTotalRechargeAmount()

	stats := map[string]interface{}{
		"totalUsers":    totalUsers,
		"activeUsers":   activeUsers,
		"todayGames":    todayGames,
		"totalRecharge": float64(totalRecharge) / 100,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// APIUpdateUserBalance 更新用户余额
func (h *AdminHandler) APIUpdateUserBalance(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userID, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		http.Error(w, "无效的用户ID", http.StatusBadRequest)
		return
	}

	var req struct {
		Balance float64 `json:"balance"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "无效的请求数据", http.StatusBadRequest)
		return
	}

	// 更新用户余额
	newBalance := int64(req.Balance * 100) // 转换为分
	err = h.db.UpdateUserBalance(userID, newBalance)
	if err != nil {
		http.Error(w, "更新余额失败", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// APIDeleteUser 删除用户
func (h *AdminHandler) APIDeleteUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userID, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		http.Error(w, "无效的用户ID", http.StatusBadRequest)
		return
	}

	err = h.db.DeleteUser(userID)
	if err != nil {
		http.Error(w, "删除用户失败", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// LoginPage 显示登录页面
func (h *AdminHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	// 如果已经登录，重定向到仪表板
	if auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/admin", http.StatusFound)
		return
	}

	tmpl, err := template.ParseFiles("web/admin/templates/login.html")
	if err != nil {
		http.Error(w, "模板加载失败", http.StatusInternalServerError)
		return
	}

	data := struct {
		Error string
	}{
		Error: r.URL.Query().Get("error"),
	}

	tmpl.Execute(w, data)
}

// LoginHandler 处理登录请求
func (h *AdminHandler) LoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/admin/login", http.StatusFound)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	log.Printf("Login attempt - Username: %s", username)

	if auth.ValidateCredentials(username, password) {
		log.Printf("Login credentials valid for user: %s", username)
		err := auth.Login(w, r)
		if err != nil {
			log.Printf("Login session creation failed for user %s: %v", username, err)
			http.Redirect(w, r, "/admin/login?error=登录失败", http.StatusFound)
			return
		}
		log.Printf("Login successful for user: %s", username)
		http.Redirect(w, r, "/admin", http.StatusFound)
	} else {
		log.Printf("Login credentials invalid for user: %s", username)
		http.Redirect(w, r, "/admin/login?error=用户名或密码错误", http.StatusFound)
		return
	}
}

// LogoutHandler 处理登出请求
func (h *AdminHandler) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	err := auth.Logout(w, r)
	if err != nil {
		http.Error(w, "登出失败", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/login", http.StatusFound)
}

// APIGetUsers 获取用户列表API
func (h *AdminHandler) APIGetUsers(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	limit := 20
	offset := (page - 1) * limit

	// 获取筛选参数
	search := r.URL.Query().Get("search")
	status := r.URL.Query().Get("status")
	sortBy := r.URL.Query().Get("sort_by")

	users, err := h.db.GetUsersWithFilters(offset, limit, search, status, sortBy)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "获取用户数据失败",
		})
		return
	}

	totalUsers, _ := h.db.GetTotalUsersCountWithFilters(search, status)
	totalPages := (totalUsers + limit - 1) / limit

	response := map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"users": users,
			"pagination": map[string]interface{}{
				"current_page": page,
				"total_pages":  totalPages,
				"total_users":  totalUsers,
				"per_page":     limit,
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// APICreateUser 创建用户API
func (h *AdminHandler) APICreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID       int64   `json:"id"`
		Username string  `json:"username"`
		Balance  float64 `json:"balance"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "无效的请求数据",
		})
		return
	}

	// 检查用户是否已存在
	existingUser, _ := h.db.GetUser(req.ID)
	if existingUser != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "用户ID已存在",
		})
		return
	}

	// 创建新用户
	newUser := &models.User{
		ID:       req.ID,
		Username: req.Username,
		Balance:  int64(req.Balance * 100), // 转换为分
	}

	err := h.db.CreateUser(newUser)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "创建用户失败",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "用户创建成功",
	})
}

// APIGetRecharges 获取充值记录列表API
func (h *AdminHandler) APIGetRecharges(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	limit := 20
	offset := (page - 1) * limit

	recharges, err := h.db.GetRechargesWithPagination(offset, limit)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "获取充值数据失败",
		})
		return
	}

	// 获取总数用于分页
	totalRecharges, _ := h.db.GetTotalRechargesCount()
	totalPages := (totalRecharges + limit - 1) / limit

	// 转换充值数据格式
	rechargeList := make([]map[string]interface{}, len(recharges))
	for i, recharge := range recharges {
		// 获取用户信息
		user, _ := h.db.GetUser(recharge.UserID)
		username := "未知用户"
		if user != nil {
			username = user.FirstName
			if user.Username != "" {
				username = "@" + user.Username
			}
		}

		rechargeList[i] = map[string]interface{}{
			"id":         recharge.ID,
			"user_id":    recharge.UserID,
			"username":   username,
			"amount":     float64(recharge.Amount) / 100, // 转换为元
			"type":       recharge.Type,
			"status":     "completed", // 默认状态
			"created_at": recharge.CreatedAt,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"data":        rechargeList,
		"page":        page,
		"total_pages": totalPages,
		"total":       totalRecharges,
	})
}

// APIGetUser 获取单个用户信息API
func (h *AdminHandler) APIGetUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userID, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "无效的用户ID",
		})
		return
	}

	user, err := h.db.GetUser(userID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "用户不存在",
		})
		return
	}

	// 转换用户数据
	userData := map[string]interface{}{
		"id":         user.ID,
		"username":   user.Username,
		"first_name": user.FirstName,
		"last_name":  user.LastName,
		"balance":    float64(user.Balance) / 100, // 转换为元
		"status":     "active",                    // 默认状态，可以根据需要扩展
		"created_at": user.CreatedAt,
		"updated_at": user.UpdatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(userData)
}

// APIUpdateUser 更新用户信息API
func (h *AdminHandler) APIUpdateUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userID, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "无效的用户ID",
		})
		return
	}

	var req struct {
		Username string  `json:"username"`
		Balance  float64 `json:"balance"`
		Status   string  `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "无效的请求数据",
		})
		return
	}

	// 更新用户信息
	err = h.db.UpdateUserInfo(userID, req.Username, int64(req.Balance*100))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "更新用户失败",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "用户更新成功",
	})
}

// APIGetGames 获取游戏列表API
func (h *AdminHandler) APIGetGames(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	limit := 20
	offset := (page - 1) * limit

	games, err := h.db.GetGamesWithPagination(offset, limit)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "获取游戏数据失败",
		})
		return
	}

	// 转换游戏数据格式
	gameList := make([]map[string]interface{}, len(games))
	for i, game := range games {
		// 获取玩家1信息
		player1, _ := h.db.GetUser(game.Player1ID)
		player1Name := "未知用户"
		if player1 != nil {
			player1Name = player1.FirstName
			if player1.Username != "" {
				player1Name = "@" + player1.Username
			}
		}

		// 获取玩家2信息
		player2Name := "等待中"
		if game.Player2ID != nil {
			player2, _ := h.db.GetUser(*game.Player2ID)
			if player2 != nil {
				player2Name = player2.FirstName
				if player2.Username != "" {
					player2Name = "@" + player2.Username
				}
			}
		}

		gameList[i] = map[string]interface{}{
			"id":         game.ID,
			"player1_id": game.Player1ID,
			"player1":    player1Name,
			"player2_id": game.Player2ID,
			"player2":    player2Name,
			"bet_amount": float64(game.BetAmount) / 100, // 转换为元
			"status":     game.Status,
			"winner_id":  game.WinnerID,
			"created_at": game.CreatedAt,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    gameList,
		"page":    page,
		"total":   len(gameList),
	})
}
