package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

type AlertConfig struct {
	OfflineAfterSec  int `json:"offline_after_sec"`  // 多久没上报判定离线
	GPUTempThreshold int `json:"gpu_temp_threshold"` // 温度告警阈值
	GPUIdleThreshold int `json:"gpu_idle_threshold"` // 利用率低于该值持续 idle_after_sec 告警，0 关闭
	GPUIdleAfterSec  int `json:"gpu_idle_after_sec"`
}

type HistoryConfig struct {
	IntervalSec     int `json:"interval_sec"`     // 采样间隔
	RetentionPoints int `json:"retention_points"` // 每个序列保留的点数
}

type Config struct {
	Listen    string        `json:"listen"`
	Token     string        `json:"token"`      // agent 接入令牌
	PanelUser string        `json:"panel_user"` // 面板 Basic 认证，留空则不启用
	PanelPass string        `json:"panel_pass"`
	Alerts    AlertConfig   `json:"alerts"`
	History   HistoryConfig `json:"history"`
	ConfigPath string       `json:"-"`
}

func DefaultConfig() *Config {
	return &Config{
		Listen: ":8080",
		Alerts: AlertConfig{
			OfflineAfterSec:  30,
			GPUTempThreshold: 88,
			GPUIdleThreshold: 0,
			GPUIdleAfterSec:  600,
		},
		History: HistoryConfig{
			IntervalSec:     5,
			RetentionPoints: 720, // 5s * 720 = 1 小时
		},
	}
}

// LoadConfig 依次尝试 flag 指定路径、./config.json、/etc/gpu-panel/config.json
func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()
	candidates := []string{}
	if path != "" {
		candidates = append(candidates, path)
	}
	candidates = append(candidates, "config.json", "/etc/gpu-panel/config.json")

	loaded := false
	for _, p := range candidates {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if err := json.Unmarshal(b, cfg); err != nil {
			return nil, fmt.Errorf("解析配置 %s 失败: %w", p, err)
		}
		cfg.ConfigPath = p
		loaded = true
		break
	}
	if !loaded {
		cfg.ConfigPath = candidates[0]
	}

	applyEnv(cfg)

	if cfg.Alerts.OfflineAfterSec <= 0 {
		cfg.Alerts.OfflineAfterSec = 30
	}
	if cfg.Alerts.GPUTempThreshold <= 0 {
		cfg.Alerts.GPUTempThreshold = 88
	}
	if cfg.History.IntervalSec <= 0 {
		cfg.History.IntervalSec = 5
	}
	if cfg.History.RetentionPoints <= 0 {
		cfg.History.RetentionPoints = 720
	}

	if cfg.Token == "" {
		cfg.Token = randomToken()
		fmt.Printf("[配置] 未设置 token，已随机生成: %s\n", cfg.Token)
		fmt.Printf("[配置] 请把它填入矿机的 agent 配置文件\n")
	}
	return cfg, nil
}

func applyEnv(c *Config) {
	if v := os.Getenv("GPU_PANEL_TOKEN"); v != "" {
		c.Token = v
	}
	if v := os.Getenv("GPU_PANEL_LISTEN"); v != "" {
		c.Listen = v
	}
	if v := os.Getenv("GPU_PANEL_USER"); v != "" {
		c.PanelUser = v
	}
	if v := os.Getenv("GPU_PANEL_PASS"); v != "" {
		c.PanelPass = v
	}
}

func randomToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "change-me"
	}
	return hex.EncodeToString(b)
}

// Save 回写配置文件（仅用于初始化时落盘生成的 token）
func (c *Config) Save() error {
	if c.ConfigPath == "" {
		return errors.New("未指定配置路径")
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.ConfigPath, append(b, '\n'), 0600)
}
