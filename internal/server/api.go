package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.hub.state())
}

// handleHistory 支持单指标（返回数组）或多指标 metric=util,temp（返回 map）
func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	agent := q.Get("agent")

	gpu := -1
	if v := q.Get("gpu"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			gpu = n
		}
	}
	metric := q.Get("metric")
	if metric == "" {
		metric = "util"
	}
	points := 120
	if v := q.Get("points"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			points = n
		}
	}

	w.Header().Set("Content-Type", "application/json")
	names := strings.Split(metric, ",")
	if len(names) > 1 {
		out := map[string][]point{}
		for _, n := range names {
			n = strings.TrimSpace(n)
			out[n] = s.store.History(agent, gpu, n, points)
		}
		json.NewEncoder(w).Encode(out)
		return
	}
	json.NewEncoder(w).Encode(s.store.History(agent, gpu, strings.TrimSpace(metric), points))
}
