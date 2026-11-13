package guard

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/codexguard/sub2api-codex-guard/internal/config"
	pluginv1 "github.com/codexguard/sub2api-codex-guard/internal/pluginapi/v1"
)

// 每次指纹检测随机抽取 3 道数字挑战；同一账号内最多 3 题并发。
const (
	fingerprintTarget            = 3
	fingerprintGlobalConcurrency = 6
	fingerprintMaxText           = 64000
	challengeTimeout             = 3 * time.Minute
)

type fingerprintChallenge struct {
	Instructions  string `json:"instructions,omitempty"`
	Prompt        string `json:"prompt"`
	ExpectedCount int    `json:"expected_count"`
}

//go:embed data/lm-detector/enrollment-suite.json
var fingerprintSuiteJSON []byte

// fingerprintSuite 是参考模型采集答案时使用的 12 个提示环境，每个环境 3 道题。
var fingerprintSuite = func() []fingerprintChallenge {
	var suite []fingerprintChallenge
	for _, c := range mustDecode[[]struct {
		System        string `json:"system"`
		UserPrefix    string `json:"user_prefix"`
		Prompt        string `json:"prompt"`
		ExpectedCount int    `json:"expected_count"`
	}](fingerprintSuiteJSON) {
		prompt := c.Prompt
		if c.UserPrefix != "" {
			prompt = c.UserPrefix + "\n\nFinal task:\n" + c.Prompt
		}
		suite = append(suite, fingerprintChallenge{c.System, prompt, c.ExpectedCount})
	}
	return suite
}()

func fingerprintChallenges() []fingerprintChallenge {
	challenges := make([]fingerprintChallenge, fingerprintTarget)
	for i, j := range rand.Perm(len(fingerprintSuite))[:fingerprintTarget] {
		challenges[i] = fingerprintSuite[j]
	}
	return challenges
}

// fingerprintAliases 把待测模型映射到参考库里登记的候选模型。
var fingerprintAliases = map[string]string{"gpt-6.1-sol": "gpt-6-astra", "deepseek-flash": "deepseek-v4.1-flash-1017"}

func fingerprintModelID(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	model = model[strings.LastIndex(model, "/")+1:]
	if alias, ok := fingerprintAliases[model]; ok {
		return alias
	}
	return model
}

// FingerprintSample 是一道数字挑战的作答记录。
type FingerprintSample struct {
	Instructions  string `json:"instructions,omitempty"`
	Prompt        string `json:"prompt"`
	ExpectedCount int    `json:"expected_count"`
	Attempts      int    `json:"attempts,omitempty"`
	Text          string `json:"text,omitempty"`
	Error         string `json:"error,omitempty"`
	Parsed        int    `json:"parsed_numbers"`
	Valid         bool   `json:"accepted"`
}

// runFingerprint 在一个账号上跑 3 道数字挑战并做模型归因。
func (e *Engine) runFingerprint(ctx context.Context, cfg config.Config, identity *pluginv1.ResolveOutboundIdentityResponse, account AccountView, model string) Result {
	started := time.Now()
	result := Result{
		Test: config.TestFingerprint, AccountID: account.ID, AccountName: account.Name, Model: model,
		Status: "failed", Time: started.UTC().Format(time.RFC3339), DetectorRevision: detectorRevision,
	}
	challenges := fingerprintChallenges()
	concurrency := cfg.FingerprintConcurrency
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > fingerprintTarget {
		concurrency = fingerprintTarget
	}
	type event struct {
		index  int
		sample FingerprintSample
	}
	jobs := make(chan int, fingerprintTarget)
	events := make(chan event, fingerprintTarget)
	for i := range challenges {
		jobs <- i
	}
	close(jobs)
	var workers sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				if ctx.Err() != nil {
					return
				}
				sample := e.collectFingerprintSample(ctx, identity, model, challenges[index])
				events <- event{index, sample}
			}
		}()
	}
	go func() {
		workers.Wait()
		close(events)
	}()
	samples := make([]*FingerprintSample, fingerprintTarget)
	for ev := range events {
		sample := ev.sample
		samples[ev.index] = &sample
	}
	var outputs []fingerprintOutput
	for _, sample := range samples {
		if sample == nil {
			continue
		}
		result.Samples = append(result.Samples, *sample)
		outputs = append(outputs, fingerprintOutput{sample.Text, sample.ExpectedCount})
	}
	attribution, err := analyzeFingerprint(outputs)
	result.DurationMS = time.Since(started).Milliseconds()
	switch {
	case err == nil:
		result.Attribution = &attribution
		if attribution.Used < fingerprintTarget {
			result.Status = "partial"
		} else {
			result.Status = "completed"
		}
	case len(result.Samples) > 0 && result.Samples[len(result.Samples)-1].Error != "":
		result.Status, result.Error = "failed", result.Samples[len(result.Samples)-1].Error
	default:
		result.Status, result.Error = "failed", truncate(err.Error(), 500)
	}
	return result
}

// collectFingerprintSample 反复请求一道挑战，直到答案可以评分；保留最后一次答案。
func (e *Engine) collectFingerprintSample(ctx context.Context, identity *pluginv1.ResolveOutboundIdentityResponse, model string, challenge fingerprintChallenge) FingerprintSample {
	sample := FingerprintSample{
		Instructions: challenge.Instructions, Prompt: challenge.Prompt, ExpectedCount: challenge.ExpectedCount,
	}
	answer, attempts, err := e.askCodex(ctx, identity, model, "", challenge.Instructions, challenge.Prompt, challengeTimeout, func(answer string) error {
		text, parsed, err := fingerprintText(answer, challenge.ExpectedCount)
		sample.Text, sample.Parsed = text, parsed
		return err
	})
	_ = answer
	sample.Attempts = attempts
	sample.Valid = err == nil
	if err != nil {
		sample.Error = truncate(err.Error(), 500)
	}
	return sample
}

// fingerprintText 返回参与打分的文本和数字个数；答案过长会被拒绝。
func fingerprintText(answer string, expected int) (text string, parsed int, err error) {
	if len(answer) > fingerprintMaxText {
		return "", 0, errors.New("回答过长")
	}
	text, numbers := answer, parseNumbers(answer)
	if len(numbers) > expected {
		numbers = numbers[:expected]
		values := make([]string, len(numbers))
		for i, n := range numbers {
			values[i] = strconv.Itoa(n)
		}
		text = strings.Join(values, ", ")
	}
	if len(numbers) < minimumNumbers(expected) {
		err = errors.New("有效数字不足")
	}
	return text, len(numbers), err
}

// retryableError 标记 accept 拒绝的答案，重发请求可能得到可评分答案。
type retryableError struct{ error }

func (e *Engine) askCodex(ctx context.Context, identity *pluginv1.ResolveOutboundIdentityResponse, model, effort, instructions, prompt string, limit time.Duration, accept func(string) error) (out string, attempts int, err error) {
	for delay := 2 * time.Second; ; delay *= 2 {
		if err = ctx.Err(); err != nil {
			return
		}
		attempts++
		out, err = e.askResponses(ctx, identity, model, effort, instructions, prompt, limit)
		if err == nil && accept != nil {
			if acceptErr := accept(out); acceptErr != nil {
				err = retryableError{acceptErr}
			}
		}
		if err == nil || attempts >= 3 {
			return
		}
		if _, ok := err.(retryableError); !ok {
			return
		}
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return out, attempts, ctx.Err()
		}
	}
}

func mustDecode[T any](data []byte) T {
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		panic(err)
	}
	return value
}
