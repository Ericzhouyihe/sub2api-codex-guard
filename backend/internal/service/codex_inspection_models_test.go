// Adapted from codex-candy-eval-source/tests/test_service_codex_inspection_models_test.go.
package service

import (
	"context"
	"errors"
	"maps"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/codexeval"
	"github.com/stretchr/testify/require"
)

type codexInspectionModelRepo struct {
	*upstreamBillingProbeAccountRepo
	updates, merges int
	fail            bool
}

func (r *codexInspectionModelRepo) Update(ctx context.Context, account *Account) error {
	r.updates++
	return errors.New("inspection must not rewrite credentials or whole accounts")
}
func (r *codexInspectionModelRepo) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	r.merges++
	if r.fail {
		return errors.New("fixture inspection update failure")
	}
	return r.upstreamBillingProbeAccountRepo.UpdateExtra(ctx, id, updates)
}

func newCodexInspectionModelFixture(mapping map[string]any) (*AccountTestService, *codexInspectionModelRepo) {
	a := &Account{ID: 20, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"api_key": "fixture-key", "base_url": "https://fixture.invalid", "model_mapping": mapping,
			"compact_model_mapping": map[string]any{"compact": "target-model"}},
		Extra: map[string]any{"unrelated": true, "openai_responses_supported": true, codexInspectionVerdict: "normal",
			QualityModelIsolationKey: map[string]any{"independent": map[string]any{"reason": "quality_rule:17"}}}}
	r := &codexInspectionModelRepo{upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{20: a}}}
	return &AccountTestService{accountRepo: r}, r
}

func TestCodexInspectionModelTransitions(t *testing.T) {
	for _, mode := range []string{"restricted", "last-entry", "empty", "absent", "wildcard", "wildcard-target", "alias", "passthrough", "oauth-aliases"} {
		t.Run(mode, func(t *testing.T) {
			mapping := map[string]any{"target-model": "target-model", "other": "other"}
			svc, repo := newCodexInspectionModelFixture(mapping)
			a := repo.accounts[20]
			switch mode {
			case "last-entry":
				delete(mapping, "other")
			case "empty":
				a.Credentials["model_mapping"] = map[string]any{}
			case "absent":
				delete(a.Credentials, "model_mapping")
			case "wildcard":
				mapping["*"] = "other"
			case "wildcard-target":
				mapping["alias-*"] = "target-model"
			case "alias":
				mapping["alias"] = "target-model"
			case "passthrough":
				a.Extra["openai_passthrough"] = true
			case "oauth-aliases":
				a.Type = AccountTypeOAuth
				a.Credentials[OpenAIModelMappingModeKey] = "aliases"
			}
			credentials := maps.Clone(a.Credentials)
			otherAdmitted := a.IsModelSupported("other")
			settings := codexeval.DefaultInspectionSettings()
			settings.ControlModels = true
			settings.DegradedRemoveModels = []string{"target-model"}
			ctx := context.Background()
			require.True(t, a.IsModelSupported("target-model"))
			require.NoError(t, svc.PersistCodexInspectionVerdict(ctx, 20, codexeval.InspectionVerdictNormal, settings))
			require.Zero(t, repo.merges)
			require.NoError(t, svc.PersistCodexInspectionVerdict(ctx, 20, codexeval.InspectionVerdictDegraded, settings))
			require.False(t, a.IsModelSupported("target-model"), "degradation must actually block account admission")
			require.Equal(t, otherAdmitted, a.IsModelSupported("other"), "unrelated admission must not change")
			if mode == "alias" {
				require.False(t, a.IsModelSupported("alias"))
			}
			if mode == "wildcard-target" {
				require.False(t, a.IsModelSupported("alias-anything"))
			}
			require.False(t, supportsOpenAIRequestModel(a, "compact", true), "compact targets cannot bypass inspection")
			require.True(t, a.Schedulable)
			require.Equal(t, credentials, a.Credentials)
			require.Equal(t, []string{"target-model"}, a.Extra[CodexInspectionBlockedModelsExtraKey])
			require.NoError(t, svc.PersistCodexInspectionVerdict(ctx, 20, codexeval.InspectionVerdictDegraded, settings))
			require.Equal(t, 1, repo.merges, "stable degradation is write-free")
			// An administrator may edit mappings while degraded. Recovery must
			// leave the new mapping and every independent restriction in place.
			a.Credentials["model_mapping"] = map[string]any{"target-model": "administrator-target", "other": "other"}
			a.Extra[CodexInspectionBlockedModelsExtraKey] = []any{"target-model"}
			settings.ControlModels = false
			settings.DegradedRemoveModels = []string{"changed-global-target"}
			require.NoError(t, svc.PersistCodexInspectionVerdict(ctx, 20, codexeval.InspectionVerdictNormal, settings))
			require.Equal(t, "administrator-target", a.GetMappedModel("target-model"))
			require.True(t, a.IsModelSupported("target-model"))
			require.True(t, a.IsQualityModelIsolated("independent"))
			require.Empty(t, a.Extra[CodexInspectionBlockedModelsExtraKey])
			require.Equal(t, "normal", a.Extra[codexInspectionVerdict])
			require.NoError(t, svc.PersistCodexInspectionVerdict(ctx, 20, codexeval.InspectionVerdictNormal, settings))
			require.Equal(t, 2, repo.merges)
			require.Zero(t, repo.updates)
		})
	}
}

func TestCodexInspectionModelDisabledAndEmptyList(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		svc, repo := newCodexInspectionModelFixture(nil)
		settings := codexeval.DefaultInspectionSettings()
		settings.ControlModels = enabled
		if enabled {
			settings.DegradedRemoveModels = []string{}
		}
		require.NoError(t, svc.PersistCodexInspectionVerdict(context.Background(), 20, codexeval.InspectionVerdictDegraded, settings))
		require.True(t, repo.accounts[20].IsModelSupported("target-model"))
		require.NotContains(t, repo.accounts[20].Extra, CodexInspectionBlockedModelsExtraKey)
		require.Equal(t, 1, repo.merges)
	}
}

func TestCodexInspectionModelWriteFailure(t *testing.T) {
	svc, repo := newCodexInspectionModelFixture(map[string]any{"target-model": "target-model"})
	repo.fail = true
	settings := codexeval.DefaultInspectionSettings()
	settings.ControlModels = true
	settings.DegradedRemoveModels = []string{"target-model"}
	require.ErrorContains(t, svc.PersistCodexInspectionVerdict(context.Background(), 20, codexeval.InspectionVerdictDegraded, settings), "fixture inspection update failure")
	a := repo.accounts[20]
	require.Equal(t, "normal", a.Extra[codexInspectionVerdict])
	require.NotContains(t, a.Extra, CodexInspectionBlockedModelsExtraKey)
	require.True(t, a.IsModelSupported("target-model"))
	require.Zero(t, repo.updates)
}

func TestCodexInspectionModelsAccumulateUntilRecovery(t *testing.T) {
	svc, repo := newCodexInspectionModelFixture(nil)
	settings := codexeval.DefaultInspectionSettings()
	settings.ControlModels = true
	for _, names := range [][]string{{"first", "first"}, {"second"}} {
		settings.DegradedRemoveModels = names
		require.NoError(t, svc.PersistCodexInspectionVerdict(context.Background(), 20, codexeval.InspectionVerdictDegraded, settings))
	}
	a := repo.accounts[20]
	require.Equal(t, []string{"first", "second"}, a.Extra[CodexInspectionBlockedModelsExtraKey])
	require.False(t, a.IsModelSupported("first"))
	require.False(t, a.IsModelSupported("second"))
	for _, invalid := range []string{"*", "", "bad\nname"} {
		settings.DegradedRemoveModels = []string{invalid}
		require.Error(t, svc.PersistCodexInspectionVerdict(context.Background(), 20, codexeval.InspectionVerdictDegraded, settings))
	}
	require.Error(t, svc.PersistCodexInspectionVerdict(context.Background(), 20, codexeval.InspectionVerdict("unknown"), settings))
	require.Equal(t, 2, repo.merges)
}

func TestCodexInspectionModelsRespectOneHopAndPassthrough(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		t.Run(map[bool]string{false: "one-hop", true: "passthrough-stale-mapping"}[passthrough], func(t *testing.T) {
			svc, repo := newCodexInspectionModelFixture(map[string]any{"first": "second", "second": "target-model", "target-model": "target-model"})
			a := repo.accounts[20]
			a.Extra["openai_passthrough"] = passthrough
			settings := codexeval.DefaultInspectionSettings()
			settings.ControlModels = true
			settings.DegradedRemoveModels = []string{"target-model"}
			require.NoError(t, svc.PersistCodexInspectionVerdict(context.Background(), 20, codexeval.InspectionVerdictDegraded, settings))
			require.True(t, a.IsModelSupported("first"), "one-hop mapping must not recursively follow second -> target-model")
			require.Equal(t, passthrough, a.IsModelSupported("second"), "passthrough must ignore stale ordinary mappings")
			require.False(t, a.IsModelSupported("target-model"))
		})
	}
}
