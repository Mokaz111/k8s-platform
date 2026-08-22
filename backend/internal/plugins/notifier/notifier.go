package notifier

import (
	"context"
)

// MVP_TODO: INotifier 通知插件接口（设计文档 Section 8.4）
// 用于多渠道消息推送：邮件、短信、Webhook、钉钉/企微/飞书机器人等
type INotifier interface {
	ChannelType() string

	Name() string

	Init(ctx context.Context, config map[string]interface{}) error

	Send(ctx context.Context, target []string, msg *NotificationMessage) error

	BatchSend(ctx context.Context, messages []*NotificationMessage) error

	ValidateTarget(ctx context.Context, target string) error
}

type NotificationMessage struct {
	Title    string
	Content  string
	HTMLBody string
	Priority int
	Category string
	TraceID  string
	Template string
	Vars     map[string]interface{}
	Attachments []*NotificationAttachment
}

type NotificationAttachment struct {
	Filename string
	Content  []byte
	MIMEType string
	URL      string
}
