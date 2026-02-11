// Package cloudflare 实现 Cloudflare DNS API v4 对接。
// 本文件实现 Cloudflare DNS 客户端，提供 DNS 记录的查询、添加、更新、删除操作，
// 以及 Cloudflare 独有的 CDN 代理（proxied）控制能力。
package cloudflare

import (
	"bytes"
	"context"
	"dns-health-monitor/internal/provider"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Cloudflare API v4 基础地址
const defaultBaseURL = "https://api.cloudflare.com/client/v4"

// CloudflareDNSClient Cloudflare DNS 客户端
// 通过 Cloudflare API v4 管理 DNS 解析记录，使用 Bearer Token 认证。
type CloudflareDNSClient struct {
	// apiToken Cloudflare API Token，需具备 Zone-DNS-Edit 和 Zone-Zone-Read 权限
	apiToken string
	// baseURL API 基础地址，默认为 Cloudflare 官方端点，可自定义用于测试
	baseURL string
	// client HTTP 客户端，用于发送 API 请求
	client *http.Client
	// zoneCache 域名 → Zone ID 的缓存，避免重复查询
	zoneCache sync.Map
}

// CFError Cloudflare API 错误信息结构
type CFError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// CFStatus Cloudflare API 通用响应状态
type CFStatus struct {
	Success  bool      `json:"success"`
	Errors   []CFError `json:"errors"`
	Messages []string  `json:"messages"`
}

// CFZone Cloudflare Zone 信息
type CFZone struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// ZonesResp Cloudflare Zones 列表 API 响应
type ZonesResp struct {
	CFStatus
	Result []CFZone `json:"result"`
}

// CFDNSRecord Cloudflare DNS 记录结构
type CFDNSRecord struct {
	ID      string `json:"id"`
	ZoneID  string `json:"zone_id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

// RecordsResp Cloudflare DNS 记录列表 API 响应
type RecordsResp struct {
	CFStatus
	Result []CFDNSRecord `json:"result"`
}

// SingleRecordResp Cloudflare 单条 DNS 记录 API 响应（用于创建、更新、查询单条记录）
type SingleRecordResp struct {
	CFStatus
	Result CFDNSRecord `json:"result"`
}

// NewCloudflareDNSClient 创建 Cloudflare DNS 客户端实例。
// apiToken: Cloudflare API Token，需具备 Zone-DNS-Edit 和 Zone-Zone-Read 权限
// 可选参数 baseURL: 自定义 API 基础地址（用于测试），不传则使用默认地址
func NewCloudflareDNSClient(apiToken string, baseURL ...string) *CloudflareDNSClient {
	base := defaultBaseURL
	if len(baseURL) > 0 && baseURL[0] != "" {
		base = baseURL[0]
	}

	return &CloudflareDNSClient{
		apiToken: apiToken,
		baseURL:  base,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// doRequest 执行 Cloudflare API 请求。
// 使用 Bearer Token 认证，自动处理 JSON 序列化/反序列化和错误响应。
// ctx: 上下文，用于控制请求超时和取消
// method: HTTP 方法（GET、POST、PUT、PATCH、DELETE）
// url: 完整的 API 请求 URL
// body: 请求体（可为 nil），会被 JSON 序列化
// result: 响应体反序列化目标（可为 nil）
func (c *CloudflareDNSClient) doRequest(ctx context.Context, method, url string, body interface{}, result interface{}) error {
	// 序列化请求体
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("序列化请求体失败: %w", err)
		}
		reqBody = bytes.NewReader(jsonData)
	}

	// 创建 HTTP 请求
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return fmt.Errorf("创建 HTTP 请求失败: %w", err)
	}

	// 设置请求头：Bearer Token 认证和 JSON 内容类型
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("Cloudflare API 请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应体
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应体失败: %w", err)
	}

	// 检查 HTTP 状态码
	if resp.StatusCode >= 300 {
		// 尝试解析 Cloudflare 错误响应
		var cfResp CFStatus
		if jsonErr := json.Unmarshal(respBody, &cfResp); jsonErr == nil && len(cfResp.Errors) > 0 {
			return fmt.Errorf("Cloudflare API 错误 (HTTP %d): %s", resp.StatusCode, cfResp.Errors[0].Message)
		}
		return fmt.Errorf("Cloudflare API 请求失败，HTTP 状态码: %d", resp.StatusCode)
	}

	// 先检查 Cloudflare API 业务级错误（success=false）
	// 所有响应都包含 success 字段，先用 CFStatus 解析检查
	var cfStatus CFStatus
	if err := json.Unmarshal(respBody, &cfStatus); err != nil {
		return fmt.Errorf("解析响应状态失败: %w", err)
	}
	if !cfStatus.Success {
		if len(cfStatus.Errors) > 0 {
			return fmt.Errorf("Cloudflare API 错误: %s", cfStatus.Errors[0].Message)
		}
		return fmt.Errorf("Cloudflare API 返回失败状态")
	}

	// 反序列化完整响应体到目标结构
	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("解析响应体失败: %w", err)
		}
	}

	return nil
}

// getZoneID 根据域名获取 Cloudflare Zone ID。
// 使用 sync.Map 缓存查询结果，避免重复请求。
// 仅匹配状态为 "active" 的 Zone。
// ctx: 上下文
// domain: 主域名，例如 "example.com"
func (c *CloudflareDNSClient) getZoneID(ctx context.Context, domain string) (string, error) {
	// 先从缓存中查找
	if cached, ok := c.zoneCache.Load(domain); ok {
		return cached.(string), nil
	}

	// 调用 Cloudflare API 查询 Zone 列表
	apiURL := fmt.Sprintf("%s/zones?name=%s&status=active", c.baseURL, domain)
	var resp ZonesResp
	if err := c.doRequest(ctx, http.MethodGet, apiURL, nil, &resp); err != nil {
		return "", fmt.Errorf("查询域名 Zone 失败: %w", err)
	}

	// 查找匹配的活跃 Zone
	for _, zone := range resp.Result {
		if zone.Name == domain && zone.Status == "active" {
			// 缓存 Zone ID
			c.zoneCache.Store(domain, zone.ID)
			return zone.ID, nil
		}
	}

	return "", fmt.Errorf("未找到域名 %s 对应的 Zone", domain)
}

// compositeID 将 zoneID 和 recordID 组合为复合 ID。
// Cloudflare API 操作单条记录时需要 zoneID，但 DNSProvider 接口仅传递 recordID，
// 因此使用 "zoneID:recordID" 格式在系统中传递，确保后续操作能提取 zoneID。
func compositeID(zoneID, recordID string) string {
	return zoneID + ":" + recordID
}

// parseCompositeID 从复合 ID 中解析出 zoneID 和 recordID。
// 如果格式不正确（不包含 ":"），返回错误。
func parseCompositeID(id string) (zoneID, recordID string, err error) {
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("无效的记录 ID 格式: %s", id)
	}
	return parts[0], parts[1], nil
}

// getRecordDetail 获取单条 DNS 记录的详细信息。
// zoneID: Cloudflare Zone ID
// recordID: DNS 记录 ID
func (c *CloudflareDNSClient) getRecordDetail(ctx context.Context, zoneID, recordID string) (*CFDNSRecord, error) {
	apiURL := fmt.Sprintf("%s/zones/%s/dns_records/%s", c.baseURL, zoneID, recordID)
	var resp SingleRecordResp
	if err := c.doRequest(ctx, http.MethodGet, apiURL, nil, &resp); err != nil {
		return nil, fmt.Errorf("获取 DNS 记录详情失败: %w", err)
	}
	return &resp.Result, nil
}

// SupportsPause 返回 Cloudflare 是否支持暂停/启用操作。
// Cloudflare 不支持暂停单条 DNS 记录，始终返回 false。
func (c *CloudflareDNSClient) SupportsPause() bool {
	return false
}

// ListRecords 查询指定域名和主机记录下的所有 DNS 记录。
// 先通过域名获取 Zone ID，再查询指定子域名和记录类型的 DNS 记录列表，
// 并将 Cloudflare 格式转换为统一的 DNSRecord 格式。
// 返回的 RecordID 为 "zoneID:recordID" 复合格式。
func (c *CloudflareDNSClient) ListRecords(ctx context.Context, domain, subDomain, recordType string) ([]provider.DNSRecord, error) {
	// 获取 Zone ID
	zoneID, err := c.getZoneID(ctx, domain)
	if err != nil {
		return nil, err
	}

	// 构造完整的记录名称（子域名.域名）
	name := domain
	if subDomain != "" && subDomain != "@" {
		name = subDomain + "." + domain
	}

	// 查询 DNS 记录
	apiURL := fmt.Sprintf("%s/zones/%s/dns_records?name=%s&type=%s", c.baseURL, zoneID, name, recordType)
	var resp RecordsResp
	if err := c.doRequest(ctx, http.MethodGet, apiURL, nil, &resp); err != nil {
		return nil, fmt.Errorf("查询 DNS 记录列表失败: %w", err)
	}

	// 转换为统一的 DNSRecord 格式
	records := make([]provider.DNSRecord, 0, len(resp.Result))
	for _, r := range resp.Result {
		records = append(records, provider.DNSRecord{
			RecordID:   compositeID(zoneID, r.ID),
			DomainName: domain,
			SubDomain:  subDomain,
			Type:       r.Type,
			Value:      r.Content,
			TTL:        r.TTL,
			Status:     "ENABLE", // Cloudflare 不支持暂停，记录始终为启用状态
		})
	}

	return records, nil
}

// AddRecord 在 Cloudflare 中添加一条新的 DNS 记录。
// 先获取 Zone ID，然后 POST 创建记录，返回 "zoneID:recordID" 复合格式的记录 ID。
func (c *CloudflareDNSClient) AddRecord(ctx context.Context, domain, subDomain, recordType, value string, ttl int) (string, error) {
	// 获取 Zone ID
	zoneID, err := c.getZoneID(ctx, domain)
	if err != nil {
		return "", err
	}

	// 构造完整的记录名称
	name := domain
	if subDomain != "" && subDomain != "@" {
		name = subDomain + "." + domain
	}

	// 构造请求体
	body := map[string]interface{}{
		"type":    recordType,
		"name":    name,
		"content": value,
		"ttl":     ttl,
	}

	// POST 创建记录
	apiURL := fmt.Sprintf("%s/zones/%s/dns_records", c.baseURL, zoneID)
	var resp SingleRecordResp
	if err := c.doRequest(ctx, http.MethodPost, apiURL, body, &resp); err != nil {
		return "", fmt.Errorf("创建 DNS 记录失败: %w", err)
	}

	return compositeID(zoneID, resp.Result.ID), nil
}

// UpdateRecord 更新一条已有的 DNS 记录。
// 通过 PUT 请求更新指定记录的子域名、类型、值和 TTL。
// recordID 为 "zoneID:recordID" 复合格式。
func (c *CloudflareDNSClient) UpdateRecord(ctx context.Context, recordID, subDomain, recordType, value string, ttl int) error {
	zoneID, cfRecordID, err := parseCompositeID(recordID)
	if err != nil {
		return err
	}

	// 先获取记录详情，以获取完整的 name（需要域名部分）
	record, err := c.getRecordDetail(ctx, zoneID, cfRecordID)
	if err != nil {
		return fmt.Errorf("更新记录失败，无法获取记录详情: %w", err)
	}

	// 从现有记录的 name 中提取域名部分，构造新的 name
	// 记录的 name 格式为 "subdomain.domain.com" 或 "domain.com"
	name := record.Name
	if subDomain != "" && subDomain != "@" {
		domain := getDomainFromName(record.Name)
		name = subDomain + "." + domain
	}

	// 构造请求体
	body := map[string]interface{}{
		"type":    recordType,
		"name":    name,
		"content": value,
		"ttl":     ttl,
		"proxied": record.Proxied, // 保留原有的 proxied 状态
	}

	// PUT 更新记录
	apiURL := fmt.Sprintf("%s/zones/%s/dns_records/%s", c.baseURL, zoneID, cfRecordID)
	if err := c.doRequest(ctx, http.MethodPut, apiURL, body, nil); err != nil {
		return fmt.Errorf("更新 DNS 记录失败: %w", err)
	}

	return nil
}

// PauseRecord Cloudflare 不支持暂停操作，返回错误。
func (c *CloudflareDNSClient) PauseRecord(ctx context.Context, recordID string) error {
	return fmt.Errorf("Cloudflare 不支持暂停/启用操作")
}

// ResumeRecord Cloudflare 不支持启用操作，返回错误。
func (c *CloudflareDNSClient) ResumeRecord(ctx context.Context, recordID string) error {
	return fmt.Errorf("Cloudflare 不支持暂停/启用操作")
}

// DeleteRecord 删除一条 DNS 记录。
// recordID 为 "zoneID:recordID" 复合格式。
func (c *CloudflareDNSClient) DeleteRecord(ctx context.Context, recordID string) error {
	zoneID, cfRecordID, err := parseCompositeID(recordID)
	if err != nil {
		return err
	}

	apiURL := fmt.Sprintf("%s/zones/%s/dns_records/%s", c.baseURL, zoneID, cfRecordID)
	if err := c.doRequest(ctx, http.MethodDelete, apiURL, nil, nil); err != nil {
		return fmt.Errorf("删除 DNS 记录失败: %w", err)
	}

	return nil
}

// UpdateRecordValue 仅更新记录的 content 字段，保留其他属性（type、name、proxied、ttl）不变。
// 先获取记录详情，然后使用原有属性 + 新值进行 PUT 更新。
// recordID 为 "zoneID:recordID" 复合格式。
func (c *CloudflareDNSClient) UpdateRecordValue(ctx context.Context, recordID, newValue string) error {
	zoneID, cfRecordID, err := parseCompositeID(recordID)
	if err != nil {
		return err
	}

	// 获取记录当前详情
	record, err := c.getRecordDetail(ctx, zoneID, cfRecordID)
	if err != nil {
		return fmt.Errorf("更新记录值失败，无法获取记录详情: %w", err)
	}

	// 使用原有属性 + 新值进行更新
	body := map[string]interface{}{
		"type":    record.Type,
		"name":    record.Name,
		"content": newValue,
		"ttl":     record.TTL,
		"proxied": record.Proxied,
	}

	apiURL := fmt.Sprintf("%s/zones/%s/dns_records/%s", c.baseURL, zoneID, cfRecordID)
	if err := c.doRequest(ctx, http.MethodPut, apiURL, body, nil); err != nil {
		return fmt.Errorf("更新记录值失败: %w", err)
	}

	return nil
}

// GetRecordValue 获取指定记录的当前 content 值。
// recordID 为 "zoneID:recordID" 复合格式。
func (c *CloudflareDNSClient) GetRecordValue(ctx context.Context, recordID string) (string, error) {
	zoneID, cfRecordID, err := parseCompositeID(recordID)
	if err != nil {
		return "", err
	}

	record, err := c.getRecordDetail(ctx, zoneID, cfRecordID)
	if err != nil {
		return "", fmt.Errorf("获取记录值失败: %w", err)
	}

	return record.Content, nil
}

// getDomainFromName 从完整的 DNS 记录名称中提取主域名。
// 例如 "www.example.com" → "example.com"，"example.com" → "example.com"
func getDomainFromName(name string) string {
	parts := strings.Split(name, ".")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], ".")
	}
	return name
}

// SetProxied 设置指定 DNS 记录的 CDN 代理状态。
// 先获取记录详情，然后使用 PUT 请求更新 proxied 字段，保留其他属性不变。
// recordID 为 "zoneID:recordID" 复合格式。
func (c *CloudflareDNSClient) SetProxied(ctx context.Context, recordID string, proxied bool) error {
	zoneID, cfRecordID, err := parseCompositeID(recordID)
	if err != nil {
		return err
	}

	// 获取记录当前详情
	record, err := c.getRecordDetail(ctx, zoneID, cfRecordID)
	if err != nil {
		return fmt.Errorf("更新 CDN 代理状态失败，无法获取记录详情: %w", err)
	}

	// 使用原有属性 + 新的 proxied 值进行更新
	body := map[string]interface{}{
		"type":    record.Type,
		"name":    record.Name,
		"content": record.Content,
		"ttl":     record.TTL,
		"proxied": proxied,
	}

	apiURL := fmt.Sprintf("%s/zones/%s/dns_records/%s", c.baseURL, zoneID, cfRecordID)
	if err := c.doRequest(ctx, http.MethodPut, apiURL, body, nil); err != nil {
		return fmt.Errorf("更新 CDN 代理状态失败: %w", err)
	}

	return nil
}

// GetProxied 获取指定 DNS 记录的当前 CDN 代理状态。
// recordID 为 "zoneID:recordID" 复合格式。
func (c *CloudflareDNSClient) GetProxied(ctx context.Context, recordID string) (bool, error) {
	zoneID, cfRecordID, err := parseCompositeID(recordID)
	if err != nil {
		return false, err
	}

	record, err := c.getRecordDetail(ctx, zoneID, cfRecordID)
	if err != nil {
		return false, fmt.Errorf("获取 CDN 代理状态失败: %w", err)
	}

	return record.Proxied, nil
}

// 编译时检查：确保 CloudflareDNSClient 实现了 DNSProvider 接口
var _ provider.DNSProvider = (*CloudflareDNSClient)(nil)

// 编译时检查：确保 CloudflareDNSClient 实现了 ProxiedController 接口
var _ provider.ProxiedController = (*CloudflareDNSClient)(nil)
