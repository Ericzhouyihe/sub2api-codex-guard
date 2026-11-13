package guard

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/codexguard/sub2api-codex-guard/internal/config"
	pluginv1 "github.com/codexguard/sub2api-codex-guard/internal/pluginapi/v1"
)

func TestHasStandalone21(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"答案是 21。", true},
		{"21", true},
		{"最少取 21 个糖果", true},
		{"答案是 121", false},
		{"答案是 210", false},
		{"答案是 22", false},
		{"答案是 20", false},
		{"答案是 2 1", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := hasStandalone21(tc.text); got != tc.want {
			t.Errorf("hasStandalone21(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}

func TestParseResponseStream(t *testing.T) {
	body := strings.Join([]string{
		"event: response.output_text.delta",
		`data: {"type":"response.output_text.delta","delta":"答案是"}`,
		"",
		"event: response.output_text.delta",
		`data: {"type":"response.output_text.delta","delta":" 21。"}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed","response":{"output":[]}}`,
		"",
		"data: [DONE]",
	}, "\n")
	text, err := parseResponseStream(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parseResponseStream: %v", err)
	}
	if text != "答案是 21。" {
		t.Fatalf("text = %q", text)
	}
}

func TestParseResponseStreamFailure(t *testing.T) {
	body := `data: {"type":"response.failed","response":{"error":{"message":"quota exceeded"}}}`
	if _, err := parseResponseStream(strings.NewReader(body)); err == nil {
		t.Fatal("expected error")
	}
}

func TestRunCandyCorrect(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content-type = %q", r.Header.Get("Content-Type"))
		}
		raw, _ := io.ReadAll(r.Body)
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Errorf("invalid payload: %v", err)
		}
		if payload["model"] != "gpt-5.5" {
			t.Errorf("model = %v", payload["model"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"最少取 21 个\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{}}\n\n"))
	}))
	defer server.Close()
	old := candyResponsesURL
	candyResponsesURL = server.URL
	defer func() { candyResponsesURL = old }()

	engine := New()
	defer engine.Close()
	identity := &pluginv1.ResolveOutboundIdentityResponse{Found: true, Token: "secret-token"}
	result := engine.runCandy(context.Background(), config.Default(), identity, AccountView{ID: 1, Name: "测试账号"}, "gpt-5.5", 1)
	if result.Status != "correct" {
		t.Fatalf("status = %q, error = %q, answer = %q", result.Status, result.Error, result.Answer)
	}
	if gotAuth != "Bearer secret-token" {
		t.Fatalf("authorization = %q", gotAuth)
	}
}

func TestRunCandyHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer server.Close()
	old := candyResponsesURL
	candyResponsesURL = server.URL
	defer func() { candyResponsesURL = old }()

	engine := New()
	defer engine.Close()
	identity := &pluginv1.ResolveOutboundIdentityResponse{Found: true, Token: "t"}
	result := engine.runCandy(context.Background(), config.Default(), identity, AccountView{ID: 1}, "gpt-5.5", 1)
	if result.Status != "failed" || !strings.Contains(result.Error, "502") {
		t.Fatalf("unexpected result: %#v", result)
	}
}
