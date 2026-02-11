// Package tencentcloud 实现腾讯云 DNSPod API 3.0 对接。
// 本文件实现腾讯云 DNS 客户端，提供 DNS 记录的查询、添加、更新、删除、暂停、启用操作。
// 由于腾讯云 API 操作单条记录时需要同时传递 Domain 和 RecordId，
// 本实现使用 "domain:recordId" 复合 ID 格式在系统中传递，确保后续操作能提取 Domain。
package tencentcloud

import (
	"bytes"
	"context"
	"dns-health-monitor/internal/provider"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// 腾讯云 DNSPod API 端点
	defaultEndpoint = "https://dnspod.tencentcloudapi.com"
	// API 版本号
	apiVersion = "2021-03-23"
	// 服务名称（用于签名）
	serviceName = "dnspod"
)

// TencentCloudDNSClient 腾讯云 DNS 客户端
// 实现 provider.DNSProvider 接口，通过腾讯云 DNSPod API 3.0 管理 DNS 解析记录。
type TencentCloudDNSClient struct {
	// secretID 腾讯云 SecretId
	secretID string
	// secretKey 腾讯云 SecretKey
	secretKey string
	// endpoint API 端点地址，默认为腾讯云官方端点，可自定义用于测试
	endpoint string
	// httpClient HTTP 客户端
	httpClient *http.Client
}

// --- 请求结构体 ---

// describeRecordListReq 查询记录列表请求
// 注意：腾讯云 DescribeRecordList API 使用小写 d 的 Subdomain
type describeRecordListReq struct {
	Domain     string `json:"Domain"`
	Subdomain  string `json:"Subdomain,omitempty"`
	RecordType string `json:"RecordType,omitempty"`
	RecordLine string `json:"RecordLine,omitempty"`
}

// createRecordReq 新增记录请求
// 注意：腾讯云 CreateRecord API 使用大写 D 的 SubDomain
type createRecordReq struct {
	Domain     string `json:"Domain"`
	SubDomain  string `json:"SubDomain"`
	RecordType string `json:"RecordType"`
	RecordLine string `json:"RecordLine"`
	Value      string `json:"Value"`
	TTL        int    `json:"TTL"`
}

// modifyRecordReq 修改记录请求
type modifyRecordReq struct {
	Domain     string `json:"Domain"`
	SubDomain  string `json:"SubDomain"`
	RecordType string `json:"RecordType"`
	RecordLine string `json:"RecordLine"`
	Value      string `json:"Value"`
	RecordId   uint64 `json:"RecordId"`
	TTL        int    `json:"TTL"`
}

// deleteRecordReq 删除记录请求
type deleteRecordReq struct {
	Domain   string `json:"Domain"`
	RecordId uint64 `json:"RecordId"`
}

// describeRecordReq 查询单条记录详情请求
type describeRecordReq struct {
	Domain   string `json:"Domain"`
	RecordId uint64 `json:"RecordId"`
}

// modifyRecordStatusReq 修改记录状态请求
type modifyRecordStatusReq struct {
	Domain   string `json:"Domain"`
	RecordId uint64 `json:"RecordId"`
	Status   string `json:"Status"` // "ENABLE" 或 "DISABLE"
}

// --- 响应结构体 ---

// tcError 腾讯云 API 错误信息
type tcError struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
}

// tcRecord 腾讯云 DNS 记录结构（DescribeRecordList 返回）
type tcRecord struct {
	RecordId uint64 `json:"RecordId"`
	Name     string `json:"Name"`
	Type     string `json:"Type"`
	Value    string `json:"Value"`
	TTL      int    `json:"TTL"`
	Status   string `json:"Status"`
	Line     string `json:"Line"`
}

// describeRecordListResp 查询记录列表响应
type describeRecordListResp struct {
	Response struct {
		RecordCountInfo struct {
			TotalCount int `json:"TotalCount"`
		} `json:"RecordCountInfo"`
		RecordList []tcRecord `json:"RecordList"`
		Error      *tcError   `json:"Error,omitempty"`
	} `json:"Response"`
}

// createRecordResp 新增记录响应
type createRecordResp struct {
	Response struct {
		RecordId uint64   `json:"RecordId"`
		Error    *tcError `json:"Error,omitempty"`
	} `json:"Response"`
}

// describeRecordResp 查询单条记录详情响应
type describeRecordResp struct {
	Response struct {
		RecordInfo struct {
			Id         uint64 `json:"Id"`
			SubDomain  string `json:"SubDomain"`
			RecordType string `json:"RecordType"`
			Value      string `json:"Value"`
			TTL        int    `json:"TTL"`
			RecordLine string `json:"RecordLine"`
		} `json:"RecordInfo"`
		Error *tcError `json:"Error,omitempty"`
	} `json:"Response"`
}

// tcStatusResp 通用状态响应（用于修改、删除等无特殊返回值的操作）
type tcStatusResp struct {
	Response struct {
		Error *tcError `json:"Error,omitempty"`
	} `json:"Response"`
}

// --- 复合 ID 工具函数 ---

// compositeID 将 domain 和 recordId 组合为复合 ID。
// 格式为 "domain:recordId"，例如 "example.com:12345"。
// 腾讯云 API 操作单条记录时需要同时传递 Domain 和 RecordId，
// 但 DNSProvider 接口仅传递 recordID，因此使用复合 ID 传递 domain 信息。
func compositeID(domain string, recordId uint64) string {
	return domain + ":" + strconv.FormatUint(recordId, 10)
}

// parseCompositeID 从复合 ID 中解析出 domain 和 recordId。
// 如果格式不正确，返回错误。
func parseCompositeID(id string) (domain string, recordId uint64, err error) {
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", 0, fmt.Errorf("无效的腾讯云记录 ID 格式: %s（期望格式: domain:recordId）", id)
	}
	recordId, err = strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("无效的记录 ID 数值: %s", parts[1])
	}
	return parts[0], recordId, nil
}

// --- 客户端构造与接口实现 ---

// NewTencentCloudDNSClient 创建腾讯云 DNS 客户端实例。
// secretID: 腾讯云 SecretId
// secretKey: 腾讯云 SecretKey
// 可选参数 endpoint: 自定义 API 端点地址（用于测试），不传则使用默认端点
func NewTencentCloudDNSClient(secretID, secretKey string, endpoint ...string) *TencentCloudDNSClient {
	ep := defaultEndpoint
	if len(endpoint) > 0 && endpoint[0] != "" {
		ep = endpoint[0]
	}

	return &TencentCloudDNSClient{
		secretID:  secretID,
		secretKey: secretKey,
		endpoint:  ep,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SupportsPause 返回腾讯云是否支持暂停/启用操作。
// 腾讯云 DNSPod 支持 ModifyRecordStatus API，因此返回 true。
func (c *TencentCloudDNSClient) SupportsPause() bool {
	return true
}

// ListRecords 查询指定域名和主机记录下的所有 DNS 记录。
// 使用腾讯云 DescribeRecordList API 获取记录列表。
// 返回的 RecordID 为 "domain:recordId" 复合格式。
func (c *TencentCloudDNSClient) ListRecords(ctx context.Context, domain, subDomain, recordType string) ([]provider.DNSRecord, error) {
	req := describeRecordListReq{
		Domain:     domain,
		Subdomain:  subDomain,
		RecordType: recordType,
		RecordLine: "默认",
	}

	var resp describeRecordListResp
	if err := c.doRequest(ctx, "DescribeRecordList", req, &resp); err != nil {
		return nil, fmt.Errorf("查询 DNS 记录列表失败: %w", err)
	}

	if resp.Response.Error != nil {
		// 如果是"没有记录"的错误，返回空列表而非错误
		if resp.Response.Error.Code == "ResourceNotFound.NoDataOfRecord" {
			return []provider.DNSRecord{}, nil
		}
		return nil, fmt.Errorf("腾讯云 API 错误 [%s]: %s", resp.Response.Error.Code, resp.Response.Error.Message)
	}

	records := make([]provider.DNSRecord, 0, len(resp.Response.RecordList))
	for _, r := range resp.Response.RecordList {
		records = append(records, provider.DNSRecord{
			RecordID:   compositeID(domain, r.RecordId),
			DomainName: domain,
			SubDomain:  r.Name,
			Type:       r.Type,
			Value:      r.Value,
			TTL:        r.TTL,
			Status:     convertStatus(r.Status),
		})
	}

	return records, nil
}

// AddRecord 添加一条新的 DNS 记录。
// 使用腾讯云 CreateRecord API 创建记录。
// 返回 "domain:recordId" 复合格式的记录 ID。
func (c *TencentCloudDNSClient) AddRecord(ctx context.Context, domain, subDomain, recordType, value string, ttl int) (string, error) {
	req := createRecordReq{
		Domain:     domain,
		SubDomain:  subDomain,
		RecordType: recordType,
		RecordLine: "默认",
		Value:      value,
		TTL:        ttl,
	}

	var resp createRecordResp
	if err := c.doRequest(ctx, "CreateRecord", req, &resp); err != nil {
		return "", fmt.Errorf("添加 DNS 记录失败: %w", err)
	}

	if resp.Response.Error != nil {
		return "", fmt.Errorf("腾讯云 API 错误 [%s]: %s", resp.Response.Error.Code, resp.Response.Error.Message)
	}

	if resp.Response.RecordId == 0 {
		return "", fmt.Errorf("添加记录成功但返回的 RecordId 为空")
	}

	return compositeID(domain, resp.Response.RecordId), nil
}

// UpdateRecord 更新一条已有的 DNS 记录。
// 使用腾讯云 ModifyRecord API 更新记录。
// recordID 为 "domain:recordId" 复合格式。
func (c *TencentCloudDNSClient) UpdateRecord(ctx context.Context, recordID, subDomain, recordType, value string, ttl int) error {
	domain, recID, err := parseCompositeID(recordID)
	if err != nil {
		return err
	}

	// 获取记录详情以得到 RecordLine
	info, err := c.getRecordInfo(ctx, domain, recID)
	if err != nil {
		return fmt.Errorf("更新记录失败，无法获取记录详情: %w", err)
	}

	req := modifyRecordReq{
		Domain:     domain,
		SubDomain:  subDomain,
		RecordType: recordType,
		RecordLine: info.RecordLine,
		Value:      value,
		RecordId:   recID,
		TTL:        ttl,
	}

	var resp tcStatusResp
	if err := c.doRequest(ctx, "ModifyRecord", req, &resp); err != nil {
		return fmt.Errorf("更新 DNS 记录失败: %w", err)
	}

	if resp.Response.Error != nil {
		return fmt.Errorf("腾讯云 API 错误 [%s]: %s", resp.Response.Error.Code, resp.Response.Error.Message)
	}

	return nil
}

// PauseRecord 暂停一条 DNS 记录。
// 使用腾讯云 ModifyRecordStatus API 将记录状态设置为 DISABLE。
// recordID 为 "domain:recordId" 复合格式。
func (c *TencentCloudDNSClient) PauseRecord(ctx context.Context, recordID string) error {
	return c.setRecordStatus(ctx, recordID, "DISABLE")
}

// ResumeRecord 启用一条已暂停的 DNS 记录。
// 使用腾讯云 ModifyRecordStatus API 将记录状态设置为 ENABLE。
// recordID 为 "domain:recordId" 复合格式。
func (c *TencentCloudDNSClient) ResumeRecord(ctx context.Context, recordID string) error {
	return c.setRecordStatus(ctx, recordID, "ENABLE")
}

// DeleteRecord 删除一条 DNS 记录。
// 使用腾讯云 DeleteRecord API 删除记录。
// recordID 为 "domain:recordId" 复合格式。
func (c *TencentCloudDNSClient) DeleteRecord(ctx context.Context, recordID string) error {
	domain, recID, err := parseCompositeID(recordID)
	if err != nil {
		return err
	}

	req := deleteRecordReq{
		Domain:   domain,
		RecordId: recID,
	}

	var resp tcStatusResp
	if err := c.doRequest(ctx, "DeleteRecord", req, &resp); err != nil {
		return fmt.Errorf("删除 DNS 记录失败: %w", err)
	}

	if resp.Response.Error != nil {
		return fmt.Errorf("腾讯云 API 错误 [%s]: %s", resp.Response.Error.Code, resp.Response.Error.Message)
	}

	return nil
}

// UpdateRecordValue 更新解析记录的值（用于故障转移切换）。
// 先获取记录详情，然后使用原有属性 + 新值进行更新。
// recordID 为 "domain:recordId" 复合格式。
func (c *TencentCloudDNSClient) UpdateRecordValue(ctx context.Context, recordID, newValue string) error {
	domain, recID, err := parseCompositeID(recordID)
	if err != nil {
		return err
	}

	info, err := c.getRecordInfo(ctx, domain, recID)
	if err != nil {
		return fmt.Errorf("更新记录值失败，无法获取记录详情: %w", err)
	}

	req := modifyRecordReq{
		Domain:     domain,
		SubDomain:  info.SubDomain,
		RecordType: info.RecordType,
		RecordLine: info.RecordLine,
		Value:      newValue,
		RecordId:   recID,
		TTL:        info.TTL,
	}

	var resp tcStatusResp
	if err := c.doRequest(ctx, "ModifyRecord", req, &resp); err != nil {
		return fmt.Errorf("更新记录值失败: %w", err)
	}

	if resp.Response.Error != nil {
		return fmt.Errorf("腾讯云 API 错误 [%s]: %s", resp.Response.Error.Code, resp.Response.Error.Message)
	}

	return nil
}

// GetRecordValue 获取解析记录的当前值。
// recordID 为 "domain:recordId" 复合格式。
func (c *TencentCloudDNSClient) GetRecordValue(ctx context.Context, recordID string) (string, error) {
	domain, recID, err := parseCompositeID(recordID)
	if err != nil {
		return "", err
	}

	info, err := c.getRecordInfo(ctx, domain, recID)
	if err != nil {
		return "", fmt.Errorf("获取记录值失败: %w", err)
	}

	return info.Value, nil
}

// --- 内部辅助方法 ---

// recordDetail 记录详情（内部使用，DescribeRecord API 返回的关键字段）
type recordDetail struct {
	SubDomain  string
	RecordType string
	Value      string
	TTL        int
	RecordLine string
}

// getRecordInfo 获取单条 DNS 记录的详细信息。
// 调用腾讯云 DescribeRecord API。
// domain: 主域名
// recordId: 记录 ID（数值）
func (c *TencentCloudDNSClient) getRecordInfo(ctx context.Context, domain string, recordId uint64) (*recordDetail, error) {
	req := describeRecordReq{
		Domain:   domain,
		RecordId: recordId,
	}

	var resp describeRecordResp
	if err := c.doRequest(ctx, "DescribeRecord", req, &resp); err != nil {
		return nil, fmt.Errorf("查询记录详情失败: %w", err)
	}

	if resp.Response.Error != nil {
		return nil, fmt.Errorf("腾讯云 API 错误 [%s]: %s", resp.Response.Error.Code, resp.Response.Error.Message)
	}

	return &recordDetail{
		SubDomain:  resp.Response.RecordInfo.SubDomain,
		RecordType: resp.Response.RecordInfo.RecordType,
		Value:      resp.Response.RecordInfo.Value,
		TTL:        resp.Response.RecordInfo.TTL,
		RecordLine: resp.Response.RecordInfo.RecordLine,
	}, nil
}

// setRecordStatus 设置 DNS 记录状态（暂停或启用）。
// 使用腾讯云 ModifyRecordStatus API。
// recordID: "domain:recordId" 复合格式
// status: "ENABLE" 或 "DISABLE"
func (c *TencentCloudDNSClient) setRecordStatus(ctx context.Context, recordID, status string) error {
	domain, recID, err := parseCompositeID(recordID)
	if err != nil {
		return err
	}

	req := modifyRecordStatusReq{
		Domain:   domain,
		RecordId: recID,
		Status:   status,
	}

	var resp tcStatusResp
	if err := c.doRequest(ctx, "ModifyRecordStatus", req, &resp); err != nil {
		return fmt.Errorf("设置记录状态失败: %w", err)
	}

	if resp.Response.Error != nil {
		return fmt.Errorf("腾讯云 API 错误 [%s]: %s", resp.Response.Error.Code, resp.Response.Error.Message)
	}

	return nil
}

// doRequest 执行腾讯云 API 请求。
// 对请求进行 TC3-HMAC-SHA256 签名，发送 HTTP POST 请求并解析响应。
// ctx: 上下文，用于控制请求超时和取消
// action: API 操作名称，如 "DescribeRecordList"、"CreateRecord"
// data: 请求体结构，会被 JSON 序列化
// result: 响应体反序列化目标
func (c *TencentCloudDNSClient) doRequest(ctx context.Context, action string, data interface{}, result interface{}) error {
	jsonStr, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化请求体失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewBuffer(jsonStr))
	if err != nil {
		return fmt.Errorf("创建 HTTP 请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-TC-Version", apiVersion)

	// TC3-HMAC-SHA256 签名
	sign(c.secretID, c.secretKey, req, action, string(jsonStr), serviceName)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("发送 HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应体失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// 尝试解析腾讯云错误响应
		var errResp tcStatusResp
		if jsonErr := json.Unmarshal(body, &errResp); jsonErr == nil && errResp.Response.Error != nil {
			return fmt.Errorf("腾讯云 API 错误 (HTTP %d) [%s]: %s",
				resp.StatusCode, errResp.Response.Error.Code, errResp.Response.Error.Message)
		}
		return fmt.Errorf("腾讯云 API 请求失败，HTTP 状态码: %d", resp.StatusCode)
	}

	if result != nil {
		if err := json.Unmarshal(body, result); err != nil {
			return fmt.Errorf("解析响应体失败: %w", err)
		}
	}

	return nil
}

// convertStatus 将腾讯云记录状态转换为系统内部统一状态。
func convertStatus(tcStatus string) string {
	switch tcStatus {
	case "ENABLE":
		return "ENABLE"
	case "DISABLE":
		return "DISABLE"
	default:
		return "ENABLE"
	}
}

// 编译时检查：确保 TencentCloudDNSClient 实现了 DNSProvider 接口
var _ provider.DNSProvider = (*TencentCloudDNSClient)(nil)
