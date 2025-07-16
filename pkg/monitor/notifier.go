package monitor

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
	"time"
)

// EmailNotifier 邮件通知器
type EmailNotifier struct {
	host     string
	port     int
	username string
	password string
	from     string
	to       []string
	ssl      bool
}

// NewEmailNotifier 创建邮件通知器
func NewEmailNotifier(host string, port int, username, password, from string, to []string, ssl bool) *EmailNotifier {
	return &EmailNotifier{
		host:     host,
		port:     port,
		username: username,
		password: password,
		from:     from,
		to:       to,
		ssl:      ssl,
	}
}

// Name 获取通知器名称
func (n *EmailNotifier) Name() string {
	return "email"
}

// Send 发送告警
func (n *EmailNotifier) Send(ctx context.Context, alert Alert) error {
	// 构造邮件主题
	subject := fmt.Sprintf("[%s] %s", alert.Level, alert.Name)

	// 构造邮件正文
	var body strings.Builder
	body.WriteString(fmt.Sprintf("告警名称: %s\n", alert.Name))
	body.WriteString(fmt.Sprintf("告警级别: %s\n", alert.Level))
	body.WriteString(fmt.Sprintf("告警消息: %s\n", alert.Message))
	body.WriteString(fmt.Sprintf("开始时间: %s\n", alert.StartsAt.Format(time.RFC3339)))

	if !alert.EndsAt.IsZero() {
		body.WriteString(fmt.Sprintf("结束时间: %s\n", alert.EndsAt.Format(time.RFC3339)))
	}

	if len(alert.Labels) > 0 {
		body.WriteString("\n标签:\n")
		for k, v := range alert.Labels {
			body.WriteString(fmt.Sprintf("  %s: %s\n", k, v))
		}
	}

	if len(alert.Annotations) > 0 {
		body.WriteString("\n注释:\n")
		for k, v := range alert.Annotations {
			body.WriteString(fmt.Sprintf("  %s: %s\n", k, v))
		}
	}

	if alert.GeneratorURL != "" {
		body.WriteString(fmt.Sprintf("\n详情: %s\n", alert.GeneratorURL))
	}

	// 构造邮件头
	var message strings.Builder
	message.WriteString(fmt.Sprintf("From: %s\r\n", n.from))
	message.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(n.to, ",")))
	message.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	message.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	message.WriteString("\r\n")
	message.WriteString(body.String())

	// 发送邮件
	auth := smtp.PlainAuth("", n.username, n.password, n.host)
	err := smtp.SendMail(
		fmt.Sprintf("%s:%d", n.host, n.port),
		auth,
		n.from,
		n.to,
		[]byte(message.String()),
	)

	if err != nil {
		return fmt.Errorf("发送邮件失败: %v", err)
	}

	return nil
}

// WebhookNotifier Webhook通知器
type WebhookNotifier struct {
	url     string
	timeout time.Duration
}

// NewWebhookNotifier 创建Webhook通知器
func NewWebhookNotifier(url string, timeout time.Duration) *WebhookNotifier {
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	return &WebhookNotifier{
		url:     url,
		timeout: timeout,
	}
}

// Name 获取通知器名称
func (n *WebhookNotifier) Name() string {
	return "webhook"
}

// Send 发送告警
func (n *WebhookNotifier) Send(ctx context.Context, alert Alert) error {
	// 创建HTTP客户端
	// 将告警信息序列化为JSON
	// 发送POST请求到Webhook URL
	// 这里简化实现，实际应该使用HTTP客户端发送请求
	fmt.Printf("发送告警到Webhook: %s\n", n.url)
	fmt.Printf("告警名称: %s\n", alert.Name)
	fmt.Printf("告警级别: %s\n", alert.Level)
	fmt.Printf("告警消息: %s\n", alert.Message)

	return nil
}
