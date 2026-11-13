package guard

import (
	"sync"
	"time"

	"github.com/codexguard/sub2api-codex-guard/internal/config"
)

// Scheduler 按固定间隔触发配置中启用的检测项目；间隔来自配置，最小 5 分钟。
type Scheduler struct {
	engine *Engine

	mu       sync.Mutex
	enabled  bool
	interval time.Duration
	next     time.Time
	stopOnce sync.Once
	stopCh   chan struct{}
}

func newScheduler(engine *Engine) *Scheduler {
	return &Scheduler{engine: engine, stopCh: make(chan struct{})}
}

func (s *Scheduler) Start() {
	go s.loop()
}

func (s *Scheduler) Stop() {
	s.stopOnce.Do(func() { close(s.stopCh) })
}

// Configure 在配置应用时调用。开启定时且尚未安排时，从现在起算一个间隔。
func (s *Scheduler) Configure(cfg config.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	oldInterval := s.interval
	s.enabled = cfg.ScheduleEnabled
	s.interval = time.Duration(cfg.ScheduleMinutes) * time.Minute
	switch {
	case !s.enabled:
		s.next = time.Time{}
	case s.next.IsZero() || oldInterval != s.interval:
		s.next = time.Now().Add(s.interval)
	}
}

func (s *Scheduler) loop() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.tick()
		case <-s.stopCh:
			return
		}
	}
}

func (s *Scheduler) tick() {
	s.mu.Lock()
	if !s.enabled || s.next.IsZero() || time.Now().Before(s.next) {
		s.mu.Unlock()
		return
	}
	s.next = time.Now().Add(s.interval)
	s.mu.Unlock()

	cfg := s.engine.config()
	if err := s.engine.runner.Start(cfg, "schedule", cfg.ScheduleTests); err != nil {
		s.engine.log("info", "定时检测跳过："+err.Error())
		return
	}
	s.engine.log("info", "已触发定时检测："+stringsJoin(cfg.ScheduleTests))
}

// AfterScheduledRun 在一轮定时检测结束后安排下一轮，避免检测耗时造成漂移累积。
func (s *Scheduler) AfterScheduledRun() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.enabled {
		s.next = time.Now().Add(s.interval)
	}
}

func (s *Scheduler) NextRunAt() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.enabled || s.next.IsZero() {
		return ""
	}
	return s.next.UTC().Format(time.RFC3339)
}
