// Package provider 定义了云服务商 DNS 操作的统一接口。
// 所有云服务商（如阿里云、腾讯云等）的 DNS 客户端都应实现 DNSProvider 接口，
// 以便系统能够以统一的方式管理不同服务商的 DNS 解析记录。
package provider

import "context"

// DNSRecord 统一 DNS 记录结构
// 用于在系统内部表示一条 DNS 解析记录，屏蔽不同云服务商的数据格式差异。
type DNSRecord struct {
	// RecordID 记录唯一标识，由云服务商分配
	RecordID string

	// DomainName 主域名，例如 "example.com"
	DomainName string

	// SubDomain 主机记录，例如 "www"，"@" 表示根域名
	SubDomain string

	// Type 记录类型，"A" 或 "AAAA"
	Type string

	// Value IP 地址，A 记录为 IPv4，AAAA 记录为 IPv6
	Value string

	// TTL 生存时间（秒）
	TTL int

	// Status 记录状态，"ENABLE" 表示启用，"DISABLE" 表示暂停
	Status string
}

// DNSProvider 云服务商统一接口
// 定义了对 DNS 解析记录的所有操作，包括查询、添加、更新、暂停、启用和删除。
// 不同云服务商通过实现此接口来对接系统。
type DNSProvider interface {
	// SupportsPause 返回该服务商是否支持暂停/启用操作。
	// 支持暂停的服务商（如阿里云）在 IP 不可达时暂停记录而非删除；
	// 不支持暂停的服务商则通过删除记录并缓存来实现类似效果。
	SupportsPause() bool

	// ListRecords 查询指定域名和主机记录下的所有 DNS 记录。
	// domain: 主域名，例如 "example.com"
	// subDomain: 主机记录，例如 "www"
	// recordType: 记录类型，"A" 或 "AAAA"
	// 返回匹配的 DNS 记录列表，或错误信息。
	ListRecords(ctx context.Context, domain, subDomain, recordType string) ([]DNSRecord, error)

	// AddRecord 添加一条新的 DNS 记录。
	// domain: 主域名
	// subDomain: 主机记录
	// recordType: 记录类型，"A" 或 "AAAA"
	// value: IP 地址
	// ttl: 生存时间（秒）
	// 返回新创建记录的 ID，或错误信息。
	AddRecord(ctx context.Context, domain, subDomain, recordType, value string, ttl int) (string, error)

	// UpdateRecord 更新一条已有的 DNS 记录。
	// recordID: 要更新的记录 ID
	// subDomain: 主机记录
	// recordType: 记录类型
	// value: 新的 IP 地址
	// ttl: 新的生存时间（秒）
	// 返回错误信息（如果有）。
	UpdateRecord(ctx context.Context, recordID, subDomain, recordType, value string, ttl int) error

	// PauseRecord 暂停一条 DNS 记录。
	// 仅在 SupportsPause() 返回 true 的服务商上可用。
	// recordID: 要暂停的记录 ID
	// 返回错误信息（如果有）。
	PauseRecord(ctx context.Context, recordID string) error

	// ResumeRecord 启用一条已暂停的 DNS 记录。
	// 仅在 SupportsPause() 返回 true 的服务商上可用。
	// recordID: 要启用的记录 ID
	// 返回错误信息（如果有）。
	ResumeRecord(ctx context.Context, recordID string) error

	// DeleteRecord 删除一条 DNS 记录。
	// recordID: 要删除的记录 ID
	// 返回错误信息（如果有）。
	DeleteRecord(ctx context.Context, recordID string) error

	// UpdateRecordValue 更新解析记录的值（用于故障转移切换）。
	// 仅更新记录的值（IP地址或域名），不修改其他属性。
	// recordID: 要更新的记录 ID
	// newValue: 新的 IP 地址或域名
	// 返回错误信息（如果有）。
	UpdateRecordValue(ctx context.Context, recordID, newValue string) error

	// GetRecordValue 获取解析记录的当前值。
	// 用于查询记录当前指向的 IP 地址或域名。
	// recordID: 要查询的记录 ID
	// 返回记录的当前值（IP地址或域名），或错误信息。
	GetRecordValue(ctx context.Context, recordID string) (string, error)
}
