// Command smoketest 启动本地插件二进制，用真实的 go-plugin 客户端完成
// 握手、GetInfo、配置校验/应用和一次 Forward 透传，验证插件进程可被宿主加载。
//
//	go run ./tools/smoketest -binary .\dist\windows-amd64\codex-guard.exe
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sync"
	"time"

	hcplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	pluginv1 "github.com/codexguard/sub2api-codex-guard/internal/pluginapi/v1"
)

// fakeHost 模拟宿主反向 HostService：一个可调度账号、无可解析凭据、内存 KV。
type fakeHost struct {
	pluginv1.UnimplementedHostServiceServer
	mu   sync.Mutex
	data map[string][]byte
}

func newFakeHost() *fakeHost {
	return &fakeHost{data: map[string][]byte{}}
}

func (h *fakeHost) KVGet(_ context.Context, request *pluginv1.KVGetRequest) (*pluginv1.KVGetResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	value, ok := h.data[request.GetNamespace()+"/"+request.GetKey()]
	if !ok {
		return &pluginv1.KVGetResponse{Found: false}, nil
	}
	return &pluginv1.KVGetResponse{Found: true, Value: value}, nil
}

func (h *fakeHost) KVSet(_ context.Context, request *pluginv1.KVSetRequest) (*pluginv1.KVSetResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.data[request.GetNamespace()+"/"+request.GetKey()] = append([]byte(nil), request.GetValue()...)
	return &pluginv1.KVSetResponse{}, nil
}

func (h *fakeHost) KVDelete(_ context.Context, request *pluginv1.KVDeleteRequest) (*pluginv1.KVDeleteResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.data, request.GetNamespace()+"/"+request.GetKey())
	return &pluginv1.KVDeleteResponse{}, nil
}

func (h *fakeHost) KVList(_ context.Context, request *pluginv1.KVListRequest) (*pluginv1.KVListResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	prefix := request.GetNamespace() + "/" + request.GetKeyPrefix()
	keys := []string{}
	for key := range h.data {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			keys = append(keys, key[len(request.GetNamespace())+1:])
		}
	}
	return &pluginv1.KVListResponse{Keys: keys}, nil
}

func (h *fakeHost) ListAccounts(context.Context, *pluginv1.ListAccountsRequest) (*pluginv1.ListAccountsResponse, error) {
	return &pluginv1.ListAccountsResponse{
		AccountIds: []int64{1},
		Accounts: []*pluginv1.AccountInfo{{
			Id: 1, Platform: "openai", AccountType: "oauth", Name: "smoke-account", Status: "active", Schedulable: true,
		}},
	}, nil
}

func (h *fakeHost) ResolveOutboundIdentity(context.Context, *pluginv1.ResolveOutboundIdentityRequest) (*pluginv1.ResolveOutboundIdentityResponse, error) {
	return &pluginv1.ResolveOutboundIdentityResponse{Found: false}, nil
}

func main() {
	binary := flag.String("binary", "", "插件二进制路径")
	flag.Parse()
	if *binary == "" {
		fmt.Fprintln(os.Stderr, "-binary is required")
		os.Exit(2)
	}
	if err := run(*binary); err != nil {
		fmt.Fprintln(os.Stderr, "smoketest failed:", err)
		os.Exit(1)
	}
	fmt.Println("smoketest passed")
}

func run(binary string) error {
	client := hcplugin.NewClient(&hcplugin.ClientConfig{
		HandshakeConfig:  pluginv1.HandshakeConfig,
		Plugins:          pluginv1.ClientPluginMap(),
		Cmd:              exec.Command(binary),
		AllowedProtocols: []hcplugin.Protocol{hcplugin.ProtocolGRPC},
	})
	defer client.Kill()
	rpcClient, err := client.Client()
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	raw, err := rpcClient.Dispense(pluginv1.TransportPluginName)
	if err != nil {
		return fmt.Errorf("dispense: %w", err)
	}
	transport, ok := raw.(*pluginv1.TransportClient)
	if !ok {
		return fmt.Errorf("unexpected transport type %T", raw)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	info, err := transport.GetInfo(ctx, &pluginv1.GetInfoRequest{})
	if err != nil {
		return fmt.Errorf("GetInfo: %w", err)
	}
	if info.GetPluginId() != "io.github.codexguard.sub2api" || info.GetPluginVersion() != "0.2.0" {
		return fmt.Errorf("unexpected info: %+v", info)
	}
	valid, err := transport.ValidateConfig(ctx, &pluginv1.ValidateConfigRequest{ConfigJson: []byte(`{"models":["gpt-5.5"],"runs":2}`)})
	if err != nil || !valid.GetValid() {
		return fmt.Errorf("ValidateConfig: %v %+v", err, valid)
	}
	applied, err := transport.ApplyConfig(ctx, &pluginv1.ApplyConfigRequest{ConfigJson: valid.GetNormalizedConfigJson()})
	if err != nil || !applied.GetApplied() {
		return fmt.Errorf("ApplyConfig: %v %+v", err, applied)
	}
	health, err := transport.Health(ctx, &pluginv1.HealthRequest{})
	if err != nil || !health.GetHealthy() || health.GetStatusJson() == "" {
		return fmt.Errorf("Health: %v %+v", err, health)
	}

	// 验证宿主反向服务：通过 go-plugin broker 暴露 HostService，让插件拨号回来。
	host := newFakeHost()
	if transport.Broker == nil {
		return fmt.Errorf("go-plugin broker is nil")
	}
	brokerID := transport.Broker.NextId()
	go transport.Broker.AcceptAndServe(brokerID, func(options []grpc.ServerOption) *grpc.Server {
		server := grpc.NewServer(options...)
		pluginv1.RegisterHostServiceServer(server, host)
		return server
	})
	initResp, err := transport.InitHostServices(ctx, &pluginv1.InitHostServicesRequest{
		HostServiceId: brokerID, HostServiceApiVersion: pluginv1.HostServiceAPIVersion,
	})
	if err != nil || !initResp.GetReady() {
		return fmt.Errorf("InitHostServices: %v %+v", err, initResp)
	}
	if _, err := transport.TestConfig(ctx, &pluginv1.TestConfigRequest{}); err != nil {
		return fmt.Errorf("TestConfig: %w", err)
	}
	if err := waitForRun(ctx, transport, "candy", 10*time.Second); err != nil {
		return err
	}
	// 逐一触发其余检测，验证多测试 runner 与状态 JSON 的接线。
	for _, test := range []string{"fingerprint", "pelican", "siwc"} {
		rawConfig := []byte(`{"models":["gpt-6.1-sol"],"active_test":"` + test + `"}`)
		valid, err := transport.ValidateConfig(ctx, &pluginv1.ValidateConfigRequest{ConfigJson: rawConfig})
		if err != nil || !valid.GetValid() {
			return fmt.Errorf("ValidateConfig(%s): %v %+v", test, err, valid)
		}
		if applied, err := transport.ApplyConfig(ctx, &pluginv1.ApplyConfigRequest{ConfigJson: valid.GetNormalizedConfigJson()}); err != nil || !applied.GetApplied() {
			return fmt.Errorf("ApplyConfig(%s): %v %+v", test, err, applied)
		}
		if _, err := transport.TestConfig(ctx, &pluginv1.TestConfigRequest{}); err != nil {
			return fmt.Errorf("TestConfig(%s): %w", test, err)
		}
		if err := waitForRun(ctx, transport, test, 10*time.Second); err != nil {
			return err
		}
	}
	host.mu.Lock()
	persisted := host.data["codex-guard/state"]
	host.mu.Unlock()
	if len(persisted) == 0 {
		return fmt.Errorf("检测结果未持久化到宿主 KV")
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Smoke", "ok")
		_, _ = w.Write([]byte("echo:" + string(body)))
	}))
	defer upstream.Close()

	stream, err := transport.Forward(ctx)
	if err != nil {
		return fmt.Errorf("Forward: %w", err)
	}
	body := "hello"
	if err := stream.Send(&pluginv1.ForwardRequest{Frame: &pluginv1.ForwardRequest_Start{Start: &pluginv1.ForwardRequestStart{
		RequestId: "smoke", Method: http.MethodPost, Url: upstream.URL + "/backend-api/codex/responses",
		Host: "chatgpt.com", ContentLength: int64(len(body)), HasBody: true,
		AccountId: 1, Platform: "openai", AccountType: "oauth",
	}}}); err != nil {
		return fmt.Errorf("send start: %w", err)
	}
	if err := stream.Send(&pluginv1.ForwardRequest{Frame: &pluginv1.ForwardRequest_BodyChunk{BodyChunk: []byte(body)}}); err != nil {
		return fmt.Errorf("send body: %w", err)
	}
	if err := stream.Send(&pluginv1.ForwardRequest{Frame: &pluginv1.ForwardRequest_BodyEnd{BodyEnd: true}}); err != nil {
		return fmt.Errorf("send end: %w", err)
	}
	if err := stream.CloseSend(); err != nil {
		return fmt.Errorf("close send: %w", err)
	}
	var status int32
	var received string
	for {
		frame, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("recv: %w", err)
		}
		switch value := frame.GetFrame().(type) {
		case *pluginv1.ForwardResponse_Start:
			status = value.Start.GetStatusCode()
		case *pluginv1.ForwardResponse_BodyChunk:
			received += string(value.BodyChunk)
		case *pluginv1.ForwardResponse_End:
			if received != "echo:"+body || status != http.StatusOK {
				return fmt.Errorf("bad forward response: status=%d body=%q", status, received)
			}
			return nil
		case *pluginv1.ForwardResponse_Error:
			return fmt.Errorf("forward error: %s", value.Error.GetMessage())
		}
	}
	return fmt.Errorf("forward ended without end frame")
}

func waitForRun(ctx context.Context, transport *pluginv1.TransportClient, test string, timeout time.Duration) error {
	type summary struct {
		Total int `json:"total"`
	}
	type testStatus struct {
		Running bool    `json:"running"`
		Summary summary `json:"summary"`
	}
	type status struct {
		Tests map[string]testStatus `json:"tests"`
		Candy testStatus            `json:"candy"`
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		reply, err := transport.Health(ctx, &pluginv1.HealthRequest{})
		if err != nil {
			return fmt.Errorf("Health while waiting: %w", err)
		}
		var parsed status
		if err := json.Unmarshal([]byte(reply.GetStatusJson()), &parsed); err != nil {
			return fmt.Errorf("parse status_json: %w", err)
		}
		current, ok := parsed.Tests[test]
		if !ok && test == "candy" {
			current, ok = parsed.Candy, true
		}
		if ok && !current.Running && current.Summary.Total > 0 {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("%s 检测在 %s 内未完成", test, timeout)
}
