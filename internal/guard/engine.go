// Package guard 实现 Sub2API openai.oauth.outbound_transport.v1 插件运行时。
package guard

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	hcplugin "github.com/hashicorp/go-plugin"

	"github.com/codexguard/sub2api-codex-guard/internal/config"
	pluginv1 "github.com/codexguard/sub2api-codex-guard/internal/pluginapi/v1"
)

const (
	// PluginID 必须与 manifest.source.json 中的 id 完全一致，宿主启动时校验。
	PluginID = "io.github.codexguard.sub2api"
	// Capability 是当前宿主唯一开放的插件能力。
	Capability = "openai.oauth.outbound_transport.v1"
)

// Version 由打包器通过 -ldflags 注入；与清单 version 必须一致。
var Version = "0.2.0"

// Engine 是插件进程内唯一的传输实现。
type Engine struct {
	pluginv1.UnimplementedTransportPluginServer

	mu        sync.RWMutex
	cfg       config.Config
	host      *hostService
	runner    *Runner
	scheduler *Scheduler
	clients   *clientPool
	closed    atomic.Bool

	logMu sync.Mutex
	logs  []logEntry
}

// New 创建使用默认配置的引擎；宿主随后通过 ApplyConfig 下发已保存配置。
func New() *Engine {
	e := &Engine{
		cfg:     config.Default(),
		host:    &hostService{},
		clients: newClientPool(),
	}
	e.runner = newRunner(e)
	e.scheduler = newScheduler(e)
	e.scheduler.Configure(e.cfg)
	e.scheduler.Start()
	e.log("info", "Codex 降智测试插件已启动")
	return e
}

// Close 停止定时器与进行中的检测，释放连接。
func (e *Engine) Close() {
	if e.closed.Swap(true) {
		return
	}
	e.scheduler.Stop()
	e.runner.Stop()
	e.host.close()
	e.clients.closeIdle()
}

func (e *Engine) config() config.Config {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.cfg
}

// SetHostBroker 实现 pluginv1.HostBrokerReceiver。
func (e *Engine) SetHostBroker(broker *hcplugin.GRPCBroker) {
	e.host.SetBroker(broker)
}

func (e *Engine) GetInfo(context.Context, *pluginv1.GetInfoRequest) (*pluginv1.GetInfoResponse, error) {
	return &pluginv1.GetInfoResponse{
		PluginId:            PluginID,
		PluginVersion:       Version,
		ProtocolVersion:     pluginv1.ProtocolVersion,
		TransportApiVersion: pluginv1.TransportAPIVersion,
		Capabilities:        []string{Capability},
	}, nil
}

// Health 只读取内存状态，不访问网络，供宿主健康检查和 UI 状态轮询使用。
func (e *Engine) Health(context.Context, *pluginv1.HealthRequest) (*pluginv1.HealthResponse, error) {
	if e.closed.Load() {
		return &pluginv1.HealthResponse{Healthy: false, Message: "插件正在停止"}, nil
	}
	if e.runner.Running() {
		return &pluginv1.HealthResponse{Healthy: true, Message: "运行中（正在检测）", StatusJson: e.statusJSON()}, nil
	}
	return &pluginv1.HealthResponse{Healthy: true, Message: "运行中", StatusJson: e.statusJSON()}, nil
}

func (e *Engine) ValidateConfig(_ context.Context, request *pluginv1.ValidateConfigRequest) (*pluginv1.ValidateConfigResponse, error) {
	cfg, err := config.Parse(request.GetConfigJson())
	if err != nil {
		return &pluginv1.ValidateConfigResponse{Valid: false, Message: err.Error()}, nil
	}
	return &pluginv1.ValidateConfigResponse{Valid: true, Message: "配置有效", NormalizedConfigJson: cfg.JSON()}, nil
}

// ApplyConfig 原子切换配置；失败时保留旧配置。
func (e *Engine) ApplyConfig(_ context.Context, request *pluginv1.ApplyConfigRequest) (*pluginv1.ApplyConfigResponse, error) {
	cfg, err := config.Parse(request.GetConfigJson())
	if err != nil {
		return &pluginv1.ApplyConfigResponse{Applied: false, Message: err.Error()}, nil
	}
	e.mu.Lock()
	e.cfg = cfg
	e.mu.Unlock()
	e.scheduler.Configure(cfg)
	e.log("info", fmt.Sprintf("配置已应用：%d 个模型，每账号 %d 次，定时检测 %s",
		len(cfg.Models), cfg.Runs, boolLabel(cfg.ScheduleEnabled)))
	return &pluginv1.ApplyConfigResponse{Applied: true, Message: "配置已应用"}, nil
}

// TestConfig 异步启动指定检测，立即返回，结果通过 Health.status_json 轮询。
func (e *Engine) TestConfig(_ context.Context, request *pluginv1.TestConfigRequest) (*pluginv1.TestConfigResponse, error) {
	cfg := e.config()
	if raw := request.GetConfigJson(); len(raw) > 0 {
		parsed, err := config.Parse(raw)
		if err != nil {
			return &pluginv1.TestConfigResponse{Success: false, Message: err.Error(), StatusJson: e.statusJSON()}, nil
		}
		cfg = parsed
	}
	test := cfg.ActiveTest
	if test == "" {
		test = config.TestCandy
	}
	if err := e.runner.Start(cfg, "manual", []string{test}); err != nil {
		if errors.Is(err, errAlreadyRunning) {
			return &pluginv1.TestConfigResponse{Success: true, Message: "检测任务已在运行中，请等待本轮结束", StatusJson: e.statusJSON()}, nil
		}
		return &pluginv1.TestConfigResponse{Success: false, Message: err.Error(), StatusJson: e.statusJSON()}, nil
	}
	return &pluginv1.TestConfigResponse{Success: true, Message: testLabel(test) + "检测已开始，结果会自动刷新", StatusJson: e.statusJSON()}, nil
}

// InitHostServices 拨号回宿主提供的 HostService，并恢复上一次检测结果。
func (e *Engine) InitHostServices(ctx context.Context, request *pluginv1.InitHostServicesRequest) (*pluginv1.InitHostServicesResponse, error) {
	if err := e.host.init(ctx, request.GetHostServiceId(), request.GetHostServiceApiVersion()); err != nil {
		return &pluginv1.InitHostServicesResponse{Ready: false, Message: err.Error()}, nil
	}
	go e.runner.LoadState()
	return &pluginv1.InitHostServicesResponse{Ready: true, Message: "宿主服务已连接"}, nil
}

func boolLabel(value bool) string {
	if value {
		return "已开启"
	}
	return "已关闭"
}
