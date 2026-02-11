// Package api 登录/登出/注册/账户管理接口实现
package api

import (
	"dns-health-monitor/internal/model"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// AuthHandler 认证处理器
type AuthHandler struct {
	DB          *gorm.DB
	JWTSecret   []byte
	TokenExpiry time.Duration // token 有效期，默认 24 小时
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler(db *gorm.DB, jwtSecret []byte, tokenExpiry time.Duration) *AuthHandler {
	if tokenExpiry <= 0 {
		tokenExpiry = 24 * time.Hour
	}
	return &AuthHandler{
		DB:          db,
		JWTSecret:   jwtSecret,
		TokenExpiry: tokenExpiry,
	}
}

// LoginRequest 登录请求体
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse 登录成功响应
type LoginResponse struct {
	Token    string `json:"token"`
	Username string `json:"username"`
}

// RegisterRequest 注册请求体
type RegisterRequest struct {
	Username        string `json:"username" binding:"required,min=3,max=32"`
	Password        string `json:"password" binding:"required,min=6,max=64"`
	ConfirmPassword string `json:"confirm_password" binding:"required"`
}

// ChangePasswordRequest 修改密码请求体
type ChangePasswordRequest struct {
	OldPassword     string `json:"old_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required,min=6,max=64"`
	ConfirmPassword string `json:"confirm_password" binding:"required"`
}

// ChangeUsernameRequest 修改用户名请求体
type ChangeUsernameRequest struct {
	NewUsername string `json:"new_username" binding:"required,min=3,max=32"`
	Password    string `json:"password" binding:"required"`
}

// CheckSetup 检查系统是否需要初始化注册
// GET /api/setup-status
// 如果数据库中没有任何用户，返回 {"need_setup": true}
func (h *AuthHandler) CheckSetup(c *gin.Context) {
	var count int64
	if err := h.DB.Model(&model.User{}).Count(&count).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询用户失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"need_setup": count == 0})
}

// Register 首次注册管理员账户
// POST /api/register
// 仅在数据库中没有任何用户时允许注册
func (h *AuthHandler) Register(c *gin.Context) {
	// 检查是否已有用户，防止重复注册
	var count int64
	if err := h.DB.Model(&model.User{}).Count(&count).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询用户失败"})
		return
	}
	if count > 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "系统已初始化，不允许注册"})
		return
	}

	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效，用户名至少3位，密码至少6位"})
		return
	}

	if req.Password != req.ConfirmPassword {
		c.JSON(http.StatusBadRequest, gin.H{"error": "两次输入的密码不一致"})
		return
	}

	// 使用 bcrypt 对密码进行哈希
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "密码加密失败"})
		return
	}

	user := model.User{
		Username:     req.Username,
		PasswordHash: string(hashedPassword),
	}

	if err := h.DB.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建用户失败"})
		return
	}

	// 注册成功后自动生成 token，免去再次登录
	token, err := GenerateToken(h.JWTSecret, user.ID, user.Username, h.TokenExpiry)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成令牌失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "注册成功",
		"token":    token,
		"username": user.Username,
	})
}

// Login 处理用户登录
// POST /api/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效"})
		return
	}

	// 查询用户
	var user model.User
	if err := h.DB.Where("username = ?", req.Username).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}

	// 生成 JWT token
	token, err := GenerateToken(h.JWTSecret, user.ID, user.Username, h.TokenExpiry)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成令牌失败"})
		return
	}

	c.JSON(http.StatusOK, LoginResponse{
		Token:    token,
		Username: user.Username,
	})
}

// Logout 处理用户登出
// POST /api/logout
func (h *AuthHandler) Logout(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "已成功退出登录"})
}

// ChangePassword 修改密码
// PUT /api/account/password
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	userID, _ := c.Get("userID")

	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效，新密码至少6位"})
		return
	}

	if req.NewPassword != req.ConfirmPassword {
		c.JSON(http.StatusBadRequest, gin.H{"error": "两次输入的新密码不一致"})
		return
	}

	// 查询当前用户
	var user model.User
	if err := h.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}

	// 验证旧密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.OldPassword)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "原密码错误"})
		return
	}

	// 生成新密码哈希
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "密码加密失败"})
		return
	}

	// 更新密码
	if err := h.DB.Model(&user).Update("password_hash", string(hashedPassword)).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "修改密码失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "密码修改成功"})
}

// ChangeUsername 修改用户名
// PUT /api/account/username
func (h *AuthHandler) ChangeUsername(c *gin.Context) {
	userID, _ := c.Get("userID")

	var req ChangeUsernameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效，用户名至少3位"})
		return
	}

	// 查询当前用户
	var user model.User
	if err := h.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "密码错误"})
		return
	}

	// 检查新用户名是否已被占用
	var existing model.User
	if err := h.DB.Where("username = ? AND id != ?", req.NewUsername, userID).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "该用户名已被使用"})
		return
	}

	// 更新用户名
	if err := h.DB.Model(&user).Update("username", req.NewUsername).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "修改用户名失败"})
		return
	}

	// 生成新 token（因为用户名变了）
	token, err := GenerateToken(h.JWTSecret, user.ID, req.NewUsername, h.TokenExpiry)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成令牌失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "用户名修改成功",
		"token":    token,
		"username": req.NewUsername,
	})
}

// GetAccountInfo 获取当前用户信息
// GET /api/account
func (h *AuthHandler) GetAccountInfo(c *gin.Context) {
	userID, _ := c.Get("userID")
	username, _ := c.Get("username")

	c.JSON(http.StatusOK, gin.H{
		"id":       userID,
		"username": username,
	})
}
