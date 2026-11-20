package guard

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/codexguard/sub2api-codex-guard/internal/config"
	pluginv1 "github.com/codexguard/sub2api-codex-guard/internal/pluginapi/v1"
)

const (
	candyMaxRuns  = 10
	answerTimeout = 10 * time.Minute
)

// candyPrompt 与 haowang02/cpa-plugin-codex-candy-eval 的原始糖果题一致（MIT）。
const candyPrompt = `不使用任何外部工具回答以下问题：

在一个黑色的袋子里放有三种口味的糖果，每种糖果有两种不同的形状（圆形和五角星形，不同的形状靠手感可以分辨）。现已知不同口味的糖和不同形状的数量统计如下表。参赛者需要在活动前决定摸出的糖果数目，那么，最少取出多少个糖果才能保证手中同时拥有不同形状的苹果味和桃子味的糖？（同时手中有圆形苹果味匹配五角星桃子味糖果，或者有圆形桃子味匹配五角星苹果味糖果都满足要求）

        苹果味  桃子味  西瓜味
圆形       7      9      8
五角星形   7      6      4
`

// candyResponsesURL 可在测试中替换。
var candyResponsesURL = "https://chatgpt.com/backend-api/codex/responses"

// runCandy 让一个账号对糖果题作答一次并评分。
func (e *Engine) runCandy(ctx context.Context, cfg config.Config, identity *pluginv1.ResolveOutboundIdentityResponse, account AccountView, model string, run int) Result {
	started := time.Now()
	result := Result{
		AccountID: account.ID, AccountName: account.Name, Model: model, Run: run,
		Status: "failed", Time: started.UTC().Format(time.RFC3339),
	}
	answer, err := e.askResponses(ctx, identity, model, cfg.Effort, "", candyPrompt, answerTimeout)
	result.DurationMS = time.Since(started).Milliseconds()
	if err != nil {
		result.Error = truncate(err.Error(), 500)
		return result
	}
	result.Answer = truncate(answer, 1000)
	if hasStandalone21(answer) {
		result.Status = "correct"
	} else {
		result.Status = "wrong"
	}
	return result
}

// askResponses 向 Codex Responses 接口发送一轮最小请求，返回模型输出文本。
// instructions 非空时作为系统指令下发；timeout 控制单次请求时长。
func (e *Engine) askResponses(ctx context.Context, identity *pluginv1.ResolveOutboundIdentityResponse, model, effort, instructions, prompt string, timeout time.Duration) (string, error) {
	payload := map[string]any{
		"model":  model,
		"stream": true,
		"store":  false,
		"input": []map[string]any{
			{"role": "user", "content": prompt},
		},
	}
	if effort != "" && effort != "none" {
		payload["reasoning"] = map[string]any{"effort": effort}
	}
	if instructions != "" {
		payload["instructions"] = instructions
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, candyResponsesURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	for key, values := range identity.GetHeaders() {
		if values != nil {
			request.Header[key] = append([]string(nil), values.GetValues()...)
		}
	}
	request.Header.Del("Content-Length")
	request.Header.Set("Authorization", "Bearer "+identity.GetToken())
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("OpenAI-Beta", "responses=experimental")
	request.Host = "chatgpt.com"
	request.ContentLength = int64(len(body))
	request.GetBody = nil

	client, err := e.clients.client(identity.GetProxyUrl())
	if err != nil {
		return "", fmt.Errorf("代理配置无效：%w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		if requestCtx.Err() != nil {
			return "", fmt.Errorf("请求超时或被取消：%w", requestCtx.Err())
		}
		return "", fmt.Errorf("请求上游失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 8*1024))
		return "", fmt.Errorf("上游返回 HTTP %d：%s", response.StatusCode, truncate(strings.TrimSpace(string(raw)), 500))
	}
	return parseResponseStream(response.Body)
}

// parseResponseStream 解析 Codex Responses 的 SSE 事件，返回模型输出文本。
func parseResponseStream(reader io.Reader) (string, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var delta strings.Builder
	var doneText string
	var failure string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var event struct {
			Type     string          `json:"type"`
			Delta    string          `json:"delta"`
			Text     string          `json:"text"`
			Message  string          `json:"message"`
			Response json.RawMessage `json:"response"`
		}
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			continue
		}
		switch event.Type {
		case "response.output_text.delta":
			delta.WriteString(event.Delta)
		case "response.output_text.done":
			if event.Text != "" {
				doneText = event.Text
			}
		case "response.completed":
			if delta.Len() == 0 && doneText == "" {
				doneText = extractResponseText(event.Response)
			}
		case "response.failed", "response.incomplete":
			failure = extractResponseError(event.Response)
			if failure == "" {
				failure = event.Type
			}
		case "error":
			failure = event.Message
			if failure == "" {
				failure = "上游返回 error 事件"
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("读取上游响应失败：%w", err)
	}
	text := delta.String()
	if strings.TrimSpace(text) == "" {
		text = doneText
	}
	if strings.TrimSpace(text) == "" && failure != "" {
		return "", fmt.Errorf("上游检测失败：%s", truncate(failure, 500))
	}
	return text, nil
}

func extractResponseText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var response struct {
		Output []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if json.Unmarshal(raw, &response) != nil {
		return ""
	}
	var builder strings.Builder
	for _, item := range response.Output {
		for _, content := range item.Content {
			builder.WriteString(content.Text)
		}
	}
	return builder.String()
}

func extractResponseError(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var response struct {
		Error struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &response) != nil {
		return ""
	}
	if response.Error.Message != "" {
		return response.Error.Message
	}
	return response.Error.Code
}

// hasStandalone21 判断文本中是否出现独立的 "21"（前后都不是数字）。
func hasStandalone21(text string) bool {
	isDigit := func(i int) bool { return i >= 0 && i < len(text) && text[i] >= '0' && text[i] <= '9' }
	for i := 0; i+1 < len(text); i++ {
		if text[i] == '2' && text[i+1] == '1' && !isDigit(i-1) && !isDigit(i+2) {
			return true
		}
	}
	return false
}

func truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "…"
}
