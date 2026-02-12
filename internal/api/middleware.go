// Package api JWT 认证中间件
package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// JWTClaims 自定义 JWT 声明
type JWTClaims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// GenerateToken 生成 JWT token
// jwtSecret: JWT 签名密钥
// userID: 用户 ID
// username: 用户名
// expiration: token 有效期
func GenerateToken(jwtSecret []byte, userID uint, username string, expiration time.Duration) (string, error) {
	claims := JWTClaims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// JWTAuthMiddleware 创建 JWT 认证中间件
// jwtSecret: JWT 签名密钥，从外部传入（方便测试）
func JWTAuthMiddleware(jwtSecret []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从 Authorization 头中提取 token
		authHeader := c.GetHeader("Authorization")
		tokenString := ""

		if authHeader != "" {
			// 检查 Bearer 前缀
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "认证令牌格式错误"})
				c.Abort()
				return
			}
			tokenString = parts[1]
		} else if queryToken := c.Query("token"); queryToken != "" {
			// 支持通过URL参数传递token（用于SSE等不支持自定义Header的场景）
			tokenString = queryToken
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未提供认证令牌"})
			c.Abort()
			return
		}

		// 解析并验证 token
		claims := &JWTClaims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			// 确保签名方法是 HMAC
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "认证令牌无效或已过期"})
			c.Abort()
			return
		}

		// 将用户信息存入 gin.Context
		c.Set("userID", claims.UserID)
		c.Set("username", claims.Username)

		c.Next()
	}
}
