package repository

import "github.com/Wei-Shaw/sub2api/internal/service"

var codexInspectionOwnedExtraKeys = []string{
	"codex_eval_inspection_enabled",
	"codex_eval_inspection_verdict",
	"codex_eval_inspection_removed_models",
	service.CodexInspectionBlockedModelsExtraKey,
}

// Call after reading currentExtra under the account update row lock. Ordinary
// edits may neither restore an old restriction nor erase a newer one. The
// dedicated inspection operations use UpdateExtra's atomic JSONB merge.
func preserveCodexInspectionExtra(extra, currentExtra map[string]any) map[string]any {
	for _, key := range codexInspectionOwnedExtraKeys {
		delete(extra, key)
		if value, exists := currentExtra[key]; exists {
			if extra == nil {
				extra = make(map[string]any)
			}
			extra[key] = value
		}
	}
	return extra
}
