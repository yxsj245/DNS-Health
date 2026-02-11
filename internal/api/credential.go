// Package api 凭证管理接口实现
// 支持通用凭证字段，不同服务商可以有不同的凭证字段结构
package api

import (
	"dns-health-monitor/internal/crypto"
	"dns-health-monitor/internal/model"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// CredentialHandler 凭证管理处理器
type CredentialHandler struct {
	DB         *gorm.DB
	EncryptKey []byte // AES-256 加密密钥（32 字节）
}

// NewCredentialHandler 创建凭证管理处理器
func NewCredentialHandler(db *gorm.DB, encryptKey []byte) *CredentialHandler {
	return &CredentialHandler{
		DB:         db,
		EncryptKey: encryptKey,
	}
}

// CreateCredentialRequest 创建凭证请求体
// Credentials 为通用字段映射，不同服务商传入不同的 key-value
type CreateCredentialRequest struct {
	ProviderType string            `json:"provider_type" binding:"required"`
	Name         string            `json:"name" binding:"required"`
	Credentials  map[string]string `json:"credentials" binding:"required"`
}

// UpdateCredentialRequest 更新凭证请求体
// 不允许变更 provider_type，只能修改名称和凭证字段
// Credentials 中的字段为空字符串或不传则保留原值
type UpdateCredentialRequest struct {
	Name        string            `json:"name" binding:"required"`
	Credentials map[string]string `json:"credentials"`
}

// CredentialResponse 凭证响应（脱敏）
type CredentialResponse struct {
	ID           uint              `json:"id"`
	ProviderType string            `json:"provider_type"`
	Name         string            `json:"name"`
	Credentials  map[string]string `json:"credentials"` // 脱敏后的凭证字段
	CreatedAt    string            `json:"created_at"`
}

// decryptCredentials 解密凭证字段，返回明文 map
// 优先从新的 CredentialsEncrypted 字段读取，兼容旧数据格式
func (h *CredentialHandler) decryptCredentials(cred model.Credential) (map[string]string, error) {
	// 新格式：CredentialsEncrypted 存储加密后的 JSON
	if cred.CredentialsEncrypted != "" {
		plainJSON, err := crypto.Decrypt(cred.CredentialsEncrypted, h.EncryptKey)
		if err != nil {
			return nil, err
		}
		var fields map[string]string
		if err := json.Unmarshal([]byte(plainJSON), &fields); err != nil {
			return nil, err
		}
		return fields, nil
	}

	// 兼容旧格式：从 AccessKeyIDEncrypted / AccessKeySecretEncrypted 读取
	result := make(map[string]string)
	if cred.AccessKeyIDEncrypted != "" {
		val, err := crypto.Decrypt(cred.AccessKeyIDEncrypted, h.EncryptKey)
		if err != nil {
			val = "****"
		}
		result["access_key_id"] = val
	}
	if cred.AccessKeySecretEncrypted != "" {
		val, err := crypto.Decrypt(cred.AccessKeySecretEncrypted, h.EncryptKey)
		if err != nil {
			val = "****"
		}
		result["access_key_secret"] = val
	}
	return result, nil
}

// ListCredentials 获取凭证列表（脱敏显示）
// GET /api/credentials
func (h *CredentialHandler) ListCredentials(c *gin.Context) {
	var credentials []model.Credential
	if err := h.DB.Order("created_at DESC").Find(&credentials).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询凭证列表失败"})
		return
	}

	resp := make([]CredentialResponse, 0, len(credentials))
	for _, cred := range credentials {
		fields, err := h.decryptCredentials(cred)
		if err != nil {
			// 解密失败时用占位符
			fields = map[string]string{"error": "****"}
		}

		// 对所有字段值进行脱敏
		masked := make(map[string]string, len(fields))
		for k, v := range fields {
			masked[k] = crypto.MaskSecret(v)
		}

		resp = append(resp, CredentialResponse{
			ID:           cred.ID,
			ProviderType: cred.ProviderType,
			Name:         cred.Name,
			Credentials:  masked,
			CreatedAt:    cred.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	c.JSON(http.StatusOK, resp)
}

// CreateCredential 添加凭证
// POST /api/credentials
// 请求体: {"provider_type": "aliyun", "name": "xxx", "credentials": {"access_key_id": "xxx", "access_key_secret": "xxx"}}
func (h *CredentialHandler) CreateCredential(c *gin.Context) {
	var req CreateCredentialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效，provider_type、name、credentials 均为必填"})
		return
	}

	if len(req.Credentials) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "凭证字段不能为空"})
		return
	}

	// 将凭证字段序列化为 JSON 后加密存储
	credJSON, err := json.Marshal(req.Credentials)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "序列化凭证字段失败"})
		return
	}

	encrypted, err := crypto.Encrypt(string(credJSON), h.EncryptKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "加密凭证失败"})
		return
	}

	credential := model.Credential{
		ProviderType:         req.ProviderType,
		Name:                 req.Name,
		CredentialsEncrypted: encrypted,
	}

	if err := h.DB.Create(&credential).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存凭证失败"})
		return
	}

	// 返回脱敏后的凭证
	masked := make(map[string]string, len(req.Credentials))
	for k, v := range req.Credentials {
		masked[k] = crypto.MaskSecret(v)
	}

	c.JSON(http.StatusCreated, CredentialResponse{
		ID:           credential.ID,
		ProviderType: credential.ProviderType,
		Name:         credential.Name,
		Credentials:  masked,
		CreatedAt:    credential.CreatedAt.Format("2006-01-02 15:04:05"),
	})
}

// UpdateCredential 更新凭证
// PUT /api/credentials/:id
// 请求体: {"name": "xxx", "credentials": {"access_key_id": "xxx", "access_key_secret": "xxx"}}
// 不允许变更 provider_type
func (h *CredentialHandler) UpdateCredential(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的凭证 ID"})
		return
	}

	var credential model.Credential
	if err := h.DB.First(&credential, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "凭证不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询凭证失败"})
		return
	}

	var req UpdateCredentialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效，name 为必填"})
		return
	}

	// 解密现有凭证字段，作为基础值
	existingFields, err := h.decryptCredentials(credential)
	if err != nil {
		existingFields = make(map[string]string)
	}

	// 合并：只覆盖用户实际填写的字段（非空值），空值保留原值
	merged := make(map[string]string)
	for k, v := range existingFields {
		merged[k] = v
	}
	for k, v := range req.Credentials {
		if v != "" {
			merged[k] = v
		}
	}

	if len(merged) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "凭证字段不能为空"})
		return
	}

	// 将合并后的凭证字段序列化为 JSON 后加密存储
	credJSON, err := json.Marshal(merged)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "序列化凭证字段失败"})
		return
	}

	encrypted, err := crypto.Encrypt(string(credJSON), h.EncryptKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "加密凭证失败"})
		return
	}

	// 更新字段
	credential.Name = req.Name
	credential.CredentialsEncrypted = encrypted
	// 清空旧格式字段
	credential.AccessKeyIDEncrypted = ""
	credential.AccessKeySecretEncrypted = ""

	if err := h.DB.Save(&credential).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新凭证失败"})
		return
	}

	// 返回脱敏后的凭证
	masked := make(map[string]string, len(merged))
	for k, v := range merged {
		masked[k] = crypto.MaskSecret(v)
	}

	c.JSON(http.StatusOK, CredentialResponse{
		ID:           credential.ID,
		ProviderType: credential.ProviderType,
		Name:         credential.Name,
		Credentials:  masked,
		CreatedAt:    credential.CreatedAt.Format("2006-01-02 15:04:05"),
	})
}

// DeleteCredential 删除凭证
// DELETE /api/credentials/:id
func (h *CredentialHandler) DeleteCredential(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的凭证 ID"})
		return
	}

	var credential model.Credential
	if err := h.DB.First(&credential, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "凭证不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询凭证失败"})
		return
	}

	if err := h.DB.Delete(&credential).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除凭证失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "凭证已删除"})
}
