package guard

import (
	"encoding/json"
	"time"
)

const (
	maxLogs         = 200
	maxResults      = 200
	maxPelicanItems = 20
	kvNamespace     = "codex-guard"
	kvStateKey      = "state"
)

type logEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

// Result 是一次检测记录；不同检测类型使用其中的不同字段。
type Result struct {
	Test        string `json:"test,omitempty"`
	AccountID   int64  `json:"account_id"`
	AccountName string `json:"account_name"`
	Model       string `json:"model"`
	Run         int    `json:"run"`
	Status      string `json:"status"`
	Answer      string `json:"answer,omitempty"`
	HTML        string `json:"html,omitempty"`
	Variant     string `json:"variant,omitempty"`
	Attempts    int    `json:"attempts,omitempty"`
	Error       string `json:"error,omitempty"`
	DurationMS  int64  `json:"duration_ms"`
	Time        string `json:"time"`

	DetectorRevision string                  `json:"detector_revision,omitempty"`
	Samples          []FingerprintSample     `json:"samples,omitempty"`
	Attribution      *FingerprintAttribution `json:"attribution,omitempty"`

	Code       string `json:"code,omitempty"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

// Summary 是当前结果集的汇总计数。
type Summary struct {
	Total     int `json:"total"`
	Correct   int `json:"correct,omitempty"`
	Wrong     int `json:"wrong,omitempty"`
	Failed    int `json:"failed,omitempty"`
	Skipped   int `json:"skipped,omitempty"`
	Completed int `json:"completed,omitempty"`
	Partial   int `json:"partial,omitempty"`
	Available int `json:"available,omitempty"`
}

func (s *Summary) add(status string) {
	s.Total++
	switch status {
	case "correct":
		s.Correct++
	case "wrong":
		s.Wrong++
	case "failed", "auth_error", "restricted", "limited":
		s.Failed++
	case "skipped":
		s.Skipped++
	case "completed":
		s.Completed++
	case "partial":
		s.Partial++
	case "available":
		s.Available++
	}
}

// AccountView 是状态面板展示的账号快照。
type AccountView struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Schedulable bool   `json:"schedulable"`
}

// TestStatus 是一类检测的状态与结果。
type TestStatus struct {
	Running   bool          `json:"running"`
	Stopping  bool          `json:"stopping"`
	LastRunAt string        `json:"last_run_at"`
	LastError string        `json:"last_error"`
	Summary   Summary       `json:"summary"`
	Results   []Result      `json:"results"`
	Accounts  []AccountView `json:"accounts"`
}

type persistedTest struct {
	LastRunAt string   `json:"last_run_at"`
	Summary   Summary  `json:"summary"`
	Results   []Result `json:"results"`
}

type persistedState struct {
	// 兼容 v0.1.0 的糖果单测试状态。
	LastRunAt string   `json:"last_run_at,omitempty"`
	Summary   Summary  `json:"summary,omitempty"`
	Results   []Result `json:"results,omitempty"`

	Accounts []AccountView            `json:"accounts,omitempty"`
	Tests    map[string]persistedTest `json:"tests,omitempty"`
}

func (e *Engine) log(level, message string) {
	e.logMu.Lock()
	defer e.logMu.Unlock()
	e.logs = append(e.logs, logEntry{Time: time.Now().UTC().Format(time.RFC3339), Level: level, Message: message})
	if len(e.logs) > maxLogs {
		e.logs = append([]logEntry(nil), e.logs[len(e.logs)-maxLogs:]...)
	}
}

func (e *Engine) logsSnapshot() []logEntry {
	e.logMu.Lock()
	defer e.logMu.Unlock()
	return append([]logEntry(nil), e.logs...)
}

func (e *Engine) statusJSON() string {
	tests, accounts := e.runner.Status()
	payload := map[string]any{
		"version":     Version,
		"tests":       tests,
		"accounts":    accounts,
		"logs":        e.logsSnapshot(),
		"next_run_at": e.scheduler.NextRunAt(),
	}
	// 兼容 v0.1.0 的 UI 和脚本：candy 字段始终存在。
	for name, status := range tests {
		payload[name] = status
	}
	if _, ok := payload["candy"]; !ok {
		payload["candy"] = TestStatus{Results: []Result{}, Accounts: accounts}
	}
	raw, _ := json.Marshal(payload)
	return string(raw)
}
