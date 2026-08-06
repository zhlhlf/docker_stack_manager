package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"docker_stack_manager/internal/models"
)

// DingTalk sends cleanup notifications to a DingTalk custom robot webhook.
type DingTalk struct {
	Webhook string
	Client  *http.Client
}

// NewDingTalk creates a notifier. Empty webhook disables sending.
func NewDingTalk(webhook string) *DingTalk {
	return &DingTalk{
		Webhook: strings.TrimSpace(webhook),
		Client:  &http.Client{Timeout: 8 * time.Second},
	}
}

// Enabled reports whether webhook is configured.
func (d *DingTalk) Enabled() bool {
	return d != nil && d.Webhook != ""
}

// NotifyClean pushes a markdown message when services were cleaned.
func (d *DingTalk) NotifyClean(cleaned []models.ServiceInfo, source string) error {
	if !d.Enabled() || len(cleaned) == 0 {
		return nil
	}
	if source == "" {
		source = "manual"
	}

	var b strings.Builder
	b.WriteString("### Docker Stack Manager 清理通知\n\n")
	b.WriteString(fmt.Sprintf("- **来源**: %s\n", source))
	b.WriteString(fmt.Sprintf("- **时间**: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("- **清理数量**: %d\n\n", len(cleaned)))
	b.WriteString("#### 服务列表\n\n")
	for i, svc := range cleaned {
		reason := svc.Violation.Reason
		if reason == "no_stack" {
			reason = "无 Stack 归属"
		} else if reason == "port_not_allowed" {
			reason = "端口不在白名单"
		}
		stack := svc.Stack
		if stack == "" {
			stack = "未归属"
		}
		ports := strings.Join(svc.PublishedPorts, ", ")
		if ports == "" {
			ports = "-"
		}
		b.WriteString(fmt.Sprintf("%d. `%s`  \n", i+1, svc.Name))
		b.WriteString(fmt.Sprintf("   - Stack: %s  \n", stack))
		b.WriteString(fmt.Sprintf("   - 原因: %s  \n", reason))
		b.WriteString(fmt.Sprintf("   - 端口: %s\n", ports))
	}

	payload := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": "Stack Manager 清理通知",
			"text":  b.String(),
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, d.Webhook, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := d.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("dingtalk webhook status %d", resp.StatusCode)
	}
	return nil
}