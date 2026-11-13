package repository

import (
	"reflect"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestModelGuardExtraPreservation(t *testing.T) {
	key := service.CodexInspectionBlockedModelsExtraKey
	current := map[string]any{key: []string{"restricted"}, "codex_eval_inspection_enabled": false, "codex_eval_inspection_verdict": "degraded"}
	stale := map[string]any{key: []string{"old"}, "codex_eval_inspection_enabled": true, "form_setting": "kept"}
	got := preserveCodexInspectionExtra(stale, current)
	if !reflect.DeepEqual(got[key], []string{"restricted"}) || got["codex_eval_inspection_enabled"] != false || got["form_setting"] != "kept" {
		t.Fatalf("new restriction lost to stale save: %#v", got)
	}
	got = preserveCodexInspectionExtra(got, nil)
	for _, owned := range codexInspectionOwnedExtraKeys {
		if _, exists := got[owned]; exists {
			t.Fatalf("restored state revived by ordinary save: %s", owned)
		}
	}
	got = preserveCodexInspectionExtra(nil, current)
	if !reflect.DeepEqual(got[key], current[key]) {
		t.Fatal("nil form lost server-owned state")
	}
}

func TestModelGuardSchedulerProjectionAndOutbox(t *testing.T) {
	key := service.CodexInspectionBlockedModelsExtraKey
	for _, names := range []any{[]string{"restricted"}, []any{"restricted"}, []string{}} {
		updates := map[string]any{key: names}
		if !shouldEnqueueSchedulerOutboxForExtraUpdates(updates) {
			t.Fatal("inspection changes must propagate to other replicas")
		}
		extra := filterSchedulerExtra(updates)
		if !reflect.DeepEqual(extra[key], names) {
			t.Fatal("scheduler projection lost deny overlay")
		}
		account := &service.Account{Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Extra: extra}
		want := reflect.DeepEqual(names, []string{})
		if account.IsModelSupported("restricted") != want {
			t.Fatalf("cached admission mismatch for %#v", names)
		}
		if !account.IsModelSupported("unrelated") {
			t.Fatal("inspection incorrectly blocked unrelated model")
		}
	}
}
