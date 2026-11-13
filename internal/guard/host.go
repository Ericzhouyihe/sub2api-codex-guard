package guard

import (
	"context"
	"errors"
	"fmt"
	"sync"

	hcplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	pluginv1 "github.com/codexguard/sub2api-codex-guard/internal/pluginapi/v1"
)

// hostService 持有宿主通过 go-plugin broker 反向暴露的 HostService 连接。
// 它只在 InitHostServices 成功后被使用，所有 RPC 都有超时。
type hostService struct {
	mu     sync.Mutex
	broker *hcplugin.GRPCBroker
	conn   *grpc.ClientConn
	client pluginv1.HostServiceClient
}

// SetBroker 实现 pluginv1.HostBrokerReceiver，由 runtime.go 在插件启动时调用。
func (h *hostService) SetBroker(broker *hcplugin.GRPCBroker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.broker = broker
}

func (h *hostService) init(ctx context.Context, hostServiceID uint32, apiVersion uint32) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.client != nil {
		return nil
	}
	if h.broker == nil {
		return errors.New("宿主服务连接尚未准备好")
	}
	if apiVersion < 1 {
		return fmt.Errorf("宿主服务版本无效：%d", apiVersion)
	}
	conn, err := h.broker.Dial(hostServiceID)
	if err != nil {
		return fmt.Errorf("连接宿主服务失败：%w", err)
	}
	h.conn = conn
	h.client = pluginv1.NewHostServiceClient(conn)
	return nil
}

func (h *hostService) get() pluginv1.HostServiceClient {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.client
}

func (h *hostService) close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conn != nil {
		_ = h.conn.Close()
		h.conn = nil
		h.client = nil
	}
}
