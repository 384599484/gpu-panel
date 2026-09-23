package agent

import (
	"encoding/json"
	"log"
	"math/rand"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yaojiangfeng/gpu-panel/internal/shared"
)

const agentVersion = "1.0.0"

type Config struct {
	Server     string `json:"server"` // ws://<公网IP>:8080/ws/agent
	Token      string `json:"token"`
	AgentID    string `json:"agent_id"` // 留空则用主机名
	IntervalMS int    `json:"interval_ms"`
	Mock       bool   `json:"mock"` // 本地联调用：生成假 GPU 数据
	MockGPUs   int    `json:"mock_gpus"`
	NvidiaSmi  string `json:"nvidia_smi"` // nvidia-smi 路径，留空则用 PATH 中的
	Path       string `json:"-"`
}

func DefaultConfig() *Config {
	return &Config{IntervalMS: 2000, MockGPUs: 4}
}

func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()
	candidates := []string{}
	if path != "" {
		candidates = append(candidates, path)
	}
	candidates = append(candidates, "agent.json", "/etc/gpu-panel/agent.json")
	for _, p := range candidates {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if err := json.Unmarshal(b, cfg); err != nil {
			return nil, err
		}
		cfg.Path = p
		break
	}
	if cfg.Path == "" {
		cfg.Path = candidates[0]
	}
	if v := os.Getenv("GPU_AGENT_SERVER"); v != "" {
		cfg.Server = v
	}
	if v := os.Getenv("GPU_AGENT_TOKEN"); v != "" {
		cfg.Token = v
	}
	if v := os.Getenv("GPU_AGENT_ID"); v != "" {
		cfg.AgentID = v
	}
	if cfg.IntervalMS <= 0 {
		cfg.IntervalMS = 2000
	}
	return cfg, nil
}

func logf(format string, args ...interface{}) {
	log.Printf(format, args...)
}

// Run 保持长连接，断开后指数退避重连
func Run(cfg *Config) {
	if cfg.Server == "" {
		log.Fatalf("未配置 server 地址，请在 %s 中填写 server 与 token", cfg.Path)
	}
	if cfg.NvidiaSmi != "" {
		nvidiaSmiPath = cfg.NvidiaSmi
	}
	hostname, _ := os.Hostname()
	id := cfg.AgentID
	if id == "" {
		id = hostname
	}
	interval := time.Duration(cfg.IntervalMS) * time.Millisecond
	backoff := time.Second

	for {
		err := session(cfg, id, hostname, interval)
		if err != nil {
			logf("[agent] 连接失败: %v，%.0fs 后重试", err, backoff.Seconds())
		} else {
			backoff = time.Second
		}
		time.Sleep(backoff)
		backoff *= 2
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
	}
}

func session(cfg *Config, id, hostname string, interval time.Duration) error {
	sep := "?"
	if strings.Contains(cfg.Server, "?") {
		sep = "&"
	}
	u := cfg.Server + sep + "token=" + url.QueryEscape(cfg.Token)
	conn, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	logf("[agent] 已连接到 %s", cfg.Server)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		snap := CollectHost()
		snap.AgentID = id
		snap.Hostname = hostname
		snap.Version = agentVersion
		snap.TS = time.Now().UnixMilli()
		if cfg.Mock {
			snap.GPUs = mockGPUs(cfg.MockGPUs)
		} else {
			gpus, err := CollectGPUs()
			if err != nil {
				gpuUnavailable(err)
			} else {
				snap.GPUs = gpus
			}
		}
		b, err := json.Marshal(snap)
		if err != nil {
			continue
		}
		conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
			return err
		}
		select {
		case <-done:
			return nil
		case <-ticker.C:
		}
	}
}

type mockState struct {
	util  float64
	temp  float64
	power float64
	mem   float64
}

var mockCards []mockState

func mockGPUs(n int) []shared.GPU {
	if n <= 0 {
		n = 4
	}
	if len(mockCards) != n {
		mockCards = make([]mockState, n)
		for i := range mockCards {
			mockCards[i] = mockState{util: 99, temp: 62 + rand.Float64()*6, power: 290, mem: 12288}
		}
	}
	out := make([]shared.GPU, 0, n)
	for i := range mockCards {
		s := &mockCards[i]
		s.util = clamp(s.util + rand.NormFloat64()*0.6)
		s.temp = clampTemp(s.temp + rand.NormFloat64()*0.4)
		s.power = clampRange(s.power+rand.NormFloat64()*3, 150, 340)
		s.mem = clampRange(s.mem+rand.NormFloat64()*40, 10000, 14000)
		out = append(out, shared.GPU{
			Index:      i,
			Name:       "NVIDIA GeForce RTX 5070 Ti",
			UUID:       "GPU-mock-" + string(rune('a'+i)),
			Util:       s.util,
			MemUsed:    s.mem,
			MemTotal:   16384,
			Temp:       s.temp,
			Power:      s.power,
			PowerLimit: 300,
			Fan:        65 + rand.Float64()*10,
			ClockSM:    2400 + rand.Float64()*50,
			ClockMem:   14000,
			PCIeGen:    5,
			PCIeWidth:  16,
		})
	}
	return out
}

func clamp(v float64) float64     { return clampRange(v, 90, 100) }
func clampTemp(v float64) float64 { return clampRange(v, 50, 85) }

func clampRange(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
