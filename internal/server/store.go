package server

import (
	"net"
	"sort"
	"sync"
	"time"

	"github.com/yaojiangfeng/gpu-panel/internal/shared"
)

type point struct {
	T int64   `json:"t"`
	V float64 `json:"v"`
}

// ring 固定容量的环形缓冲，避免历史数据无限增长
type ring struct {
	buf []point
	idx int
	n   int
}

func newRing(cap int) *ring {
	if cap < 2 {
		cap = 2
	}
	return &ring{buf: make([]point, cap)}
}

func (r *ring) push(t int64, v float64) {
	r.buf[r.idx] = point{T: t, V: v}
	r.idx = (r.idx + 1) % len(r.buf)
	if r.n < len(r.buf) {
		r.n++
	}
}

func (r *ring) slice(limit int) []point {
	if limit <= 0 || limit > r.n {
		limit = r.n
	}
	out := make([]point, 0, limit)
	start := (r.idx - limit + len(r.buf)) % len(r.buf)
	for i := 0; i < limit; i++ {
		out = append(out, r.buf[(start+i)%len(r.buf)])
	}
	return out
}

type gpuSeries struct {
	util  *ring
	temp  *ring
	mem   *ring
	power *ring
}

type machineState struct {
	view      shared.MachineView
	maxGPU    int
	idleSince map[int]int64
	since     map[string]int64 // 告警 key -> 首次触发时间
	hostCPU   *ring
	hostMem   *ring
	gpus      map[int]*gpuSeries
}

// Store 保存所有机器的最新状态与历史序列
type Store struct {
	mu         sync.RWMutex
	cfg        *Config
	machines   map[string]*machineState
	lastSample time.Time
}

func NewStore(cfg *Config) *Store {
	return &Store{cfg: cfg, machines: map[string]*machineState{}}
}

func (s *Store) Upsert(snap shared.Snapshot, ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.machines[snap.AgentID]
	if !ok || m.view.Hostname != snap.Hostname {
		m = &machineState{
			idleSince: map[int]int64{},
			since:     map[string]int64{},
			hostCPU:   newRing(s.cfg.History.RetentionPoints),
			hostMem:   newRing(s.cfg.History.RetentionPoints),
			gpus:      map[int]*gpuSeries{},
		}
		s.machines[snap.AgentID] = m
	}
	m.view = shared.MachineView{
		AgentID:  snap.AgentID,
		Hostname: snap.Hostname,
		IP:       ip,
		Version:  snap.Version,
		Online:   true,
		LastSeen: time.Now().UnixMilli(),
		TS:       snap.TS,
		Uptime:   snap.Uptime,
		CPU:      snap.CPU,
		Mem:      snap.Mem,
		Disks:    snap.Disks,
		Net:      snap.Net,
		GPUs:     snap.GPUs,
	}
	if len(snap.GPUs) > m.maxGPU {
		m.maxGPU = len(snap.GPUs)
	}
	for _, g := range snap.GPUs {
		if m.gpus[g.Index] == nil {
			m.gpus[g.Index] = &gpuSeries{
				util:  newRing(s.cfg.History.RetentionPoints),
				temp:  newRing(s.cfg.History.RetentionPoints),
				mem:   newRing(s.cfg.History.RetentionPoints),
				power: newRing(s.cfg.History.RetentionPoints),
			}
		}
	}
}

// Tick 由定时器驱动：刷新在线状态并按需采样历史
func (s *Store) Tick(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	offlineMs := int64(s.cfg.Alerts.OfflineAfterSec) * 1000
	for _, m := range s.machines {
		if now.UnixMilli()-m.view.LastSeen > offlineMs {
			m.view.Online = false
		}
	}
	if s.lastSample.IsZero() || now.Sub(s.lastSample) >= time.Duration(s.cfg.History.IntervalSec)*time.Second {
		s.lastSample = now
		for _, m := range s.machines {
			if !m.view.Online {
				continue
			}
			ts := now.UnixMilli()
			m.hostCPU.push(ts, m.view.CPU.Usage)
			m.hostMem.push(ts, m.view.Mem.Percent)
			for _, g := range m.view.GPUs {
				gs := m.gpus[g.Index]
				if gs == nil {
					continue
				}
				gs.util.push(ts, g.Util)
				gs.temp.push(ts, g.Temp)
				gs.power.push(ts, g.Power)
				pct := 0.0
				if g.MemTotal > 0 {
					pct = g.MemUsed / g.MemTotal * 100
				}
				gs.mem.push(ts, pct)
			}
		}
	}
}

func (s *Store) Machines() []shared.MachineView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]shared.MachineView, 0, len(s.machines))
	for _, m := range s.machines {
		out = append(out, m.view)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Online != out[j].Online {
			return out[i].Online
		}
		return out[i].Hostname < out[j].Hostname
	})
	return out
}

// History 返回某个序列的采样点，gpu < 0 表示主机级指标（cpu / mem）
func (s *Store) History(agent string, gpu int, metric string, limit int) []point {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.machines[agent]
	if !ok {
		return []point{}
	}
	if gpu < 0 {
		switch metric {
		case "mem":
			return m.hostMem.slice(limit)
		default:
			return m.hostCPU.slice(limit)
		}
	}
	gs, ok := m.gpus[gpu]
	if !ok {
		return []point{}
	}
	switch metric {
	case "temp":
		return gs.temp.slice(limit)
	case "mem":
		return gs.mem.slice(limit)
	case "power":
		return gs.power.slice(limit)
	default:
		return gs.util.slice(limit)
	}
}

func (s *Store) states() map[string]*machineState {
	return s.machines
}

func clientIP(raddr string) string {
	host, _, err := net.SplitHostPort(raddr)
	if err != nil {
		return raddr
	}
	return host
}
