package server

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yaojiangfeng/gpu-panel/internal/shared"
)

const (
	writeWait      = 10 * time.Second
	pushMinInterval = 900 * time.Millisecond
)

type viewer struct {
	conn *websocket.Conn
	send chan []byte
}

// Hub 管理 agent 长连接与浏览器订阅
type Hub struct {
	store   *Store
	cfg     *Config
	upgrader websocket.Upgrader

	mu       sync.Mutex
	viewers  map[*viewer]struct{}
	lastPush time.Time
}

func NewHub(cfg *Config, store *Store) *Hub {
	return &Hub{
		store: store,
		cfg:   cfg,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 4096,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},
		viewers: map[*viewer]struct{}{},
	}
}

// ServeAgent agent 上报通道：/ws/agent?token=xxx
func (h *Hub) ServeAgent(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("token") != h.cfg.Token {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[agent] 升级连接失败 %s: %v", r.RemoteAddr, err)
		return
	}
	ip := clientIP(r.RemoteAddr)
	log.Printf("[agent] 已连接 %s (%s)", ip, r.RemoteAddr)

	conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		return nil
	})

	defer conn.Close()
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[agent] 连接断开 %s: %v", ip, err)
			return
		}
		conn.SetReadDeadline(time.Now().Add(90 * time.Second))

		var snap shared.Snapshot
		if err := json.Unmarshal(msg, &snap); err != nil {
			log.Printf("[agent] 忽略无法解析的上报: %v", err)
			continue
		}
		if snap.AgentID == "" {
			continue
		}
		if snap.Hostname == "" {
			snap.Hostname = snap.AgentID
		}
		h.store.Upsert(snap, ip)
		h.Broadcast()
	}
}

// ServeViewer 浏览器订阅通道：/ws/viewer
func (h *Hub) ServeViewer(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	v := &viewer{conn: conn, send: make(chan []byte, 4)}
	h.mu.Lock()
	h.viewers[v] = struct{}{}
	h.mu.Unlock()

	go v.writeLoop()
	defer func() {
		h.mu.Lock()
		delete(h.viewers, v)
		h.mu.Unlock()
		conn.Close()
	}()

	if b, err := json.Marshal(h.state()); err == nil {
		v.send <- b
	}
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (v *viewer) writeLoop() {
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case b, ok := <-v.send:
			if !ok {
				return
			}
			v.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := v.conn.WriteMessage(websocket.TextMessage, b); err != nil {
				return
			}
		case <-ticker.C:
			v.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := v.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (h *Hub) state() shared.PanelState {
	return shared.PanelState{
		Now:      time.Now().UnixMilli(),
		Machines: h.store.Machines(),
		Alerts:   h.store.Alerts(),
	}
}

// Broadcast 向所有浏览器推送最新状态，带节流避免高频刷新
func (h *Hub) Broadcast() {
	h.mu.Lock()
	if time.Since(h.lastPush) < pushMinInterval || len(h.viewers) == 0 {
		h.mu.Unlock()
		return
	}
	h.lastPush = time.Now()
	b, err := json.Marshal(h.state())
	if err != nil {
		h.mu.Unlock()
		return
	}
	dead := []*viewer{}
	for v := range h.viewers {
		select {
		case v.send <- b:
		default:
			dead = append(dead, v)
		}
	}
	for _, v := range dead {
		delete(h.viewers, v)
		v.conn.Close()
	}
	h.mu.Unlock()
}
