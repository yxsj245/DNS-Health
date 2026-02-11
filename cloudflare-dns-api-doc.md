# Cloudflare DNS 解析实现文档

本文档基于 `ddns-go` 项目中 Cloudflare DNS 解析的实现，提取出核心 API 调用逻辑，方便你独立实现自己的 Cloudflare DNS 解析工具。

---

## 1. 认证方式

Cloudflare API 使用 **Bearer Token** 认证，所有请求需要在 Header 中携带：

```
Authorization: Bearer <你的API Token>
Content-Type: application/json
```

API Token 在 Cloudflare 控制台创建：https://dash.cloudflare.com/profile/api-tokens

创建 Token 时需要赋予以下权限：
- Zone - DNS - Edit（编辑 DNS 记录）
- Zone - Zone - Read（读取 Zone 信息）

---

## 2. API 基础地址

```
https://api.cloudflare.com/client/v4/zones
```

---

## 3. 核心流程

整个 DNS 解析更新流程分为 3 步：

```
获取 Zone ID → 查询现有 DNS 记录 → 创建或更新记录
```

### 3.1 获取 Zone ID

根据根域名查询对应的 Zone ID。Zone 是 Cloudflare 中域名的管理单元。

**请求：**
```
GET https://api.cloudflare.com/client/v4/zones?name={根域名}&status=active&per_page=50
```

**参数说明：**
| 参数 | 说明 | 示例 |
|------|------|------|
| name | 根域名（不含子域名） | `example.com` |
| status | Zone 状态，一般为 `active` | `active` |
| per_page | 每页返回数量 | `50` |

**响应结构：**
```json
{
  "success": true,
  "messages": [],
  "result": [
    {
      "id": "zone_id_string",
      "name": "example.com",
      "status": "active",
      "paused": false
    }
  ]
}
```

`result[0].id` 就是后续操作需要的 **Zone ID**。

---

### 3.2 查询现有 DNS 记录

用 Zone ID 查询指定子域名的 DNS 记录，判断是需要新增还是更新。

**请求：**
```
GET https://api.cloudflare.com/client/v4/zones/{zone_id}/dns_records?type={记录类型}&name={完整域名}&per_page=50
```

**参数说明：**
| 参数 | 说明 | 示例 |
|------|------|------|
| type | 记录类型 | `A`（IPv4）或 `AAAA`（IPv6） |
| name | 完整域名（Punycode 编码） | `www.example.com` |
| per_page | 每页返回数量 | `50` |
| comment | 可选，按备注筛选 | `ddns` |

**响应结构：**
```json
{
  "success": true,
  "messages": [],
  "result": [
    {
      "id": "record_id_string",
      "name": "www.example.com",
      "type": "A",
      "content": "1.2.3.4",
      "proxied": false,
      "ttl": 1,
      "comment": ""
    }
  ]
}
```

- `result` 为空数组 → 需要**新增**记录
- `result` 有数据 → 需要**更新**记录

---

### 3.3 新增 DNS 记录

**请求：**
```
POST https://api.cloudflare.com/client/v4/zones/{zone_id}/dns_records
```

**请求体（JSON）：**
```json
{
  "type": "A",
  "name": "www.example.com",
  "content": "1.2.3.4",
  "proxied": false,
  "ttl": 1,
  "comment": ""
}
```

**字段说明：**
| 字段 | 类型 | 说明 |
|------|------|------|
| type | string | `A`（IPv4）或 `AAAA`（IPv6） |
| name | string | 完整域名，建议使用 Punycode 编码 |
| content | string | IP 地址 |
| proxied | bool | 是否开启 Cloudflare CDN 代理，`false` 为仅 DNS |
| ttl | int | TTL 值，`1` 表示自动 |
| comment | string | 备注，可选 |

**响应：**
```json
{
  "success": true,
  "messages": []
}
```

---

### 3.4 更新 DNS 记录

当记录已存在且 IP 发生变化时，使用 PUT 更新。

**请求：**
```
PUT https://api.cloudflare.com/client/v4/zones/{zone_id}/dns_records/{record_id}
```

**请求体（JSON）：**
```json
{
  "id": "record_id_string",
  "type": "A",
  "name": "www.example.com",
  "content": "5.6.7.8",
  "proxied": false,
  "ttl": 1,
  "comment": ""
}
```

字段与新增相同，额外需要 `id` 字段（从查询结果中获取）。

**响应：**
```json
{
  "success": true,
  "messages": []
}
```

---

## 4. 完整流程伪代码

```
function updateDNS(apiToken, domain, subDomain, recordType, newIP):
    fullDomain = subDomain ? subDomain + "." + domain : domain
    
    // 第一步：获取 Zone ID
    zones = GET /zones?name={domain}&status=active&per_page=50
    if zones.result 为空:
        报错 "未找到域名"
        return
    zoneID = zones.result[0].id
    
    // 第二步：查询现有记录
    records = GET /zones/{zoneID}/dns_records?type={recordType}&name={fullDomain}&per_page=50
    
    if records.result 为空:
        // 第三步A：新增记录
        POST /zones/{zoneID}/dns_records
        body: { type, name: fullDomain, content: newIP, proxied: false, ttl: 1 }
    else:
        // 第三步B：更新记录（遍历所有匹配记录）
        for record in records.result:
            if record.content == newIP:
                跳过（IP 未变化）
            PUT /zones/{zoneID}/dns_records/{record.id}
            body: { id: record.id, type, name: fullDomain, content: newIP, proxied: record.proxied, ttl: 1 }
```

---

## 5. Go 语言最小实现示例

```go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/url"
)

const zonesAPI = "https://api.cloudflare.com/client/v4/zones"

// 通用响应状态
type CFStatus struct {
    Success  bool     `json:"success"`
    Messages []string `json:"messages"`
}

// Zone 查询响应
type ZonesResp struct {
    CFStatus
    Result []struct {
        ID   string `json:"id"`
        Name string `json:"name"`
    } `json:"result"`
}

// DNS 记录
type DNSRecord struct {
    ID      string `json:"id"`
    Name    string `json:"name"`
    Type    string `json:"type"`
    Content string `json:"content"`
    Proxied bool   `json:"proxied"`
    TTL     int    `json:"ttl"`
    Comment string `json:"comment"`
}

// DNS 记录查询响应
type RecordsResp struct {
    CFStatus
    Result []DNSRecord `json:"result"`
}

// 发送请求
func cfRequest(method, url, token string, data interface{}, result interface{}) error {
    var body io.Reader
    if data != nil {
        jsonBytes, _ := json.Marshal(data)
        body = bytes.NewBuffer(jsonBytes)
    }
    req, err := http.NewRequest(method, url, body)
    if err != nil {
        return err
    }
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Content-Type", "application/json")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    respBody, _ := io.ReadAll(resp.Body)
    if resp.StatusCode >= 300 {
        return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
    }
    return json.Unmarshal(respBody, result)
}

// UpdateDNS 更新或创建 DNS 记录
// token: Cloudflare API Token
// domain: 根域名，如 example.com
// fullDomain: 完整域名，如 www.example.com
// recordType: "A" 或 "AAAA"
// ip: 新的 IP 地址
func UpdateDNS(token, domain, fullDomain, recordType, ip string) error {
    // 1. 获取 Zone ID
    params := url.Values{}
    params.Set("name", domain)
    params.Set("status", "active")
    params.Set("per_page", "50")

    var zones ZonesResp
    if err := cfRequest("GET", zonesAPI+"?"+params.Encode(), token, nil, &zones); err != nil {
        return fmt.Errorf("查询 Zone 失败: %w", err)
    }
    if !zones.Success || len(zones.Result) == 0 {
        return fmt.Errorf("未找到域名 %s 对应的 Zone", domain)
    }
    zoneID := zones.Result[0].ID

    // 2. 查询现有记录
    rParams := url.Values{}
    rParams.Set("type", recordType)
    rParams.Set("name", fullDomain)
    rParams.Set("per_page", "50")

    var records RecordsResp
    if err := cfRequest("GET", fmt.Sprintf("%s/%s/dns_records?%s", zonesAPI, zoneID, rParams.Encode()), token, nil, &records); err != nil {
        return fmt.Errorf("查询 DNS 记录失败: %w", err)
    }

    if len(records.Result) == 0 {
        // 3A. 新增记录
        record := DNSRecord{
            Type:    recordType,
            Name:    fullDomain,
            Content: ip,
            Proxied: false,
            TTL:     1, // 1 = 自动
        }
        var status CFStatus
        if err := cfRequest("POST", fmt.Sprintf("%s/%s/dns_records", zonesAPI, zoneID), token, &record, &status); err != nil {
            return fmt.Errorf("新增记录失败: %w", err)
        }
        if !status.Success {
            return fmt.Errorf("新增记录失败: %v", status.Messages)
        }
        fmt.Printf("新增 %s -> %s 成功\n", fullDomain, ip)
    } else {
        // 3B. 更新记录
        for _, rec := range records.Result {
            if rec.Content == ip {
                fmt.Printf("IP %s 未变化，跳过 %s\n", ip, fullDomain)
                continue
            }
            rec.Content = ip
            rec.TTL = 1
            var status CFStatus
            if err := cfRequest("PUT", fmt.Sprintf("%s/%s/dns_records/%s", zonesAPI, zoneID, rec.ID), token, &rec, &status); err != nil {
                return fmt.Errorf("更新记录失败: %w", err)
            }
            if !status.Success {
                return fmt.Errorf("更新记录失败: %v", status.Messages)
            }
            fmt.Printf("更新 %s -> %s 成功\n", fullDomain, ip)
        }
    }
    return nil
}

func main() {
    token := "你的_cloudflare_api_token"
    err := UpdateDNS(token, "example.com", "www.example.com", "A", "1.2.3.4")
    if err != nil {
        fmt.Println("错误:", err)
    }
}
```

---

## 6. 关键注意事项

| 项目 | 说明 |
|------|------|
| 域名编码 | Cloudflare API 中 `name` 字段需要使用 Punycode 编码（对中文域名等国际化域名）。Go 中可用 `golang.org/x/net/idna` 包的 `ToASCII()` 方法 |
| TTL | 值为 `1` 表示自动（Auto），Cloudflare 会自行决定合适的 TTL |
| proxied | `true` 表示流量经过 Cloudflare CDN 代理（橙色云），`false` 表示仅 DNS 解析（灰色云）。DDNS 场景一般用 `false` |
| 记录类型 | `A` 对应 IPv4，`AAAA` 对应 IPv6 |
| 错误处理 | 所有响应都有 `success` 布尔字段和 `messages` 数组，需要检查 `success` 是否为 `true` |
| HTTP 状态码 | 300 及以上视为异常 |
| comment | 可选字段，用于给 DNS 记录添加备注 |

---

## 7. Cloudflare API 参考

- [Cloudflare API 文档](https://developers.cloudflare.com/api/)
- [DNS Records API](https://developers.cloudflare.com/api/resources/dns/subresources/records/)
- [Zones API](https://developers.cloudflare.com/api/resources/zones/)
- [创建 API Token](https://dash.cloudflare.com/profile/api-tokens)
