package guard

import (
	"context"
	"strings"
	"time"

	"github.com/codexguard/sub2api-codex-guard/internal/config"
	pluginv1 "github.com/codexguard/sub2api-codex-guard/internal/pluginapi/v1"
)

const siwcTimeout = time.Minute

// runSIWC 是 Sub2API 适配版的通道可用性探测：发一条最小 Responses 请求，
// 判断该 OpenAI OAuth 账号当前能否完成一次正常回答。Sub2API 没有 CPA 的 SIWC
// 路由/元数据概念，因此无法复刻原版的 subscription-sharing 状态机。
func (e *Engine) runSIWC(ctx context.Context, identity *pluginv1.ResolveOutboundIdentityResponse, account AccountView, model string) Result {
	started := time.Now()
	result := Result{
		Test: config.TestSIWC, AccountID: account.ID, AccountName: account.Name, Model: model,
		Status: "failed", Time: started.UTC().Format(time.RFC3339),
	}
	answer, err := e.askResponses(ctx, identity, model, "", "", "Reply with OK.", siwcTimeout)
	result.DurationMS = time.Since(started).Milliseconds()
	if err != nil {
		result.Error = truncate(err.Error(), 500)
		switch {
		case strings.Contains(result.Error, "HTTP 401"):
			result.Status, result.HTTPStatus = "auth_error", 401
		case strings.Contains(result.Error, "HTTP 429"):
			result.Status, result.HTTPStatus = "limited", 429
		}
		return result
	}
	if strings.TrimSpace(answer) == "" {
		result.Error = "请求完成但没有返回文本"
		return result
	}
	result.Status = "available"
	result.Answer = truncate(answer, 200)
	return result
}
