package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"docker_stack_manager/internal/db"
	"docker_stack_manager/internal/detector"
	"docker_stack_manager/internal/models"
	"docker_stack_manager/internal/scheduler"
)

// Server serves REST APIs and static files.
type Server struct {
	store     *db.Store
	engine    *detector.Engine
	scheduler *scheduler.Scheduler
	staticDir string
	mux       *http.ServeMux
}

// New creates an API server.
func New(store *db.Store, engine *detector.Engine, sched *scheduler.Scheduler, staticDir string) *Server {
	s := &Server{
		store:     store,
		engine:    engine,
		scheduler: sched,
		staticDir: staticDir,
		mux:       http.NewServeMux(),
	}
	s.routes()
	return s
}

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler {
	return s.withCORS(s.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/api/stacks", s.handleStacks)
	s.mux.HandleFunc("/api/stacks/", s.handleStackSub)
	s.mux.HandleFunc("/api/services", s.handleServices)
	s.mux.HandleFunc("/api/violations", s.handleViolations)
	s.mux.HandleFunc("/api/clean", s.handleClean)
	s.mux.HandleFunc("/api/settings", s.handleSettings)
	s.mux.HandleFunc("/api/stats", s.handleStats)
	s.mux.HandleFunc("/api/logs", s.handleLogs)

	fs := http.FileServer(http.Dir(s.staticDir))
	s.mux.Handle("/", s.spaFallback(fs))
}

func (s *Server) spaFallback(fs http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/" {
			http.ServeFile(w, r, s.staticDir+"/index.html")
			return
		}
		fs.ServeHTTP(w, r)
	}
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, message string, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(models.APIResponse{
		Code:    code,
		Message: message,
		Data:    data,
	})
}

func writeErr(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, message, nil)
}

func readJSON(r *http.Request, dst interface{}) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	return dec.Decode(dst)
}

func (s *Server) handleStacks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		stacks, err := s.store.ListStacks()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, "ok", stacks)
	case http.MethodPost:
		var req models.CreateStackRequest
		if err := readJSON(r, &req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		st, err := s.store.CreateStack(req.Name, req.Description)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, "created", st)
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleStackSub(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/stacks/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid stack id")
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			st, err := s.store.GetStack(id)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			if st == nil {
				writeErr(w, http.StatusNotFound, "stack not found")
				return
			}
			writeJSON(w, http.StatusOK, "ok", st)
		case http.MethodPut:
			var req models.UpdateStackRequest
			if err := readJSON(r, &req); err != nil {
				writeErr(w, http.StatusBadRequest, "invalid json")
				return
			}
			if err := s.store.UpdateStack(id, req.Description); err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			st, _ := s.store.GetStack(id)
			writeJSON(w, http.StatusOK, "updated", st)
		case http.MethodDelete:
			if err := s.store.DeleteStack(id); err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, "deleted", nil)
		default:
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	if len(parts) == 2 && parts[1] == "ports" {
		switch r.Method {
		case http.MethodGet:
			ports, err := s.store.ListPorts(id)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, "ok", ports)
		case http.MethodPost:
			var req models.AddPortRequest
			if err := readJSON(r, &req); err != nil {
				writeErr(w, http.StatusBadRequest, "invalid json")
				return
			}
			p, err := s.store.AddPort(id, req.Port, req.Protocol)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, "created", p)
		default:
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	if len(parts) == 3 && parts[1] == "ports" {
		portID, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid port id")
			return
		}
		if r.Method != http.MethodDelete {
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if err := s.store.DeletePort(portID); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, "deleted", nil)
		return
	}

	writeErr(w, http.StatusNotFound, "not found")
}

func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	services, err := s.engine.Detect(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "docker error: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, "ok", services)
}

func (s *Server) handleViolations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	services, err := s.engine.Detect(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "docker error: "+err.Error())
		return
	}
	var violations []models.ServiceInfo
	for _, svc := range services {
		if svc.Violation.IsViolation {
			violations = append(violations, svc)
		}
	}
	if violations == nil {
		violations = []models.ServiceInfo{}
	}
	writeJSON(w, http.StatusOK, "ok", violations)
}

func (s *Server) handleClean(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	cleaned, all, err := s.engine.Clean(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, "cleaned", map[string]interface{}{
		"cleaned": cleaned,
		"checked": len(all),
		"removed": len(cleaned),
	})
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := s.store.GetSettings()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, "ok", settings)
	case http.MethodPut:
		var req map[string]string
		if err := readJSON(r, &req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if v, ok := req["clean_interval"]; ok {
			n, err := strconv.Atoi(v)
			if err != nil || n < 10 {
				writeErr(w, http.StatusBadRequest, "clean_interval must be integer >= 10")
				return
			}
		}
		if v, ok := req["auto_clean_enabled"]; ok {
			if v != "true" && v != "false" {
				writeErr(w, http.StatusBadRequest, "auto_clean_enabled must be true/false")
				return
			}
		}
		if err := s.store.UpdateSettings(req); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.scheduler.Reload()
		settings, _ := s.store.GetSettings()
		writeJSON(w, http.StatusOK, "updated", settings)
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	stats, err := s.engine.Stats(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, "docker unavailable: "+err.Error(), stats)
		return
	}
	writeJSON(w, http.StatusOK, "ok", stats)
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	logs, err := s.store.ListViolationLogs(200)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, "ok", logs)
}