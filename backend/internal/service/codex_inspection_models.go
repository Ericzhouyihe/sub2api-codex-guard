package service

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/codexeval"
)

const codexInspectionRemovedModels = "codex_eval_inspection_removed_models"
const codexInspectionVerdict = "codex_eval_inspection_verdict"

// CodexInspectionBlockedModelsExtraKey is an inspection-owned deny overlay, not
// a replacement model allowlist. Scheduler snapshots must retain this key.
const CodexInspectionBlockedModelsExtraKey = "codex_eval_inspection_blocked_models"

func codexInspectionModelNames(value any) []string {
	var names []string
	switch value := value.(type) {
	case []string:
		names = value
	case []any:
		for _, item := range value {
			if name, ok := item.(string); ok {
				names = append(names, name)
			}
		}
	}
	out := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name != "" && !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out
}

// codexInspectionModelBlocked is consulted before passthrough/alias/empty-map
// admission. Matching the resolved target also blocks aliases and wildcards
// pointing at an inspected upstream model without deleting user mappings.
func (a *Account) codexInspectionModelBlocked(requestedModel string) bool {
	if a == nil {
		return false
	}
	if a.codexInspectionUpstreamModelBlocked(requestedModel) {
		return true
	}
	// Normal passthrough forwarding deliberately ignores model_mapping; stale
	// mappings must not turn a selected-model restriction into a broader deny.
	return !a.IsOpenAIPassthroughEnabled() && a.codexInspectionUpstreamModelBlocked(a.GetMappedModel(requestedModel))
}

// codexInspectionUpstreamModelBlocked checks an already-resolved model without
// mapping it a second time (administrator mappings may contain a -> b -> c
// chains, whose normal forwarding semantics are deliberately only one hop).
func (a *Account) codexInspectionUpstreamModelBlocked(model string) bool {
	if a == nil {
		return false
	}
	model = strings.TrimSpace(model)
	switch names := a.Extra[CodexInspectionBlockedModelsExtraKey].(type) {
	case []string:
		for _, name := range names {
			if model == strings.TrimSpace(name) {
				return true
			}
		}
	case []any:
		for _, item := range names {
			if name, ok := item.(string); ok && model == strings.TrimSpace(name) {
				return true
			}
		}
	}
	return false
}

func (s *AccountTestService) PersistCodexInspectionVerdict(ctx context.Context, id int64, verdict codexeval.InspectionVerdict, settings codexeval.InspectionSettings) error {
	if verdict != codexeval.InspectionVerdictNormal && verdict != codexeval.InspectionVerdictDegraded {
		return fmt.Errorf("unsupported inspection verdict: %q", verdict)
	}
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if account == nil {
		return fmt.Errorf("inspection account %d not found", id)
	}
	blocked := codexInspectionModelNames(account.Extra[CodexInspectionBlockedModelsExtraKey])
	nextBlocked := slices.Clone(blocked)
	if verdict == codexeval.InspectionVerdictNormal {
		// Only this inspection's overlay is restored. Never reopen unrelated
		// quality isolation, cooldowns, Astra restrictions or user allowlists.
		nextBlocked = []string{}
	} else if settings.ControlModels {
		settings.NormalizeModels()
		for _, name := range settings.DegradedRemoveModels {
			if !codexeval.ValidInspectionRemovalModel(name) {
				return fmt.Errorf("cannot control inspection model %q: invalid literal model name", name)
			}
			if !slices.Contains(nextBlocked, name) {
				nextBlocked = append(nextBlocked, name)
			}
		}
	}
	if len(nextBlocked) > 0 {
		candidate := *account
		candidate.Extra = maps.Clone(account.Extra)
		if candidate.Extra == nil {
			candidate.Extra = map[string]any{}
		}
		candidate.Extra[CodexInspectionBlockedModelsExtraKey] = nextBlocked
		for _, name := range nextBlocked {
			// Fail closed if this fork's admission path does not support the
			// overlay. Merely saving metadata must never claim model removal.
			if candidate.IsModelSupported(name) {
				return fmt.Errorf("cannot block inspection model %q for account %d: scheduler admission does not support managed model blocking", name, id)
			}
		}
	}
	current := codexeval.InspectionVerdictNormal
	if account.Extra[codexInspectionVerdict] == string(codexeval.InspectionVerdictDegraded) {
		current = codexeval.InspectionVerdictDegraded
	}
	if current == verdict && slices.Equal(blocked, nextBlocked) {
		return nil
	}
	updates := map[string]any{codexInspectionVerdict: string(verdict)}
	if !slices.Equal(blocked, nextBlocked) {
		updates[CodexInspectionBlockedModelsExtraKey] = nextBlocked
	}
	// A top-level Extra merge avoids whole-account/credential rewrites and
	// retains repository cache invalidation. The arrays are never mutated in
	// place, so a failed write cannot change a cached account's admission.
	return s.accountRepo.UpdateExtra(ctx, id, updates)
}
