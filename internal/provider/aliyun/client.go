// Package aliyun 实现阿里云 DNS API 对接。
// 本文件实现阿里云 DNS 客户端，提供 DNS 记录的查询、添加、更新、暂停、启用和删除操作。
package aliyun

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"dns-health-monitor/internal/provider"
)

// 默认阿里云 DNS API 端点
const defaultEndpoint = "https://alidns.aliyuncs.com/"

// AliyunDNSClient 阿里云 DNS 客户端
// 实现 provider.DNSProvider 接口，通过阿里云 DNS API 管理 DNS 解析记录。
type AliyunDNSClient struct {
	// accessKeyID 阿里云 AccessKey ID
	accessKeyID string
	// accessKeySecret 阿里云 AccessKey Secret
	accessKeySecret string
	// endpoint API 端点地址，默认为阿里云官方端点，可自定义用于测试
	endpoint string
	// httpClient HTTP 客户端，用于发送 API 请求
	httpClient *http.Client
}

// describeSubDomainResponse 查询主机记录记录的 API 响应结构
type describeSubDomainResponse struct {
	TotalCount    int `json:"TotalCount"`
	DomainRecords struct {
		Record []aliyunRecord `json:"Record"`
	} `json:"DomainRecords"`
	// 错误响应字段
	RequestID string `json:"RequestId"`
	Code      string `json:"Code"`
	Message   string `json:"Message"`
}

// aliyunRecord 阿里云 DNS 记录结构
type aliyunRecord struct {
	DomainName string `json:"DomainName"`
	RecordID   string `json:"RecordId"`
	RR         string `json:"RR"`
	Type       string `json:"Type"`
	Value      string `json:"Value"`
	TTL        int    `json:"TTL"`
	Status     string `json:"Status"`
}

// aliyunResponse 通用 API 响应结构（用于添加、更新、暂停、删除操作）
type aliyunResponse struct {
	RecordID  string `json:"RecordId"`
	RequestID string `json:"RequestId"`
	Code      string `json:"Code"`
	Message   string `json:"Message"`
}

// describeDomainRecordInfoResponse 查询单条记录详情的 API 响应结构
// 对应阿里云 DescribeDomainRecordInfo API
type describeDomainRecordInfoResponse struct {
	RecordID   string `json:"RecordId"`
	DomainName string `json:"DomainName"`
	RR         string `json:"RR"`
	Type       string `json:"Type"`
	Value      string `json:"Value"`
	TTL        int    `json:"TTL"`
	Status     string `json:"Status"`
	RequestID  string `json:"RequestId"`
	Code       string `json:"Code"`
	Message    string `json:"Message"`
}

// NewAliyunDNSClient 创建阿里云 DNS 客户端实例。
// accessKeyID: 阿里云 AccessKey ID
// accessKeySecret: 阿里云 AccessKey Secret
// 可选参数 endpoint: 自定义 API 端点地址（用于测试），不传则使用默认端点
func NewAliyunDNSClient(accessKeyID, accessKeySecret string, endpoint ...string) *AliyunDNSClient {
	ep := defaultEndpoint
	if len(endpoint) > 0 && endpoint[0] != "" {
		ep = endpoint[0]
	}

	return &AliyunDNSClient{
		accessKeyID:     accessKeyID,
		accessKeySecret: accessKeySecret,
		endpoint:        ep,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SupportsPause 返回阿里云是否支持暂停/启用操作。
// 阿里云支持 SetDomainRecordStatus API，因此返回 true。
func (c *AliyunDNSClient) SupportsPause() bool {
	return true
}

// ListRecords 查询指定域名和主机记录下的所有 DNS 记录。
// 使用阿里云 DescribeSubDomainRecords API 获取记录列表。
// domain: 主域名，例如 "example.com"
// subDomain: 主机记录，例如 "www"
// recordType: 记录类型，"A" 或 "AAAA"
func (c *AliyunDNSClient) ListRecords(ctx context.Context, domain, subDomain, recordType string) ([]provider.DNSRecord, error) {
	// 构造完整主机记录：subDomain.domain（阿里云 API 要求完整格式）
	fullSubDomain := subDomain + "." + domain
	if subDomain == "@" {
		fullSubDomain = "@." + domain
	}

	params := url.Values{}
	params.Set("Action", "DescribeSubDomainRecords")
	params.Set("DomainName", domain)
	params.Set("SubDomain", fullSubDomain)
	params.Set("Type", recordType)

	body, err := c.doRequest(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("查询主机记录记录失败: %w", err)
	}

	var resp describeSubDomainResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析查询响应失败: %w", err)
	}

	// 检查 API 错误
	if resp.Code != "" {
		return nil, fmt.Errorf("阿里云 API 错误 [%s]: %s", resp.Code, resp.Message)
	}

	// 转换为统一的 DNSRecord 格式
	records := make([]provider.DNSRecord, 0, len(resp.DomainRecords.Record))
	for _, r := range resp.DomainRecords.Record {
		records = append(records, provider.DNSRecord{
			RecordID:   r.RecordID,
			DomainName: r.DomainName,
			SubDomain:  r.RR,
			Type:       r.Type,
			Value:      r.Value,
			TTL:        r.TTL,
			Status:     convertStatus(r.Status),
		})
	}

	return records, nil
}

// AddRecord 添加一条新的 DNS 记录。
// 使用阿里云 AddDomainRecord API 创建记录。
// domain: 主域名
// subDomain: 主机记录（RR 值），例如 "www"，"@" 表示根域名
// recordType: 记录类型，"A" 或 "AAAA"
// value: IP 地址
// ttl: 生存时间（秒）
// 返回新创建记录的 ID。
func (c *AliyunDNSClient) AddRecord(ctx context.Context, domain, subDomain, recordType, value string, ttl int) (string, error) {
	params := url.Values{}
	params.Set("Action", "AddDomainRecord")
	params.Set("DomainName", domain)
	params.Set("RR", subDomain)
	params.Set("Type", recordType)
	params.Set("Value", value)
	params.Set("TTL", strconv.Itoa(ttl))

	body, err := c.doRequest(ctx, params)
	if err != nil {
		return "", fmt.Errorf("添加 DNS 记录失败: %w", err)
	}

	var resp aliyunResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("解析添加记录响应失败: %w", err)
	}

	// 检查 API 错误
	if resp.Code != "" {
		return "", fmt.Errorf("阿里云 API 错误 [%s]: %s", resp.Code, resp.Message)
	}

	if resp.RecordID == "" {
		return "", fmt.Errorf("添加记录成功但返回的 RecordId 为空")
	}

	return resp.RecordID, nil
}

// UpdateRecord 更新一条已有的 DNS 记录。
// 使用阿里云 UpdateDomainRecord API 更新记录。
// recordID: 要更新的记录 ID
// subDomain: 主机记录（RR 值）
// recordType: 记录类型
// value: 新的 IP 地址
// ttl: 新的生存时间（秒）
func (c *AliyunDNSClient) UpdateRecord(ctx context.Context, recordID, subDomain, recordType, value string, ttl int) error {
	params := url.Values{}
	params.Set("Action", "UpdateDomainRecord")
	params.Set("RecordId", recordID)
	params.Set("RR", subDomain)
	params.Set("Type", recordType)
	params.Set("Value", value)
	params.Set("TTL", strconv.Itoa(ttl))

	body, err := c.doRequest(ctx, params)
	if err != nil {
		return fmt.Errorf("更新 DNS 记录失败: %w", err)
	}

	var resp aliyunResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("解析更新记录响应失败: %w", err)
	}

	// 检查 API 错误
	if resp.Code != "" {
		return fmt.Errorf("阿里云 API 错误 [%s]: %s", resp.Code, resp.Message)
	}

	return nil
}

// PauseRecord 暂停一条 DNS 记录。
// 使用阿里云 SetDomainRecordStatus API 将记录状态设置为 Disable。
// recordID: 要暂停的记录 ID
func (c *AliyunDNSClient) PauseRecord(ctx context.Context, recordID string) error {
	return c.setRecordStatus(ctx, recordID, "Disable")
}

// ResumeRecord 启用一条已暂停的 DNS 记录。
// 使用阿里云 SetDomainRecordStatus API 将记录状态设置为 Enable。
// recordID: 要启用的记录 ID
func (c *AliyunDNSClient) ResumeRecord(ctx context.Context, recordID string) error {
	return c.setRecordStatus(ctx, recordID, "Enable")
}

// DeleteRecord 删除一条 DNS 记录。
// 使用阿里云 DeleteDomainRecord API 删除记录。
// recordID: 要删除的记录 ID
func (c *AliyunDNSClient) DeleteRecord(ctx context.Context, recordID string) error {
	params := url.Values{}
	params.Set("Action", "DeleteDomainRecord")
	params.Set("RecordId", recordID)

	body, err := c.doRequest(ctx, params)
	if err != nil {
		return fmt.Errorf("删除 DNS 记录失败: %w", err)
	}

	var resp aliyunResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("解析删除记录响应失败: %w", err)
	}

	// 检查 API 错误
	if resp.Code != "" {
		return fmt.Errorf("阿里云 API 错误 [%s]: %s", resp.Code, resp.Message)
	}

	return nil
}

// setRecordStatus 设置 DNS 记录状态（暂停或启用）。
// 使用阿里云 SetDomainRecordStatus API。
// recordID: 记录 ID
// status: "Enable" 或 "Disable"
func (c *AliyunDNSClient) setRecordStatus(ctx context.Context, recordID, status string) error {
	params := url.Values{}
	params.Set("Action", "SetDomainRecordStatus")
	params.Set("RecordId", recordID)
	params.Set("Status", status)

	body, err := c.doRequest(ctx, params)
	if err != nil {
		return fmt.Errorf("设置记录状态失败: %w", err)
	}

	var resp aliyunResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("解析设置状态响应失败: %w", err)
	}

	// 检查 API 错误
	if resp.Code != "" {
		return fmt.Errorf("阿里云 API 错误 [%s]: %s", resp.Code, resp.Message)
	}

	return nil
}

// doRequest 执行阿里云 API 请求。
// 对请求参数进行 HMAC-SHA1 签名，发送 HTTP GET 请求并返回响应体。
// ctx: 上下文，用于控制请求超时和取消
// params: API 请求参数（不含公共参数和签名，由 Sign 函数自动添加）
func (c *AliyunDNSClient) doRequest(ctx context.Context, params url.Values) ([]byte, error) {
	// 使用 signer.go 中的 Sign 函数添加公共参数和签名
	Sign(c.accessKeyID, c.accessKeySecret, &params, "GET")

	// 构造请求 URL
	reqURL := c.endpoint + "?" + params.Encode()

	// 创建 HTTP 请求
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建 HTTP 请求失败: %w", err)
	}

	// 发送请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送 HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应体失败: %w", err)
	}

	// 检查 HTTP 状态码
	if resp.StatusCode != http.StatusOK {
		// 尝试解析错误响应
		var errResp aliyunResponse
		if jsonErr := json.Unmarshal(body, &errResp); jsonErr == nil && errResp.Code != "" {
			return nil, fmt.Errorf("阿里云 API 错误 (HTTP %d) [%s]: %s", resp.StatusCode, errResp.Code, errResp.Message)
		}
		return nil, fmt.Errorf("阿里云 API 请求失败，HTTP 状态码: %d", resp.StatusCode)
	}

	return body, nil
}

// convertStatus 将阿里云记录状态转换为系统内部统一状态。
// 阿里云使用 "ENABLE"/"DISABLE"，系统内部也使用相同格式。
func convertStatus(aliyunStatus string) string {
	switch aliyunStatus {
	case "ENABLE":
		return "ENABLE"
	case "DISABLE":
		return "DISABLE"
	default:
		// 未知状态默认为启用
		return "ENABLE"
	}
}

// UpdateRecordValue 更新解析记录的值（用于故障转移切换）。
// 仅更新记录的值（IP地址或域名），保持其他属性（RR、Type、TTL）不变。
// 实现步骤：
// 1. 先调用 DescribeDomainRecordInfo API 获取记录当前的 RR、Type、TTL 等属性
// 2. 再调用 UpdateDomainRecord API 使用原有属性 + 新值进行更新
// recordID: 要更新的记录 ID
// newValue: 新的 IP 地址或域名
func (c *AliyunDNSClient) UpdateRecordValue(ctx context.Context, recordID, newValue string) error {
	// 第一步：获取记录当前详情，以便保留 RR、Type、TTL 等属性
	recordInfo, err := c.getRecordInfo(ctx, recordID)
	if err != nil {
		return fmt.Errorf("更新记录值失败，无法获取记录详情: %w", err)
	}

	// 第二步：使用原有属性 + 新值调用 UpdateDomainRecord API
	params := url.Values{}
	params.Set("Action", "UpdateDomainRecord")
	params.Set("RecordId", recordID)
	params.Set("RR", recordInfo.RR)
	params.Set("Type", recordInfo.Type)
	params.Set("Value", newValue)
	params.Set("TTL", strconv.Itoa(recordInfo.TTL))

	body, err := c.doRequest(ctx, params)
	if err != nil {
		return fmt.Errorf("更新记录值失败: %w", err)
	}

	var resp aliyunResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("解析更新记录值响应失败: %w", err)
	}

	// 检查 API 错误
	if resp.Code != "" {
		return fmt.Errorf("阿里云 API 错误 [%s]: %s", resp.Code, resp.Message)
	}

	return nil
}

// GetRecordValue 获取解析记录的当前值。
// 调用阿里云 DescribeDomainRecordInfo API 查询记录详情，返回记录的当前值。
// recordID: 要查询的记录 ID
// 返回记录的当前值（IP地址或域名），或错误信息。
func (c *AliyunDNSClient) GetRecordValue(ctx context.Context, recordID string) (string, error) {
	recordInfo, err := c.getRecordInfo(ctx, recordID)
	if err != nil {
		return "", fmt.Errorf("获取记录值失败: %w", err)
	}

	return recordInfo.Value, nil
}

// getRecordInfo 获取单条 DNS 记录的详细信息。
// 调用阿里云 DescribeDomainRecordInfo API。
// recordID: 记录 ID
// 返回记录详情响应结构，或错误信息。
func (c *AliyunDNSClient) getRecordInfo(ctx context.Context, recordID string) (*describeDomainRecordInfoResponse, error) {
	params := url.Values{}
	params.Set("Action", "DescribeDomainRecordInfo")
	params.Set("RecordId", recordID)

	body, err := c.doRequest(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("查询记录详情失败: %w", err)
	}

	var resp describeDomainRecordInfoResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析记录详情响应失败: %w", err)
	}

	// 检查 API 错误
	if resp.Code != "" {
		return nil, fmt.Errorf("阿里云 API 错误 [%s]: %s", resp.Code, resp.Message)
	}

	return &resp, nil
}

// 编译时检查：确保 AliyunDNSClient 实现了 DNSProvider 接口
var _ provider.DNSProvider = (*AliyunDNSClient)(nil)
