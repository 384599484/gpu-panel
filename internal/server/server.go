package server

import (
	"crypto/subtle"
	"log"
	"net/http"
	"time"
)

type Server struct {
	cfg    *Config
	store  *Store
	hub    *Hub
	notify *Notifier
}

func New(cfg *Config) *Server {
	store := NewStore(cfg)
	return &Server{cfg: cfg, store: store, hub: NewHub(cfg, store), notify: NewNotifier(cfg)}
}

// NotifyTest 发送一条企业微信自检消息
func (s *Server) NotifyTest() error {
	return s.notify.Test()
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	// agent 通道用 token 鉴权，不走面板 Basic 认证
	mux.HandleFunc("/ws/agent", s.hub.ServeAgent)

	protected := http.NewServeMux()
	protected.HandleFunc("/ws/viewer", s.hub.ServeViewer)
	protected.HandleFunc("/api/state", s.handleState)
	protected.HandleFunc("/api/history", s.handleHistory)
	protected.Handle("/", http.FileServer(http.FS(webFiles())))

	mux.Handle("/", s.panelAuth(protected))
	return mux
}

func (s *Server) panelAuth(next http.Handler) http.Handler {
	user, pass := s.cfg.PanelUser, s.cfg.PanelPass
	if user == "" || pass == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(u), []byte(user)) != 1 ||
			subtle.ConstantTimeCompare([]byte(p), []byte(pass)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="gpu-panel"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) Run() error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	go func() {
		for t := range ticker.C {
			s.store.Tick(t)
			s.notify.Observe(s.store.Alerts())
			s.hub.Broadcast()
		}
	}()

	log.Printf("[服务端] 监听 %s", s.cfg.Listen)
	log.Printf("[服务端] agent 接入地址 ws://<公网IP>%s/ws/agent?token=%s", s.cfg.Listen, s.cfg.Token)
	if s.cfg.PanelUser != "" {
		log.Printf("[服务端] 面板已启用 Basic 认证（用户 %s）", s.cfg.PanelUser)
	} else {
		log.Printf("[服务端] 面板未启用认证，建议配置 panel_user / panel_pass")
	}
	log.Printf("[服务端] 企业微信推送：%s", s.notify.Status())

	srv := &http.Server{
		Addr:              s.cfg.Listen,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	return srv.ListenAndServe()
}
