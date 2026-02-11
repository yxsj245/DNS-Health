// Package crypto 凭证加密模块，提供 AES-GCM 加密解密和脱敏功能
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

var (
	// ErrInvalidKeyLength 密钥长度不合法（必须为 32 字节）
	ErrInvalidKeyLength = errors.New("加密密钥长度必须为 32 字节（AES-256）")
	// ErrCiphertextTooShort 密文太短，无法包含 nonce
	ErrCiphertextTooShort = errors.New("密文数据太短，无法解密")
	// ErrEmptyPlaintext 明文为空
	ErrEmptyPlaintext = errors.New("明文不能为空")
	// ErrEmptyCiphertext 密文为空
	ErrEmptyCiphertext = errors.New("密文不能为空")
)

// Encrypt 使用 AES-256-GCM 加密明文，返回 base64 编码的密文
// key 必须为 32 字节长度
// 返回格式：base64(nonce + ciphertext)
func Encrypt(plaintext string, key []byte) (string, error) {
	if len(key) != 32 {
		return "", ErrInvalidKeyLength
	}
	if len(plaintext) == 0 {
		return "", ErrEmptyPlaintext
	}

	// 创建 AES cipher block
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("创建 AES cipher 失败: %w", err)
	}

	// 创建 GCM 模式
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("创建 GCM 模式失败: %w", err)
	}

	// 生成随机 nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("生成随机 nonce 失败: %w", err)
	}

	// 加密：nonce 作为前缀拼接到密文前面
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	// 返回 base64 编码的结果
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt 使用 AES-256-GCM 解密 base64 编码的密文，返回明文
// key 必须为 32 字节长度
func Decrypt(ciphertext string, key []byte) (string, error) {
	if len(key) != 32 {
		return "", ErrInvalidKeyLength
	}
	if len(ciphertext) == 0 {
		return "", ErrEmptyCiphertext
	}

	// base64 解码
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("base64 解码失败: %w", err)
	}

	// 创建 AES cipher block
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("创建 AES cipher 失败: %w", err)
	}

	// 创建 GCM 模式
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("创建 GCM 模式失败: %w", err)
	}

	// 检查密文长度是否足够包含 nonce
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", ErrCiphertextTooShort
	}

	// 分离 nonce 和实际密文
	nonce, encryptedData := data[:nonceSize], data[nonceSize:]

	// 解密
	plaintext, err := gcm.Open(nil, nonce, encryptedData, nil)
	if err != nil {
		return "", fmt.Errorf("解密失败: %w", err)
	}

	return string(plaintext), nil
}

// MaskSecret 脱敏显示敏感信息
// 对于长度 >= 8 的字符串：显示前四位 + **** + 后四位
// 对于长度 < 8 的字符串：进行适当脱敏处理
func MaskSecret(secret string) string {
	length := len(secret)

	switch {
	case length == 0:
		return ""
	case length <= 2:
		// 长度 1-2：全部用星号替代
		return maskAll(length)
	case length <= 4:
		// 长度 3-4：显示第一位 + 星号
		return string(secret[0]) + maskAll(length-1)
	case length < 8:
		// 长度 5-7：显示前两位 + 星号 + 后两位
		return secret[:2] + "****" + secret[length-2:]
	default:
		// 长度 >= 8：显示前四位 + **** + 后四位
		return secret[:4] + "****" + secret[length-4:]
	}
}

// maskAll 返回指定数量的星号
func maskAll(n int) string {
	result := make([]byte, n)
	for i := range result {
		result[i] = '*'
	}
	return string(result)
}
