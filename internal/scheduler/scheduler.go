package scheduler

import (
	"context"
	"log"
	"strconv"
	"sync"
	"time"

	"docker_stack_manager/internal/db"
	"docker_stack_manager/internal/detector"
)

// Scheduler runs periodic detection/cleanup.
type Scheduler struct {
	store   *db.Store
	engine  *detector.Engine
	mu      sync.Mutex
	cancel  context.CancelFunc
	running bool
}

// New creates a scheduler.
func New(store *db.Store, engine *detector.Engine) *Scheduler {
	return &Scheduler{store: store, engine: engine}
}

// Start begins the background loop.
func (s *Scheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.running = true
	go s.loop(ctx)
	log.Println("[scheduler] started")
}

// Stop stops the background loop.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.cancel()
	s.running = false
	log.Println("[scheduler] stopped")
}

// Reload restarts scheduler after settings change.
func (s *Scheduler) Reload() {
	s.Stop()
	s.Start()
}

func (s *Scheduler) loop(ctx context.Context) {
	for {
		interval := s.currentInterval()
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		case <-timer.C:
			s.runOnce(ctx)
		}
	}
}

func (s *Scheduler) currentInterval() time.Duration {
	v, err := s.store.GetSetting("clean_interval")
	if err != nil || v == "" {
		return 300 * time.Second
	}
	sec, err := strconv.Atoi(v)
	if err != nil || sec < 10 {
		sec = 300
	}
	return time.Duration(sec) * time.Second
}

func (s *Scheduler) runOnce(ctx context.Context) {
	auto, _ := s.store.GetSetting("auto_clean_enabled")
	runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	if auto == "true" {
		cleaned, all, err := s.engine.Clean(runCtx)
		if err != nil {
			log.Printf("[scheduler] clean error: %v", err)
			return
		}
		log.Printf("[scheduler] auto-clean done: checked=%d cleaned=%d", len(all), len(cleaned))
		return
	}

	services, err := s.engine.Detect(runCtx)
	if err != nil {
		log.Printf("[scheduler] detect error: %v", err)
		return
	}
	violations := 0
	for _, svc := range services {
		if svc.Violation.IsViolation {
			violations++
			_ = s.store.AddViolationLog(svc.Name, svc.Stack, svc.Violation.Reason)
		}
	}
	log.Printf("[scheduler] detect done: services=%d violations=%d", len(services), violations)
}