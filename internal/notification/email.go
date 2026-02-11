// Package notification 通知模块 - 邮件渠道实现
package notification

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// EmailChannel 邮件通知渠道，实现 NotificationChannel 接口
// 通过 SMTP 协议发送邮件通知
type EmailChannel struct{}

// NewEmailChannel 创建邮件渠道实例
func NewEmailChannel() *EmailChannel {
	return &EmailChannel{}
}

// Type 返回渠道类型标识
func (e *EmailChannel) Type() string {
	return "email"
}

// Send 发送邮件通知
// 1. 断言 config 为 *EmailChannelConfig
// 2. 调用 RenderEmailHTML 渲染邮件模板
// 3. 构建邮件头部（From, To, Subject, MIME-Version, Content-Type）
// 4. 通过 net/smtp 发送邮件
// 5. 返回具体的错误信息（连接超时、认证失败等）
func (e *EmailChannel) Send(ctx context.Context, event NotificationEvent, config ChannelConfig) error {
	// 1. 断言 config 类型
	emailConfig, ok := config.(*EmailChannelConfig)
	if !ok {
		return fmt.Errorf("邮件渠道配置类型错误，期望 *EmailChannelConfig，实际为 %T", config)
	}

	// 验证 SMTP 配置
	if err := ValidateSMTPConfig(emailConfig); err != nil {
		return fmt.Errorf("SMTP 配置验证失败: %w", err)
	}

	// 2. 渲染邮件 HTML 内容
	htmlContent, err := RenderEmailHTML(event)
	if err != nil {
		return fmt.Errorf("渲染邮件模板失败: %w", err)
	}

	// 3. 构建邮件主题
	subject := buildEmailSubject(event)

	// 4. 构建邮件头部和正文
	message := buildEmailMessage(emailConfig.FromAddress, emailConfig.ToAddress, subject, htmlContent)

	// 5. 通过 SMTP 发送邮件
	if err := sendSMTPEmail(ctx, emailConfig, message); err != nil {
		return err
	}

	return nil
}

// ValidateSMTPConfig 验证 SMTP 配置，确保所有必填字段均已填写
// 当任何必填字段（Host、Port、Username、Password、FromAddress、ToAddress）为空时返回错误
func ValidateSMTPConfig(config *EmailChannelConfig) error {
	var missingFields []string

	if strings.TrimSpace(config.Host) == "" {
		missingFields = append(missingFields, "服务器地址(Host)")
	}
	if config.Port <= 0 {
		missingFields = append(missingFields, "端口(Port)")
	}
	if strings.TrimSpace(config.Username) == "" {
		missingFields = append(missingFields, "用户名(Username)")
	}
	if strings.TrimSpace(config.Password) == "" {
		missingFields = append(missingFields, "密码(Password)")
	}
	if strings.TrimSpace(config.FromAddress) == "" {
		missingFields = append(missingFields, "发件人地址(FromAddress)")
	}
	if strings.TrimSpace(config.ToAddress) == "" {
		missingFields = append(missingFields, "收件人地址(ToAddress)")
	}

	if len(missingFields) > 0 {
		return fmt.Errorf("以下必填字段为空: %s", strings.Join(missingFields, ", "))
	}

	return nil
}

// TestSMTPConnection 测试 SMTP 连接并发送测试邮件
// 验证 SMTP 服务器连接和认证是否正常，并发送一封测试邮件
func TestSMTPConnection(config *EmailChannelConfig) error {
	// 先验证配置
	if err := ValidateSMTPConfig(config); err != nil {
		return fmt.Errorf("SMTP 配置验证失败: %w", err)
	}

	// 构建测试邮件内容，复用主模板样式
	subject := "[DNS 健康监控] SMTP 连接测试"
	testTime := time.Now().Format("2006-01-02 15:04:05")
	body := `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>DNS 健康监控 - SMTP 连接测试</title>
</head>
<body style="margin:0;padding:0;background-color:#F4F6F9;font-family:'Microsoft YaHei','Helvetica Neue',Arial,sans-serif;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background-color:#F4F6F9;">
<tr>
<td align="center" style="padding:20px 0;">
<table role="presentation" width="600" cellpadding="0" cellspacing="0" border="0" style="background-color:#FFFFFF;border-radius:8px;overflow:hidden;box-shadow:0 2px 8px rgba(0,0,0,0.1);">
<tr>
<td style="background-color:#27AE60;padding:40px 30px;text-align:center;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
<tr><td style="color:#FFFFFF;font-size:14px;letter-spacing:2px;padding-bottom:10px;text-align:center;">DNS 健康监控</td></tr>
<tr><td style="color:#FFFFFF;font-size:24px;font-weight:bold;padding-top:5px;text-align:center;">SMTP 连接测试成功</td></tr>
</table>
</td>
</tr>
<tr>
<td style="padding:25px 30px 15px 30px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background-color:#EAFAF1;border-left:4px solid #27AE60;border-radius:4px;">
<tr><td style="padding:15px 20px;">
<span style="color:#666666;font-size:13px;">测试结果</span><br>
<span style="color:#333333;font-size:18px;font-weight:bold;">✅ 邮件服务器配置正确</span>
</td></tr>
</table>
</td>
</tr>
<tr>
<td style="padding:10px 30px 25px 30px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="border:1px solid #E8E8E8;border-radius:4px;">
<tr><td style="background-color:#27AE60;color:#FFFFFF;font-size:14px;font-weight:bold;padding:12px 20px;">连接详情</td></tr>
<tr><td style="padding:0;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
<tr>
<td width="40%" style="padding:12px 20px;border-bottom:1px solid #F0F0F0;color:#888888;font-size:13px;">SMTP 服务器</td>
<td style="padding:12px 20px;border-bottom:1px solid #F0F0F0;color:#333333;font-size:13px;font-weight:bold;">` + config.Host + `</td>
</tr>
<tr>
<td width="40%" style="padding:12px 20px;border-bottom:1px solid #F0F0F0;color:#888888;font-size:13px;">端口</td>
<td style="padding:12px 20px;border-bottom:1px solid #F0F0F0;color:#333333;font-size:13px;">` + fmt.Sprintf("%d", config.Port) + `</td>
</tr>
<tr>
<td width="40%" style="padding:12px 20px;border-bottom:1px solid #F0F0F0;color:#888888;font-size:13px;">发件人</td>
<td style="padding:12px 20px;border-bottom:1px solid #F0F0F0;color:#333333;font-size:13px;">` + config.FromAddress + `</td>
</tr>
<tr>
<td width="40%" style="padding:12px 20px;color:#888888;font-size:13px;">测试时间</td>
<td style="padding:12px 20px;color:#333333;font-size:13px;">` + testTime + `</td>
</tr>
</table>
</td></tr>
</table>
</td>
</tr>
<tr>
<td style="background-color:#F8F9FA;padding:20px 30px;border-top:1px solid #E8E8E8;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
<tr><td style="color:#999999;font-size:12px;text-align:center;">
此邮件由 DNS 健康监控 系统自动发送，请勿直接回复。<br>
发送时间：` + testTime + `
</td></tr>
</table>
</td>
</tr>
</table>
</td>
</tr>
</table>
</body>
</html>`

	message := buildEmailMessage(config.FromAddress, config.ToAddress, subject, body)

	// 使用带超时的上下文发送测试邮件
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := sendSMTPEmail(ctx, config, message); err != nil {
		return err
	}

	return nil
}

// buildEmailSubject 根据事件类型构建邮件主题
func buildEmailSubject(event NotificationEvent) string {
	taskName := event.Domain
	if event.SubDomain != "" && event.SubDomain != "@" {
		taskName = event.SubDomain + "." + event.Domain
	}

	switch event.Type {
	case "failover":
		return fmt.Sprintf("[DNS 健康监控] 故障转移告警 - %s", taskName)
	case "recovery":
		return fmt.Sprintf("[DNS 健康监控] 服务恢复通知 - %s", taskName)
	case "consecutive_fail":
		return fmt.Sprintf("[DNS 健康监控] 连续失败告警 - %s", taskName)
	default:
		return fmt.Sprintf("[DNS 健康监控] 通知 - %s", taskName)
	}
}

// buildEmailMessage 构建完整的邮件消息（包含头部和正文）
func buildEmailMessage(from, to, subject, htmlBody string) []byte {
	// 构建邮件头部
	headers := make(map[string]string)
	headers["From"] = from
	headers["To"] = to
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=UTF-8"

	// 组装邮件消息
	var msg strings.Builder
	for key, value := range headers {
		msg.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
	}
	msg.WriteString("\r\n")
	msg.WriteString(htmlBody)

	return []byte(msg.String())
}

// sendSMTPEmail 通过 SMTP 发送邮件，支持上下文超时控制
// 自动根据端口选择连接方式：
// - 端口 465：使用隐式 TLS（SSL 直连）
// - 其他端口（如 25、587）：使用明文连接 + 可选 STARTTLS
func sendSMTPEmail(ctx context.Context, config *EmailChannelConfig, message []byte) error {
	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)

	// 使用带超时的拨号连接
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
	}

	var conn net.Conn
	var err error

	if config.Port == 465 {
		// 端口 465：使用隐式 TLS（SSL 直连）
		tlsConfig := &tls.Config{
			ServerName: config.Host,
		}
		tlsDialer := &tls.Dialer{
			NetDialer: dialer,
			Config:    tlsConfig,
		}

		connCh := make(chan net.Conn, 1)
		errCh := make(chan error, 1)
		go func() {
			c, e := tlsDialer.DialContext(ctx, "tcp", addr)
			if e != nil {
				errCh <- e
				return
			}
			connCh <- c
		}()

		select {
		case <-ctx.Done():
			return fmt.Errorf("SMTP 连接超时: %w", ctx.Err())
		case err = <-errCh:
			return wrapSMTPError(err)
		case conn = <-connCh:
		}
	} else {
		// 其他端口：使用明文 TCP 连接
		connCh := make(chan net.Conn, 1)
		errCh := make(chan error, 1)
		go func() {
			c, e := dialer.DialContext(ctx, "tcp", addr)
			if e != nil {
				errCh <- e
				return
			}
			connCh <- c
		}()

		select {
		case <-ctx.Done():
			return fmt.Errorf("SMTP 连接超时: %w", ctx.Err())
		case err = <-errCh:
			return wrapSMTPError(err)
		case conn = <-connCh:
		}
	}

	// 创建 SMTP 客户端
	client, err := smtp.NewClient(conn, config.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("创建 SMTP 客户端失败: %w", err)
	}
	defer client.Close()

	// 非 465 端口时尝试 STARTTLS 升级
	if config.Port != 465 {
		if ok, _ := client.Extension("STARTTLS"); ok {
			tlsConfig := &tls.Config{
				ServerName: config.Host,
			}
			if err := client.StartTLS(tlsConfig); err != nil {
				return fmt.Errorf("STARTTLS 升级失败: %w", err)
			}
		}
	}

	// SMTP 认证
	// 使用自定义 loginAuth 替代 PlainAuth，兼容 QQ 邮箱、163 邮箱等国内邮件服务商
	auth := loginAuth(config.Username, config.Password)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP 认证失败（请检查用户名和密码）: %w", err)
	}

	// 设置发件人
	if err := client.Mail(config.FromAddress); err != nil {
		return fmt.Errorf("设置发件人失败: %w", err)
	}

	// 设置收件人（支持多个收件人，以逗号分隔）
	recipients := strings.Split(config.ToAddress, ",")
	for _, recipient := range recipients {
		recipient = strings.TrimSpace(recipient)
		if recipient == "" {
			continue
		}
		if err := client.Rcpt(recipient); err != nil {
			return fmt.Errorf("设置收件人 %s 失败: %w", recipient, err)
		}
	}

	// 写入邮件内容
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("打开邮件数据写入失败: %w", err)
	}

	if _, err := writer.Write(message); err != nil {
		return fmt.Errorf("写入邮件内容失败: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("关闭邮件数据写入失败: %w", err)
	}

	// 发送 QUIT 命令
	if err := client.Quit(); err != nil {
		// QUIT 失败不影响邮件发送，仅记录
		return nil
	}

	return nil
}

// loginAuth 实现 SMTP LOGIN 认证方式
// QQ 邮箱、163 邮箱等国内邮件服务商通常使用 LOGIN 认证
// Go 标准库的 PlainAuth 在某些场景下不兼容这些服务商
type loginAuthData struct {
	username string
	password string
}

func loginAuth(username, password string) smtp.Auth {
	return &loginAuthData{username: username, password: password}
}

func (a *loginAuthData) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", []byte{}, nil
}

func (a *loginAuthData) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		switch string(fromServer) {
		case "Username:":
			return []byte(a.username), nil
		case "Password:":
			return []byte(a.password), nil
		default:
			return nil, fmt.Errorf("未知的 SMTP LOGIN 认证提示: %s", string(fromServer))
		}
	}
	return nil, nil
}

// wrapSMTPError 将底层网络错误包装为用户友好的错误信息
func wrapSMTPError(err error) error {
	if err == nil {
		return nil
	}

	errMsg := err.Error()

	// 连接超时
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return fmt.Errorf("SMTP 连接超时，请检查服务器地址和端口是否正确: %w", err)
	}

	// 连接被拒绝
	if strings.Contains(errMsg, "connection refused") {
		return fmt.Errorf("SMTP 连接被拒绝，请检查服务器地址和端口是否正确: %w", err)
	}

	// DNS 解析失败
	if strings.Contains(errMsg, "no such host") {
		return fmt.Errorf("SMTP 服务器地址无法解析，请检查服务器地址是否正确: %w", err)
	}

	// 其他网络错误
	return fmt.Errorf("SMTP 连接失败: %w", err)
}
