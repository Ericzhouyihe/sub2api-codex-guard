package config

import (
	"encoding/json"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if len(cfg.Models) != 1 || cfg.Models[0] != DefaultModel {
		t.Fatalf("unexpected default models: %#v", cfg.Models)
	}
	if cfg.Runs != 3 || cfg.Concurrency != 4 || cfg.ScheduleMinutes != 60 {
		t.Fatalf("unexpected default config: %#v", cfg)
	}
	if cfg.ActiveTest != TestCandy || len(cfg.ScheduleTests) != 1 || cfg.ScheduleTests[0] != TestCandy {
		t.Fatalf("unexpected default tests: %#v", cfg)
	}
	if cfg.FingerprintConcurrency != 3 || cfg.PelicanVariant != "original" {
		t.Fatalf("unexpected default engine config: %#v", cfg)
	}
}

func TestParseEmptyUsesDefaults(t *testing.T) {
	cfg, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse(nil): %v", err)
	}
	if cfg.Runs != 3 || cfg.ScheduleMinutes != 60 {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}
}

func TestParseNormalizesModelsAndIDs(t *testing.T) {
	raw := []byte(`{
		"models": [" gpt-5.5 ", "gpt-5.5", "gpt-5.4"],
		"runs": 2,
		"concurrency": 3,
		"effort": "HIGH",
		"account_ids": [3, 3, 7],
		"schedule_enabled": true,
		"schedule_minutes": 30
	}`)
	cfg, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Models) != 2 || cfg.Models[0] != "gpt-5.5" || cfg.Models[1] != "gpt-5.4" {
		t.Fatalf("models not normalized: %#v", cfg.Models)
	}
	if cfg.Effort != "high" {
		t.Fatalf("effort = %q", cfg.Effort)
	}
	if len(cfg.AccountIDs) != 2 || cfg.AccountIDs[0] != 3 || cfg.AccountIDs[1] != 7 {
		t.Fatalf("ids not normalized: %#v", cfg.AccountIDs)
	}
}

func TestParseRejectsInvalid(t *testing.T) {
	cases := map[string]string{
		"unknown field":   `{"nope": true}`,
		"runs too large":  `{"runs": 11}`,
		"bad concurrency": `{"concurrency": 17}`,
		"bad effort":      `{"effort": "max"}`,
		"bad account id":  `{"account_ids": [0]}`,
		"bad schedule":    `{"schedule_minutes": 1}`,
		"too many models": `{"models": ["1","2","3","4","5","6","7","8","9","10","11"]}`,
		"bad active":      `{"active_test": "unknown"}`,
		"bad test list":   `{"schedule_tests": ["candy","unknown"]}`,
		"bad fingerprint": `{"fingerprint_concurrency": 4}`,
		"bad pelican":     `{"pelican_variant": "fancy"}`,
	}
	for name, raw := range cases {
		if _, err := Parse([]byte(raw)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestParseMultiTestConfig(t *testing.T) {
	cfg, err := Parse([]byte(`{
		"schedule_tests": [" Candy ", "fingerprint", "candy", "pelican"],
		"active_test": "FINGERPRINT",
		"fingerprint_concurrency": 2,
		"pelican_variant": "all-in-one"
	}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.ScheduleTests) != 3 || cfg.ScheduleTests[0] != TestCandy || cfg.ScheduleTests[1] != TestFingerprint || cfg.ScheduleTests[2] != TestPelican {
		t.Fatalf("unexpected schedule tests: %#v", cfg.ScheduleTests)
	}
	if cfg.ActiveTest != TestFingerprint || cfg.FingerprintConcurrency != 2 || cfg.PelicanVariant != "all-in-one" {
		t.Fatalf("unexpected multi-test config: %#v", cfg)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	cfg, err := Parse([]byte(`{"models":["gpt-5.5"],"runs":1,"concurrency":1,"schedule_minutes":5}`))
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(cfg.JSON(), &decoded); err != nil {
		t.Fatalf("normalized JSON invalid: %v", err)
	}
	if decoded["runs"].(float64) != 1 {
		t.Fatalf("unexpected runs: %#v", decoded["runs"])
	}
}
