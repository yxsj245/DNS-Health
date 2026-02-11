// Package tencentcloud 实现腾讯云 DNSPod API 3.0 对接。
// 本文件实现腾讯云 API v3 的 TC3-HMAC-SHA256 签名算法，
// 用于对所有 API 请求进行身份认证。
package tencentcloud

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// sign 对腾讯云 API 请求进行 TC3-HMAC-SHA256 签名。
// 按照腾讯云 API v3 签名规范，计算签名并设置请求头。
//
// secretID: 腾讯云 SecretId
// secretKey: 腾讯云 SecretKey
// r: HTTP 请求对象，签名完成后会设置 Authorization、Host、X-TC-Action、X-TC-Timestamp 头
// action: API 操作名称，如 "DescribeRecordList"
// payload: 请求体 JSON 字符串
// service: 服务名称，DNSPod 为 "dnspod"
func sign(secretID, secretKey string, r *http.Request, action, payload, service string) {
	algorithm := "TC3-HMAC-SHA256"
	host := service + ".tencentcloudapi.com"
	timestamp := time.Now().Unix()
	timestampStr := strconv.FormatInt(timestamp, 10)

	// 第一步：构造规范请求串（Canonical Request）
	canonicalHeaders := "content-type:application/json\nhost:" + host + "\nx-tc-action:" + strings.ToLower(action) + "\n"
	signedHeaders := "content-type;host;x-tc-action"
	hashedRequestPayload := sha256hex(payload)
	canonicalRequest := "POST\n/\n\n" + canonicalHeaders + "\n" + signedHeaders + "\n" + hashedRequestPayload

	// 第二步：构造待签名字符串（String to Sign）
	date := time.Unix(timestamp, 0).UTC().Format("2006-01-02")
	credentialScope := date + "/" + service + "/tc3_request"
	hashedCanonicalRequest := sha256hex(canonicalRequest)
	string2sign := algorithm + "\n" + timestampStr + "\n" + credentialScope + "\n" + hashedCanonicalRequest

	// 第三步：计算签名（Signature）
	secretDate := hmacSHA256(date, "TC3"+secretKey)
	secretService := hmacSHA256(service, secretDate)
	secretSigning := hmacSHA256("tc3_request", secretService)
	signature := hex.EncodeToString([]byte(hmacSHA256(string2sign, secretSigning)))

	// 第四步：构造 Authorization 头
	authorization := algorithm + " Credential=" + secretID + "/" + credentialScope +
		", SignedHeaders=" + signedHeaders + ", Signature=" + signature

	// 设置请求头
	r.Header.Set("Authorization", authorization)
	r.Header.Set("Host", host)
	r.Header.Set("X-TC-Action", action)
	r.Header.Set("X-TC-Timestamp", timestampStr)
}

// sha256hex 计算字符串的 SHA256 哈希值并返回十六进制编码。
func sha256hex(s string) string {
	b := sha256.Sum256([]byte(s))
	return hex.EncodeToString(b[:])
}

// hmacSHA256 使用 HMAC-SHA256 算法计算消息认证码。
// 返回原始字节的字符串表示（非十六进制编码），用于签名链式派生。
func hmacSHA256(s, key string) string {
	hashed := hmac.New(sha256.New, []byte(key))
	hashed.Write([]byte(s))
	return string(hashed.Sum(nil))
}
