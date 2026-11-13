package codexeval

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func waitIdle(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		idle := len(active) == 0
		mu.Unlock()
		if idle {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("workers did not exit")
}

func resetModelGuardHistory(t *testing.T) string {
	t.Helper()
	waitIdle(t)
	mu.Lock()
	defer mu.Unlock()
	loaded = false
	stateLoadError = nil
	storageError = ""
	legacyPelicans, legacySIWC = nil, nil
	candyResults = map[string][]candyResult{}
	fingerprintResults = map[string][]fingerprintResult{}
	statePath = filepath.Join(t.TempDir(), "history.json")
	return statePath
}

func TestModelGuardRejectsExcludedTypesBeforeResolve(t *testing.T) {
	oldSender, oldResolver := sender, resolve
	t.Cleanup(func() { sender, resolve = oldSender, oldResolver })
	sender = nil
	if _, err := Start(context.Background(), StartRequest{}); err == nil {
		t.Fatal("uninitialized instance accepted work")
	}
	sender = func(context.Context, int64, []byte, http.Header) (*http.Response, error) {
		t.Fatal("excluded type sent a request")
		return nil, nil
	}
	resolve = func(context.Context, int64) (string, error) {
		t.Fatal("excluded type reached account resolution")
		return "", nil
	}
	for _, kind := range []string{"pelican", "siwc", "unknown"} {
		if _, err := Start(context.Background(), StartRequest{Type: kind, Targets: []Target{{1, 7}}, Model: "fixture"}); err == nil {
			t.Errorf("accepted excluded type %q", kind)
		}
	}
}

func TestModelGuardHistoryPersistsAndClearRollback(t *testing.T) {
	path := resetModelGuardHistory(t)
	for n := 0; n < 25; n++ {
		candyResults["7"] = appendRecent(candyResults["7"], candyResult{Info: Info{RunID: "fixture"}, OK: n%2 == 0, Answer: "21"})
		fingerprintResults["7"] = appendRecent(fingerprintResults["7"], fingerprintResult{ID: "record", Model: "fixture"})
	}
	mu.Lock()
	err := saveStateLocked()
	mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	candyResults, fingerprintResults = nil, nil
	loaded = false
	loadState()
	if len(candyResults["7"]) != 20 || len(fingerprintResults["7"]) != 20 {
		t.Fatal("history retention or restart persistence failed")
	}
	if detail, status := Record("fingerprint", 7, "record"); status != 200 || !strings.Contains(string(detail), `"type":"fingerprint"`) {
		t.Fatalf("record detail: %d %s", status, detail)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	statePath = filepath.Join(path, "unwritable", "history.json")
	if _, status := Clear("all"); status != 500 || len(candyResults["7"]) != 20 || len(fingerprintResults["7"]) != 20 {
		t.Fatal("failed clear did not roll back in-memory records")
	}
	statePath = path
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("failed clear changed persisted records")
	}
	if _, status := Clear("candy"); status != 200 || len(candyResults) != 0 || len(fingerprintResults["7"]) != 20 {
		t.Fatal("type-specific clear changed other history")
	}
}

func TestModelGuardMigratesWithoutExposingExcludedHistory(t *testing.T) {
	path := resetModelGuardHistory(t)
	old := `{"schema_version":2,"results":{"7":[{"run_id":"old","ok":true}]},"fingerprints":{},"pelicans":{"7":[{"html":"legacy"}]},"siwc":{"7":[{"status":"available"}]}}`
	if err := os.WriteFile(path, []byte(old), 0600); err != nil {
		t.Fatal(err)
	}
	loadState()
	if stateLoadError != nil {
		t.Fatal(stateLoadError)
	}
	backup, err := os.ReadFile(filepath.Join(filepath.Dir(path), "history.v2.json"))
	if err != nil || string(backup) != old {
		t.Fatal("migration did not preserve original backup")
	}
	raw, err := State()
	if err != nil {
		t.Fatal(err)
	}
	var public map[string]json.RawMessage
	if err := json.Unmarshal(raw, &public); err != nil {
		t.Fatal(err)
	}
	if public["pelican"] != nil || public["siwc"] != nil {
		t.Fatal("excluded evaluation history leaked into API")
	}
	for _, kind := range []string{"pelican", "siwc"} {
		if _, status := Record(kind, 7, "old"); status != 404 {
			t.Fatal("excluded record API available")
		}
		if _, status := Clear(kind); status != 400 {
			t.Fatal("excluded clear API available")
		}
	}
	if _, status := Clear("all"); status != 200 {
		t.Fatal("clear supported histories failed")
	}
	saved, _ := os.ReadFile(path)
	if !strings.Contains(string(saved), `"html":"legacy"`) || !strings.Contains(string(saved), `"status":"available"`) {
		t.Fatal("unsupported legacy records were destructively overwritten")
	}
}

func TestModelGuardCorruptHistoryFailsClosed(t *testing.T) {
	path := resetModelGuardHistory(t)
	broken := []byte(`{"schema_version":999,"results":{}}`)
	if err := os.WriteFile(path, broken, 0600); err != nil {
		t.Fatal(err)
	}
	loadState()
	if stateLoadError == nil {
		t.Fatal("unsupported history version accepted")
	}
	if _, status := Clear("all"); status != 500 {
		t.Fatal("corrupt history did not block writes")
	}
	saved, _ := os.ReadFile(path)
	if string(saved) != string(broken) {
		t.Fatal("corrupt history overwritten")
	}
}

func TestModelGuardInspectionPolicyChangesCancelOldRound(t *testing.T) {
	i := newInspector(InspectionHooks{})
	defer i.cancel()
	s := DefaultInspectionSettings()
	s.ControlModels = true
	i.configure(s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	i.runCancel = cancel
	i.state.Running = true
	s.ControlModels = false
	i.configure(s)
	if ctx.Err() == nil || !i.snapshot().Cancelling {
		t.Fatal("old model-control round survived policy change")
	}
}

func TestModelGuardInspectionRechecksScopeBeforeModelWrites(t *testing.T) {
	s := DefaultInspectionSettings()
	s.GroupID = 101
	writes := 0
	i := newInspector(InspectionHooks{
		Get: func(context.Context, int64) (InspectionAccount, error) {
			return InspectionAccount{GroupIDs: []int64{202}, Available: true, Enabled: true, AutoInspectionEnabled: true}, nil
		},
		PersistVerdict: func(context.Context, int64, InspectionVerdict, InspectionSettings) error { writes++; return nil },
	})
	defer i.cancel()
	i.state.Results = []InspectionResult{{Outcome: "recommended"}}
	if i.applyAccount(context.Background(), s, 0, InspectionAccount{Target: Target{AccountID: 7}}, 10, 99) || writes != 0 {
		t.Fatal("account outside the selected group was modified")
	}
}

func TestModelGuardInspectionRequiresCompleteKnownFingerprint(t *testing.T) {
	probability := 0.9
	r := fingerprintResult{Status: "completed", Model: sharedDetector.ModelIDs[0], Attribution: &fingerprintAttribution{
		Used: fingerprintTarget, Prediction: sharedDetector.ModelIDs[0], Probability: &probability,
	}}
	if !inspectionFingerprintUsable(r) {
		t.Fatal("complete enrolled attribution rejected")
	}
	for _, test := range []func(*fingerprintResult){
		func(r *fingerprintResult) { r.Status = "partial" },
		func(r *fingerprintResult) { r.Attribution.Decision = "not_model" },
		func(r *fingerprintResult) { r.Attribution.Used = 2 },
		func(r *fingerprintResult) { r.Attribution.Prediction = "" },
		func(r *fingerprintResult) { r.Attribution.Probability = nil },
		func(r *fingerprintResult) { r.Model = "custom-unenrolled-model" },
	} {
		candidate := r
		attribution := *r.Attribution
		candidate.Attribution = &attribution
		test(&candidate)
		if inspectionFingerprintUsable(candidate) {
			t.Fatalf("unsafe fingerprint accepted: %+v", candidate)
		}
	}
}

func TestModelGuardSafeInspectionDefaults(t *testing.T) {
	s := DefaultInspectionSettings()
	if s.Enabled || s.ControlModels || s.AdjustPriority {
		t.Fatal("automatic account modifications enabled by default")
	}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
}
