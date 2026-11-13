package guard

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc"

	pluginv1 "github.com/codexguard/sub2api-codex-guard/internal/pluginapi/v1"
)

type fakeForwardStream struct {
	grpc.ServerStream
	ctx       context.Context
	requests  []*pluginv1.ForwardRequest
	responses []*pluginv1.ForwardResponse
	index     int
}

func (f *fakeForwardStream) Context() context.Context { return f.ctx }

func (f *fakeForwardStream) Send(response *pluginv1.ForwardResponse) error {
	f.responses = append(f.responses, response)
	return nil
}

func (f *fakeForwardStream) Recv() (*pluginv1.ForwardRequest, error) {
	if f.index >= len(f.requests) {
		return nil, io.EOF
	}
	request := f.requests[f.index]
	f.index++
	return request, nil
}

func TestForwardPassthrough(t *testing.T) {
	var gotBody, gotHeader, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		gotHeader = r.Header.Get("X-Test")
		gotPath = r.URL.Path
		w.Header().Set("X-Upstream", "yes")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello from upstream"))
	}))
	defer server.Close()

	body := "payload"
	stream := &fakeForwardStream{
		ctx: context.Background(),
		requests: []*pluginv1.ForwardRequest{
			{Frame: &pluginv1.ForwardRequest_Start{Start: &pluginv1.ForwardRequestStart{
				RequestId: "req-1", Method: http.MethodPost,
				Url: server.URL + "/backend-api/codex/responses", Host: "chatgpt.com",
				Headers: map[string]*pluginv1.HeaderValues{
					"X-Test": {Values: []string{"abc"}},
				},
				AccountId: 9, Platform: "openai", AccountType: "oauth",
				ContentLength: int64(len(body)), HasBody: true,
			}}},
			{Frame: &pluginv1.ForwardRequest_BodyChunk{BodyChunk: []byte("pay")}},
			{Frame: &pluginv1.ForwardRequest_BodyChunk{BodyChunk: []byte("load")}},
			{Frame: &pluginv1.ForwardRequest_BodyEnd{BodyEnd: true}},
		},
	}

	engine := New()
	defer engine.Close()
	if err := engine.Forward(stream); err != nil {
		t.Fatalf("Forward: %v", err)
	}
	if gotBody != body {
		t.Fatalf("upstream body = %q", gotBody)
	}
	if gotHeader != "abc" {
		t.Fatalf("upstream header = %q", gotHeader)
	}
	if gotPath != "/backend-api/codex/responses" {
		t.Fatalf("path = %q", gotPath)
	}

	var start *pluginv1.ForwardResponseStart
	var responseBody strings.Builder
	var end *pluginv1.ForwardResponseEnd
	for _, frame := range stream.responses {
		switch value := frame.GetFrame().(type) {
		case *pluginv1.ForwardResponse_Start:
			start = value.Start
		case *pluginv1.ForwardResponse_BodyChunk:
			responseBody.Write(value.BodyChunk)
		case *pluginv1.ForwardResponse_End:
			end = value.End
		case *pluginv1.ForwardResponse_Error:
			t.Fatalf("unexpected error frame: %#v", value.Error)
		}
	}
	if start == nil || start.GetStatusCode() != http.StatusOK {
		t.Fatalf("missing/invalid start frame: %#v", start)
	}
	if start.GetHeaders()["X-Upstream"] == nil {
		t.Fatalf("response headers not forwarded: %#v", start.GetHeaders())
	}
	if responseBody.String() != "hello from upstream" {
		t.Fatalf("response body = %q", responseBody.String())
	}
	if end == nil || end.GetBytesReceived() != int64(len("hello from upstream")) {
		t.Fatalf("end frame = %#v", end)
	}
}

func TestForwardRejectsInvalidURL(t *testing.T) {
	stream := &fakeForwardStream{
		ctx: context.Background(),
		requests: []*pluginv1.ForwardRequest{
			{Frame: &pluginv1.ForwardRequest_Start{Start: &pluginv1.ForwardRequestStart{Method: "GET", Url: "ftp://example.com/x"}}},
		},
	}
	engine := New()
	defer engine.Close()
	if err := engine.Forward(stream); err != nil {
		t.Fatalf("Forward error = %v", err)
	}
	if len(stream.responses) != 1 {
		t.Fatalf("responses = %#v", stream.responses)
	}
	if stream.responses[0].GetError() == nil {
		t.Fatalf("expected error frame, got %#v", stream.responses[0])
	}
}

func TestForwardStopsWhenEngineClosed(t *testing.T) {
	engine := New()
	engine.Close()
	stream := &fakeForwardStream{ctx: context.Background()}
	if err := engine.Forward(stream); err != nil {
		t.Fatalf("Forward error = %v", err)
	}
	if len(stream.responses) != 1 || stream.responses[0].GetError() == nil {
		t.Fatalf("expected plugin_stopped error, got %#v", stream.responses)
	}
}
