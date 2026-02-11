// Package aliyun 实现阿里云 DNS API 对接。
// 本文件实现阿里云 API 的 HMAC-SHA1 签名算法，
// 用于对所有 API 请求进行身份认证。
package aliyun

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Sign 对阿里云 API 请求参数进行 HMAC-SHA1 签名。
// 该函数会向 params 中添加公共参数（AccessKeyId、时间戳、签名方法等），
// 然后按照阿里云签名规范计算签名值并添加到 params 中。
//
// accessKeyID: 阿里云 AccessKey ID
// accessKeySecret: 阿里云 AccessKey Secret
// params: API 请求参数，签名完成后会包含所有公共参数和 Signature
// httpMethod: HTTP 请求方法，通常为 "GET" 或 "POST"
func Sign(accessKeyID, accessKeySecret string, params *url.Values, httpMethod string) {
	// 1. 填充公共参数
	params.Set("SignatureMethod", "HMAC-SHA1")
	params.Set("SignatureNonce", fmt.Sprintf("%d", time.Now().UnixNano()))
	params.Set("AccessKeyId", accessKeyID)
	params.Set("SignatureVersion", "1.0")
	params.Set("Timestamp", time.Now().UTC().Format("2006-01-02T15:04:05Z"))
	params.Set("Format", "JSON")
	params.Set("Version", "2015-01-09")

	// 2. 构造待签名字符串
	stringToSign := buildStringToSign(httpMethod, params)

	// 3. 计算 HMAC-SHA1 签名
	// 阿里云签名密钥为 AccessKeySecret + "&"
	signature := computeSignature(accessKeySecret+"&", stringToSign)

	// 4. 将签名添加到参数中
	params.Set("Signature", signature)
}

// buildStringToSign 构造待签名字符串。
// 按照阿里云签名规范：HTTPMethod + "&" + percentEncode("/") + "&" + percentEncode(排序后的参数字符串)
func buildStringToSign(httpMethod string, params *url.Values) string {
	// 获取所有参数键并排序
	keys := make([]string, 0, len(*params))
	for k := range *params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 构造规范化查询字符串
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, percentEncode(k)+"="+percentEncode(params.Get(k)))
	}
	canonicalizedQueryString := strings.Join(pairs, "&")

	// 构造待签名字符串
	stringToSign := httpMethod + "&" + percentEncode("/") + "&" + percentEncode(canonicalizedQueryString)
	return stringToSign
}

// computeSignature 使用 HMAC-SHA1 算法计算签名并返回 Base64 编码结果。
// key: 签名密钥（AccessKeySecret + "&"）
// data: 待签名字符串
func computeSignature(key, data string) string {
	mac := hmac.New(sha1.New, []byte(key))
	mac.Write([]byte(data))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// percentEncode 实现阿里云特殊 URL 编码规则。
// 与标准 URL 编码的区别：
// - 空格编码为 %20（而非 +）
// - 波浪号 ~ 不编码
// - 星号 * 编码为 %2A
// - 其他特殊字符使用标准百分号编码
func percentEncode(s string) string {
	encoded := url.QueryEscape(s)
	// url.QueryEscape 将空格编码为 +，需要替换为 %20
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	// url.QueryEscape 会编码 ~，但阿里云要求 ~ 不编码
	encoded = strings.ReplaceAll(encoded, "%7E", "~")
	// url.QueryEscape 不会编码 *，但阿里云要求 * 编码为 %2A
	encoded = strings.ReplaceAll(encoded, "*", "%2A")
	return encoded
}
