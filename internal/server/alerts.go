package server

import (
	"fmt"
	"sort"
	"time"

	"github.com/yaojiangfeng/gpu-panel/internal/shared"
)

// Alerts 根据当前状态计算告警列表，持续中的告警保留首次触发时间
func (s *Store) Alerts() []shared.Alert {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UnixMilli()
	out := []shared.Alert{}
	live := map[string]bool{}

	for id, m := range s.machines {
		v := m.view
		if !v.Online {
			key := id + "|offline"
			live[key] = true
			out = append(out, shared.Alert{
				Kind:     "offline",
				Level:    "crit",
				AgentID:  id,
				Hostname: v.Hostname,
				GPU:      -1,
				Message:  fmt.Sprintf("矿机已离线（最后上报 %s）", humanAgo(now-v.LastSeen)),
				Since:    bump(m, key, now),
			})
			continue
		}
		if m.maxGPU > 0 && len(v.GPUs) < m.maxGPU {
			key := id + "|gpu_drop"
			live[key] = true
			out = append(out, shared.Alert{
				Kind:     "gpu_drop",
				Level:    "crit",
				AgentID:  id,
				Hostname: v.Hostname,
				GPU:      -1,
				Message:  fmt.Sprintf("显卡数量从 %d 减少到 %d，疑似掉卡", m.maxGPU, len(v.GPUs)),
				Since:    bump(m, key, now),
			})
		}
		th := float64(s.cfg.Alerts.GPUTempThreshold)
		for _, g := range v.GPUs {
			if g.Temp <= 0 {
				continue
			}
			if g.Temp >= th {
				key := fmt.Sprintf("%s|gpu_temp|%d", id, g.Index)
				live[key] = true
				level := "warn"
				if g.Temp >= th+7 {
					level = "crit"
				}
				out = append(out, shared.Alert{
					Kind:     "gpu_temp",
					Level:    level,
					AgentID:  id,
					Hostname: v.Hostname,
					GPU:      g.Index,
					Message:  fmt.Sprintf("GPU%d %s 温度 %.0f°C，超过阈值 %.0f°C", g.Index, g.Name, g.Temp, th),
					Since:    bump(m, key, now),
				})
			}
			idleTh := float64(s.cfg.Alerts.GPUIdleThreshold)
			if idleTh > 0 {
				key := fmt.Sprintf("%s|gpu_idle|%d", id, g.Index)
				if g.Util < idleTh {
					if m.idleSince[g.Index] == 0 {
						m.idleSince[g.Index] = now
					}
					if now-m.idleSince[g.Index] > int64(s.cfg.Alerts.GPUIdleAfterSec)*1000 {
						live[key] = true
						out = append(out, shared.Alert{
							Kind:     "gpu_idle",
							Level:    "warn",
							AgentID:  id,
							Hostname: v.Hostname,
							GPU:      g.Index,
							Message:  fmt.Sprintf("GPU%d 利用率 %.0f%% 已持续 %s 低于 %.0f%%，疑似停挖", g.Index, g.Util, humanAgo(now-m.idleSince[g.Index]), idleTh),
							Since:    bump(m, key, now),
						})
					}
				} else {
					delete(m.idleSince, g.Index)
					delete(m.since, key)
				}
			}
		}
	}
	// 清理已恢复的告警时间戳
	for _, m := range s.machines {
		for k := range m.since {
			if !live[k] {
				delete(m.since, k)
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Level != out[j].Level {
			return out[i].Level == "crit"
		}
		return out[i].Since < out[j].Since
	})
	return out
}

func bump(m *machineState, key string, now int64) int64 {
	if _, ok := m.since[key]; !ok {
		m.since[key] = now
	}
	return m.since[key]
}

func humanAgo(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	sec := ms / 1000
	switch {
	case sec < 60:
		return fmt.Sprintf("%d 秒前", sec)
	case sec < 3600:
		return fmt.Sprintf("%d 分钟前", sec/60)
	case sec < 86400:
		return fmt.Sprintf("%d 小时前", sec/3600)
	default:
		return fmt.Sprintf("%d 天前", sec/86400)
	}
}
