// Package notification 通知模块 - 邮件 HTML 模板渲染
package notification

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"dns-health-monitor/internal/model"
)

// templateData 模板渲染所需的数据结构
type templateData struct {
	// 系统信息
	SystemName string // 系统名称
	EventTitle string // 事件类型标题

	// 颜色主题
	PrimaryColor string // 主色调
	LightColor   string // 浅色背景

	// 事件基础信息
	TaskName   string // 任务名称（域名+子域名）
	OccurredAt string // 事件发生时间

	// 事件类型
	EventType model.EventType

	// 故障转移相关
	OriginalValue string // 原始解析值
	BackupValue   string // 切换后解析值
	HealthStatus  string // 当前健康状态

	// 恢复相关
	RecoveredValue string // 恢复后的解析值
	DownDuration   string // 故障持续时长

	// 连续失败相关
	FailCount     int      // 连续失败次数
	FailedIPs     []string // 失败的 IP 列表
	ProbeProtocol string   // 探测协议
	ProbePort     int      // 探测端口
}

// eventTypeConfig 事件类型对应的配置
type eventTypeConfig struct {
	Title        string // 事件标题
	PrimaryColor string // 主色调
	LightColor   string // 浅色背景
}

// 事件类型配置映射
var eventTypeConfigs = map[model.EventType]eventTypeConfig{
	model.EventTypeFailover: {
		Title:        "故障转移告警",
		PrimaryColor: "#E74C3C",
		LightColor:   "#FDEDEC",
	},
	model.EventTypeRecovery: {
		Title:        "服务恢复通知",
		PrimaryColor: "#27AE60",
		LightColor:   "#EAFAF1",
	},
	model.EventTypeConsecutiveFail: {
		Title:        "连续失败告警",
		PrimaryColor: "#F39C12",
		LightColor:   "#FEF9E7",
	},
}

// emailTemplate 邮件 HTML 模板
// 使用 HTML 表格布局，确保在主流邮件客户端（Gmail、Outlook、QQ邮箱）中正确渲染
var emailTemplate = template.Must(template.New("email").Parse(emailTemplateHTML))

// emailTemplateHTML 邮件 HTML 模板字符串
// 采用 HTML 表格布局，兼容主流邮件客户端
const emailTemplateHTML = `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{{.SystemName}} - {{.EventTitle}}</title>
</head>
<body style="margin:0;padding:0;background-color:#F4F6F9;font-family:'Microsoft YaHei','Helvetica Neue',Arial,sans-serif;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background-color:#F4F6F9;">
<tr>
<td align="center" style="padding:20px 0;">

<!-- 主容器 -->
<table role="presentation" width="600" cellpadding="0" cellspacing="0" border="0" style="background-color:#FFFFFF;border-radius:8px;overflow:hidden;box-shadow:0 2px 8px rgba(0,0,0,0.1);">

<!-- 封面区域 -->
<tr>
<td style="background-color:{{.PrimaryColor}};padding:40px 30px;text-align:center;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
<tr>
<td style="color:#FFFFFF;font-size:14px;letter-spacing:2px;padding-bottom:10px;text-align:center;">
{{.SystemName}}
</td>
</tr>
<tr>
<td style="color:#FFFFFF;font-size:24px;font-weight:bold;padding-top:5px;text-align:center;">
{{.EventTitle}}
</td>
</tr>
</table>
</td>
</tr>

<!-- 任务名称区域 -->
<tr>
<td style="padding:25px 30px 15px 30px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background-color:{{.LightColor}};border-left:4px solid {{.PrimaryColor}};border-radius:4px;">
<tr>
<td style="padding:15px 20px;">
<span style="color:#666666;font-size:13px;">任务名称</span><br>
<span style="color:#333333;font-size:18px;font-weight:bold;">{{.TaskName}}</span>
</td>
</tr>
</table>
</td>
</tr>

<!-- 事件详情区域 -->
<tr>
<td style="padding:10px 30px 25px 30px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="border:1px solid #E8E8E8;border-radius:4px;">
<tr>
<td style="background-color:{{.PrimaryColor}};color:#FFFFFF;font-size:14px;font-weight:bold;padding:12px 20px;">
事件详情
</td>
</tr>
{{if eq (printf "%s" .EventType) "failover"}}
<!-- 故障转移详情 -->
<tr>
<td style="padding:0;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
<tr>
<td width="40%" style="padding:12px 20px;border-bottom:1px solid #F0F0F0;color:#888888;font-size:13px;">故障转移时间</td>
<td style="padding:12px 20px;border-bottom:1px solid #F0F0F0;color:#333333;font-size:13px;font-weight:bold;">{{.OccurredAt}}</td>
</tr>
<tr>
<td width="40%" style="padding:12px 20px;border-bottom:1px solid #F0F0F0;color:#888888;font-size:13px;">原始解析值</td>
<td style="padding:12px 20px;border-bottom:1px solid #F0F0F0;color:#333333;font-size:13px;">{{.OriginalValue}}</td>
</tr>
<tr>
<td width="40%" style="padding:12px 20px;border-bottom:1px solid #F0F0F0;color:#888888;font-size:13px;">切换后解析值</td>
<td style="padding:12px 20px;border-bottom:1px solid #F0F0F0;color:#333333;font-size:13px;">{{.BackupValue}}</td>
</tr>
<tr>
<td width="40%" style="padding:12px 20px;color:#888888;font-size:13px;">当前健康状态</td>
<td style="padding:12px 20px;color:{{.PrimaryColor}};font-size:13px;font-weight:bold;">{{.HealthStatus}}</td>
</tr>
</table>
</td>
</tr>
{{else if eq (printf "%s" .EventType) "recovery"}}
<!-- 恢复详情 -->
<tr>
<td style="padding:0;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
<tr>
<td width="40%" style="padding:12px 20px;border-bottom:1px solid #F0F0F0;color:#888888;font-size:13px;">恢复时间</td>
<td style="padding:12px 20px;border-bottom:1px solid #F0F0F0;color:#333333;font-size:13px;font-weight:bold;">{{.OccurredAt}}</td>
</tr>
<tr>
<td width="40%" style="padding:12px 20px;border-bottom:1px solid #F0F0F0;color:#888888;font-size:13px;">恢复后解析值</td>
<td style="padding:12px 20px;border-bottom:1px solid #F0F0F0;color:#333333;font-size:13px;">{{.RecoveredValue}}</td>
</tr>
<tr>
<td width="40%" style="padding:12px 20px;color:#888888;font-size:13px;">故障持续时长</td>
<td style="padding:12px 20px;color:#333333;font-size:13px;font-weight:bold;">{{.DownDuration}}</td>
</tr>
</table>
</td>
</tr>
{{else if eq (printf "%s" .EventType) "consecutive_fail"}}
<!-- 连续失败告警详情 -->
<tr>
<td style="padding:0;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
<tr>
<td width="40%" style="padding:12px 20px;border-bottom:1px solid #F0F0F0;color:#888888;font-size:13px;">告警时间</td>
<td style="padding:12px 20px;border-bottom:1px solid #F0F0F0;color:#333333;font-size:13px;font-weight:bold;">{{.OccurredAt}}</td>
</tr>
<tr>
<td width="40%" style="padding:12px 20px;border-bottom:1px solid #F0F0F0;color:#888888;font-size:13px;">连续失败次数</td>
<td style="padding:12px 20px;color:{{.PrimaryColor}};font-size:13px;font-weight:bold;">{{.FailCount}} 次</td>
</tr>
<tr>
<td width="40%" style="padding:12px 20px;border-bottom:1px solid #F0F0F0;color:#888888;font-size:13px;vertical-align:top;">失败 IP 列表</td>
<td style="padding:12px 20px;border-bottom:1px solid #F0F0F0;color:#333333;font-size:13px;">{{range $i, $ip := .FailedIPs}}{{if $i}}<br>{{end}}{{$ip}}{{end}}</td>
</tr>
<tr>
<td width="40%" style="padding:12px 20px;border-bottom:1px solid #F0F0F0;color:#888888;font-size:13px;">探测协议</td>
<td style="padding:12px 20px;border-bottom:1px solid #F0F0F0;color:#333333;font-size:13px;">{{.ProbeProtocol}}</td>
</tr>
<tr>
<td width="40%" style="padding:12px 20px;color:#888888;font-size:13px;">探测端口</td>
<td style="padding:12px 20px;color:#333333;font-size:13px;">{{.ProbePort}}</td>
</tr>
</table>
</td>
</tr>
{{end}}
</table>
</td>
</tr>

<!-- 页脚区域 -->
<tr>
<td style="background-color:#F8F9FA;padding:20px 30px;border-top:1px solid #E8E8E8;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
<tr>
<td style="color:#999999;font-size:12px;text-align:center;">
此邮件由 {{.SystemName}} 自动发送，请勿直接回复。<br>
发送时间：{{.OccurredAt}}
</td>
</tr>
</table>
</td>
</tr>

</table>
<!-- /主容器 -->

</td>
</tr>
</table>
</body>
</html>`

// buildTaskName 根据域名和子域名构建任务名称
func buildTaskName(domain, subDomain string) string {
	if subDomain == "" || subDomain == "@" {
		return domain
	}
	return subDomain + "." + domain
}

// formatDuration 将 Duration 格式化为中文可读字符串
func formatDuration(d interface{ String() string }) string {
	s := d.String()
	// 替换英文单位为中文
	s = strings.Replace(s, "h", "小时", 1)
	s = strings.Replace(s, "m", "分", 1)
	s = strings.Replace(s, "s", "秒", 1)
	return s
}

// RenderEmailHTML 渲染邮件 HTML 内容
// 根据事件类型选择对应的颜色主题和内容模板，生成完整的 HTML 邮件
func RenderEmailHTML(event NotificationEvent) (string, error) {
	// 获取事件类型配置
	config, ok := eventTypeConfigs[event.Type]
	if !ok {
		return "", fmt.Errorf("未知的事件类型: %s", event.Type)
	}

	// 构建模板数据
	data := templateData{
		SystemName:   "DNS 健康监控",
		EventTitle:   config.Title,
		PrimaryColor: config.PrimaryColor,
		LightColor:   config.LightColor,
		TaskName:     buildTaskName(event.Domain, event.SubDomain),
		OccurredAt:   event.OccurredAt.Format("2006-01-02 15:04:05"),
		EventType:    event.Type,
	}

	// 根据事件类型填充特定字段
	switch event.Type {
	case model.EventTypeFailover:
		data.OriginalValue = event.OriginalValue
		data.BackupValue = event.BackupValue
		data.HealthStatus = event.HealthStatus
	case model.EventTypeRecovery:
		data.RecoveredValue = event.RecoveredValue
		data.DownDuration = formatDuration(event.DownDuration)
	case model.EventTypeConsecutiveFail:
		data.FailCount = event.FailCount
		data.FailedIPs = event.FailedIPs
		data.ProbeProtocol = event.ProbeProtocol
		data.ProbePort = event.ProbePort
	}

	// 渲染模板
	var buf bytes.Buffer
	if err := emailTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("渲染邮件模板失败: %w", err)
	}

	return buf.String(), nil
}
