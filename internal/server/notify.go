package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yaojiangfeng/gpu-panel/internal/shared"
)

type wecomPayload struct {
	MsgType  string `json:"msgtype"`
	Markdown struct {
		Content string `json:"content"`
	} `json:"markdown"`
}

type wecomResp struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

// Notifier 把告警状态变化推送到企业微信群机器人
type Notifier struct {
	cfg *Config
	cli *http.Client

	mu       sync.Mutex
	prev     map[string]shared.Alert // 上一轮仍在持续的告警
	sent     map[string]int64        // 告警 key -> 上次推送时间
	lastSend time.Time
}

func NewNotifier(cfg *Config) *Notifier {
	timeout := time.Duration(cfg.Notify.TimeoutSec) * time.Second
	return &Notifier{
		cfg:  cfg,
		cli:  &http.Client{Timeout: timeout},
		prev: map[string]shared.Alert{},
		sent: map[string]int64{},
	}
}

func (n *Notifier) Enabled() bool {
	return n.cfg.Notify.Enabled && strings.TrimSpace(n.cfg.Notify.Webhook) != ""
}

func alertKey(a shared.Alert) string {
	return fmt.Sprintf("%s|%s|%d", a.Kind, a.AgentID, a.GPU)
}

// Observe 对比上一轮告警，新增的推送告警，消失的推送恢复
func (n *Notifier) Observe(alerts []shared.Alert) {
	if !n.Enabled() {
		return
	}
	now := time.Now().UnixMilli()
	cur := map[string]shared.Alert{}
	for _, a := range alerts {
		cur[alertKey(a)] = a
	}

	// 机器已离线时，GPU 类告警随之消失，这不算恢复，不推送恢复消息
	offlineAgents := map[string]bool{}
	for _, a := range alerts {
		if a.Kind == "offline" {
			offlineAgents[a.AgentID] = true
		}
	}

	n.mu.Lock()
	var fired, recovered []shared.Alert
	for k, a := range cur {
		if _, ok := n.prev[k]; ok {
			continue // 已在上轮推送过，持续中不再重复
		}
		if !n.levelAllowed(a.Level) {
			continue
		}
		if last, ok := n.sent[k]; ok && now-last < int64(n.cfg.Notify.CooldownSec)*1000 {
			continue // 冷却期内不刷屏
		}
		n.sent[k] = now
		fired = append(fired, a)
	}
	for k, a := range n.prev {
		if _, ok := cur[k]; ok {
			continue
		}
		if n.cfg.Notify.Recovery {
			if _, ok := n.sent[k]; ok {
				if !offlineAgents[a.AgentID] {
					recovered = append(recovered, a)
				}
			}
		}
		delete(n.sent, k)
	}
	n.prev = cur
	n.mu.Unlock()

	sort.Slice(fired, func(i, j int) bool { return alertKey(fired[i]) < alertKey(fired[j]) })
	sort.Slice(recovered, func(i, j int) bool { return alertKey(recovered[i]) < alertKey(recovered[j]) })

	for _, a := range fired {
		go n.post(n.render("矿机告警", a.Level, a))
	}
	for _, a := range recovered {
		go n.post(n.render("告警已恢复", "info", a))
	}
}

func (n *Notifier) levelAllowed(level string) bool {
	for _, l := range n.cfg.Notify.Levels {
		if strings.EqualFold(l, level) {
			return true
		}
	}
	return false
}

func (n *Notifier) render(title, level string, a shared.Alert) string {
	color := "comment"
	switch level {
	case "crit":
		color = "red"
	case "warn":
		color = "warning"
	case "info":
		color = "info"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n", title)
	fmt.Fprintf(&b, "> 矿机：<font color=\"%s\">%s</font>\n", color, a.Hostname)
	if a.IP != "" {
		fmt.Fprintf(&b, "> 地址：%s\n", a.IP)
	}
	if a.GPU >= 0 {
		fmt.Fprintf(&b, "> 显卡：GPU%d\n", a.GPU)
	}
	fmt.Fprintf(&b, "> 类型：%s\n", kindText(a.Kind))
	fmt.Fprintf(&b, "> 详情：%s\n", a.Message)
	fmt.Fprintf(&b, "> 时间：%s\n", time.Now().Format("2006-01-02 15:04:05"))
	return b.String()
}

func kindText(kind string) string {
	switch kind {
	case "offline":
		return "矿机离线"
	case "gpu_temp":
		return "显卡高温"
	case "gpu_drop":
		return "显卡掉线"
	case "gpu_idle":
		return "疑似停挖"
	default:
		return kind
	}
}

func (n *Notifier) post(content string) {
	var p wecomPayload
	p.MsgType = "markdown"
	p.Markdown.Content = content
	b, err := json.Marshal(p)
	if err != nil {
		log.Printf("[推送] 序列化失败: %v", err)
		return
	}

	n.throttle()
	resp, err := n.cli.Post(n.cfg.Notify.Webhook, "application/json", bytes.NewReader(b))
	if err != nil {
		log.Printf("[推送] 发送失败: %v", err)
		return
	}
	defer resp.Body.Close()

	var r wecomResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		log.Printf("[推送] 响应无法解析（HTTP %d）", resp.StatusCode)
		return
	}
	if r.ErrCode != 0 {
		log.Printf("[推送] 企业微信返回错误 errcode=%d errmsg=%s", r.ErrCode, r.ErrMsg)
		return
	}
	log.Printf("[推送] 已发送:\n%s", content)
}

// throttle 群机器人限流 20 条/分钟，这里串行间隔 1 秒保守处理
func (n *Notifier) throttle() {
	n.mu.Lock()
	wait := time.Until(n.lastSend.Add(time.Second))
	n.lastSend = time.Now()
	n.mu.Unlock()
	if wait > 0 {
		time.Sleep(wait)
	}
}

// Test 发送一条测试消息，用于验证 webhook 配置
func (n *Notifier) Test() error {
	if !n.Enabled() {
		return fmt.Errorf("推送未启用：请配置 notify.enabled 与 notify.wecom_webhook")
	}
	content := fmt.Sprintf("## 矿机监控 · 推送自检\n> 状态：<font color=\"info\">正常</font>\n> 时间：%s\n",
		time.Now().Format("2006-01-02 15:04:05"))
	n.post(content)
	return nil
}

// Status 返回推送启用状态，供启动日志使用
func (n *Notifier) Status() string {
	if !n.Enabled() {
		return "未启用（未配置 webhook）"
	}
	return fmt.Sprintf("已启用，级别 %s，冷却 %ds", strings.Join(n.cfg.Notify.Levels, "/"), n.cfg.Notify.CooldownSec)
}
