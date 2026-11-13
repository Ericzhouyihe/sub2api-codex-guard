package guard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/codexguard/sub2api-codex-guard/internal/config"
	pluginv1 "github.com/codexguard/sub2api-codex-guard/internal/pluginapi/v1"
)

var errAlreadyRunning = errors.New("检测任务已在运行中")

var allTests = []string{config.TestCandy, config.TestFingerprint, config.TestPelican, config.TestSIWC}

// Runner 负责账号发现、多测试调度、结果汇总与持久化。
type Runner struct {
	engine *Engine

	mu       sync.Mutex
	running  bool
	stopping bool
	cancel   context.CancelFunc
	accounts []AccountView
	tests    map[string]*TestStatus
}

func newRunner(engine *Engine) *Runner {
	r := &Runner{engine: engine, tests: make(map[string]*TestStatus, len(allTests))}
	for _, test := range allTests {
		r.tests[test] = &TestStatus{Results: []Result{}}
	}
	return r
}

// Status 返回按测试类型分组的状态快照和账号列表。
func (r *Runner) Status() (map[string]TestStatus, []AccountView) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]TestStatus, len(r.tests))
	for name, status := range r.tests {
		copied := *status
		copied.Results = append([]Result(nil), status.Results...)
		if copied.Results == nil {
			copied.Results = []Result{}
		}
		copied.Accounts = append([]AccountView(nil), r.accounts...)
		if copied.Accounts == nil {
			copied.Accounts = []AccountView{}
		}
		out[name] = copied
	}
	return out, append([]AccountView(nil), r.accounts...)
}

// Running 报告是否有任意检测在运行。
func (r *Runner) Running() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

// Start 异步启动一组检测。tests 为空时默认糖果；trigger 为 manual 或 schedule。
func (r *Runner) Start(cfg config.Config, trigger string, tests []string) error {
	if len(tests) == 0 {
		tests = []string{config.TestCandy}
	}
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return errAlreadyRunning
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.running = true
	r.stopping = false
	r.cancel = cancel
	r.accounts = nil
	for _, test := range tests {
		status, ok := r.tests[test]
		if !ok {
			continue
		}
		status.Running = false
		status.Stopping = false
		status.LastError = ""
		status.Summary = Summary{}
		status.Results = []Result{}
	}
	r.mu.Unlock()

	go r.run(ctx, cfg, trigger, tests)
	return nil
}

func (r *Runner) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running || r.cancel == nil {
		return
	}
	r.stopping = true
	for _, status := range r.tests {
		if status.Running {
			status.Stopping = true
		}
	}
	r.cancel()
}

func (r *Runner) setError(test string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if status, ok := r.tests[test]; ok {
		status.LastError = err.Error()
	}
}

func (r *Runner) setRunning(test string, running bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if status, ok := r.tests[test]; ok {
		status.Running = running
		if !running {
			status.Stopping = false
			status.LastRunAt = time.Now().UTC().Format(time.RFC3339)
		}
	}
}

func (r *Runner) addResult(test string, result Result) {
	result.Test = test
	r.mu.Lock()
	defer r.mu.Unlock()
	status, ok := r.tests[test]
	if !ok {
		return
	}
	status.Results = append(status.Results, result)
	limit := maxResults
	if test == config.TestPelican {
		limit = maxPelicanItems
	}
	if len(status.Results) > limit {
		status.Results = append([]Result(nil), status.Results[len(status.Results)-limit:]...)
	}
	status.Summary.add(result.Status)
}

func (r *Runner) snapshotLocked() persistedState {
	state := persistedState{
		Accounts: append([]AccountView(nil), r.accounts...),
		Tests:    make(map[string]persistedTest, len(r.tests)),
	}
	for name, status := range r.tests {
		state.Tests[name] = persistedTest{
			LastRunAt: status.LastRunAt,
			Summary:   status.Summary,
			Results:   append([]Result(nil), status.Results...),
		}
	}
	// 兼容 v0.1.0 的读取路径。
	if candy, ok := state.Tests[config.TestCandy]; ok {
		state.LastRunAt = candy.LastRunAt
		state.Summary = candy.Summary
		state.Results = candy.Results
	}
	return state
}

func (r *Runner) run(ctx context.Context, cfg config.Config, trigger string, tests []string) {
	defer func() {
		r.mu.Lock()
		r.running = false
		r.stopping = false
		r.cancel = nil
		for _, status := range r.tests {
			status.Running = false
			status.Stopping = false
		}
		state := r.snapshotLocked()
		r.mu.Unlock()

		r.persist(state)
		if trigger == "schedule" {
			r.engine.scheduler.AfterScheduledRun()
		}
		r.engine.log("info", "检测任务结束")
	}()

	r.engine.log("info", fmt.Sprintf("开始检测（%s）：%s", triggerLabel(trigger), stringsJoin(tests)))
	host := r.engine.host.get()
	if host == nil {
		for _, test := range tests {
			r.setError(test, errors.New("宿主服务尚未就绪，无法读取账号列表"))
		}
		r.engine.log("warn", "检测失败：宿主服务尚未就绪")
		return
	}

	accounts, identities, err := r.discover(ctx, host, cfg)
	if err != nil {
		if ctx.Err() != nil {
			r.engine.log("info", "检测已取消")
			return
		}
		for _, test := range tests {
			r.setError(test, err)
		}
		r.engine.log("warn", "读取账号失败："+err.Error())
		return
	}
	r.mu.Lock()
	r.accounts = accounts
	r.mu.Unlock()

	for _, test := range tests {
		if ctx.Err() != nil {
			return
		}
		r.setRunning(test, true)
		r.engine.log("info", fmt.Sprintf("开始%s检测", testLabel(test)))
		switch test {
		case config.TestCandy:
			r.runCandyTest(ctx, cfg, accounts, identities)
		case config.TestFingerprint:
			r.runFingerprintTest(ctx, cfg, accounts, identities)
		case config.TestPelican:
			r.runPelicanTest(ctx, cfg, accounts, identities)
		case config.TestSIWC:
			r.runSIWCTest(ctx, cfg, accounts, identities)
		}
		r.setRunning(test, false)
		r.mu.Lock()
		state := r.snapshotLocked()
		r.mu.Unlock()
		r.persist(state)
	}
}

func (r *Runner) runCandyTest(ctx context.Context, cfg config.Config, accounts []AccountView, identities map[int64]*pluginv1.ResolveOutboundIdentityResponse) {
	groups := make(map[int64][]struct {
		model string
		run   int
	})
	order := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		order = append(order, account.ID)
		for _, model := range cfg.Models {
			for run := 1; run <= cfg.Runs; run++ {
				groups[account.ID] = append(groups[account.ID], struct {
					model string
					run   int
				}{model, run})
			}
		}
	}
	r.forEachAccount(ctx, cfg.Concurrency, order, func(account AccountView) {
		identity := identities[account.ID]
		for _, job := range groups[account.ID] {
			if ctx.Err() != nil {
				return
			}
			if identity == nil || !identity.GetFound() {
				r.addResult(config.TestCandy, Result{
					AccountID: account.ID, AccountName: account.Name, Model: job.model, Run: job.run,
					Status: "skipped", Error: "账号不可用或未启用调度", Time: time.Now().UTC().Format(time.RFC3339),
				})
				continue
			}
			r.addResult(config.TestCandy, r.engine.runCandy(ctx, cfg, identity, account, job.model, job.run))
		}
	})
}

func (r *Runner) runFingerprintTest(ctx context.Context, cfg config.Config, accounts []AccountView, identities map[int64]*pluginv1.ResolveOutboundIdentityResponse) {
	model := firstModel(cfg)
	// 每个账号内部会开 FingerprintConcurrency 路并发；账号级并发相应收紧，
	// 保证所有账号同时在途的指纹请求不超过 fingerprintGlobalConcurrency。
	perAccount := cfg.FingerprintConcurrency
	if perAccount < 1 {
		perAccount = 1
	}
	accountConcurrency := fingerprintGlobalConcurrency / perAccount
	if accountConcurrency < 1 {
		accountConcurrency = 1
	}
	if cfg.Concurrency < accountConcurrency {
		accountConcurrency = cfg.Concurrency
	}
	r.forEachAccount(ctx, accountConcurrency, accountIDs(accounts), func(account AccountView) {
		identity := identities[account.ID]
		if identity == nil || !identity.GetFound() {
			r.addResult(config.TestFingerprint, Result{
				AccountID: account.ID, AccountName: account.Name, Model: model,
				Status: "skipped", Error: "账号不可用或未启用调度", Time: time.Now().UTC().Format(time.RFC3339),
			})
			return
		}
		r.addResult(config.TestFingerprint, r.engine.runFingerprint(ctx, cfg, identity, account, model))
	})
}

func (r *Runner) runPelicanTest(ctx context.Context, cfg config.Config, accounts []AccountView, identities map[int64]*pluginv1.ResolveOutboundIdentityResponse) {
	model := firstModel(cfg)
	r.forEachAccount(ctx, cfg.Concurrency, accountIDs(accounts), func(account AccountView) {
		identity := identities[account.ID]
		if identity == nil || !identity.GetFound() {
			r.addResult(config.TestPelican, Result{
				AccountID: account.ID, AccountName: account.Name, Model: model,
				Status: "skipped", Error: "账号不可用或未启用调度", Time: time.Now().UTC().Format(time.RFC3339),
			})
			return
		}
		r.addResult(config.TestPelican, r.engine.runPelican(ctx, cfg, identity, account, model, cfg.PelicanVariant))
	})
}

func (r *Runner) runSIWCTest(ctx context.Context, cfg config.Config, accounts []AccountView, identities map[int64]*pluginv1.ResolveOutboundIdentityResponse) {
	model := firstModel(cfg)
	r.forEachAccount(ctx, cfg.Concurrency, accountIDs(accounts), func(account AccountView) {
		identity := identities[account.ID]
		if identity == nil || !identity.GetFound() {
			r.addResult(config.TestSIWC, Result{
				AccountID: account.ID, AccountName: account.Name, Model: model,
				Status: "skipped", Error: "账号不可用或未启用调度", Time: time.Now().UTC().Format(time.RFC3339),
			})
			return
		}
		r.addResult(config.TestSIWC, r.engine.runSIWC(ctx, identity, account, model))
	})
}

func (r *Runner) forEachAccount(ctx context.Context, concurrency int, order []int64, fn func(AccountView)) {
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > len(order) {
		concurrency = len(order)
	}
	if concurrency == 0 {
		return
	}
	byID := make(map[int64]AccountView, len(order))
	r.mu.Lock()
	for _, account := range r.accounts {
		byID[account.ID] = account
	}
	r.mu.Unlock()

	queue := make(chan int64)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range queue {
				if ctx.Err() != nil {
					return
				}
				fn(byID[id])
			}
		}()
	}
	for _, id := range order {
		select {
		case queue <- id:
		case <-ctx.Done():
		}
	}
	close(queue)
	wg.Wait()
}

func accountIDs(accounts []AccountView) []int64 {
	out := make([]int64, len(accounts))
	for i, account := range accounts {
		out[i] = account.ID
	}
	return out
}

func firstModel(cfg config.Config) string {
	if len(cfg.Models) == 0 {
		return config.DefaultModel
	}
	return cfg.Models[0]
}

func (r *Runner) discover(ctx context.Context, host pluginv1.HostServiceClient, cfg config.Config) ([]AccountView, map[int64]*pluginv1.ResolveOutboundIdentityResponse, error) {
	listCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, err := host.ListAccounts(listCtx, &pluginv1.ListAccountsRequest{Platform: "openai", AccountType: "oauth"})
	if err != nil {
		return nil, nil, fmt.Errorf("读取账号列表失败：%w", err)
	}

	wanted := map[int64]bool{}
	for _, id := range cfg.AccountIDs {
		wanted[id] = true
	}
	accounts := make([]AccountView, 0, len(resp.GetAccounts()))
	seen := map[int64]bool{}
	for _, info := range resp.GetAccounts() {
		id := info.GetId()
		if len(wanted) > 0 && !wanted[id] {
			continue
		}
		seen[id] = true
		accounts = append(accounts, AccountView{
			ID: id, Name: info.GetName(), Status: info.GetStatus(), Schedulable: info.GetSchedulable(),
		})
	}
	// 兼容较老的宿主：早期 HostService 只返回账号 ID 列表，没有 AccountInfo 明细。
	for _, id := range resp.GetAccountIds() {
		if len(wanted) > 0 && !wanted[id] {
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		accounts = append(accounts, AccountView{ID: id, Name: fmt.Sprintf("账号 %d", id), Status: "active", Schedulable: true})
	}
	// 指定了账号但宿主目录里没有（例如刚被停用）：仍然展示并记为跳过，而不是静默消失。
	for id := range wanted {
		if !seen[id] {
			accounts = append(accounts, AccountView{ID: id, Name: fmt.Sprintf("账号 %d", id), Status: "unknown"})
		}
	}
	sort.Slice(accounts, func(i, j int) bool { return accounts[i].ID < accounts[j].ID })

	identities := make(map[int64]*pluginv1.ResolveOutboundIdentityResponse, len(accounts))
	for _, account := range accounts {
		if cfg.SchedulableOnly && !account.Schedulable {
			continue
		}
		resolveCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		identity, err := host.ResolveOutboundIdentity(resolveCtx, &pluginv1.ResolveOutboundIdentityRequest{AccountId: account.ID})
		cancel()
		if err == nil && identity.GetFound() {
			identities[account.ID] = identity
		}
	}
	return accounts, identities, nil
}

func (r *Runner) persist(state persistedState) {
	host := r.engine.host.get()
	if host == nil {
		return
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := host.KVSet(ctx, &pluginv1.KVSetRequest{Namespace: kvNamespace, Key: kvStateKey, Value: raw}); err != nil {
		r.engine.log("warn", "保存检测结果失败："+err.Error())
	}
}

// LoadState 在宿主服务就绪后恢复上一次检测结果，供 UI 在插件重启后仍能看到历史。
func (r *Runner) LoadState() {
	host := r.engine.host.get()
	if host == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := host.KVGet(ctx, &pluginv1.KVGetRequest{Namespace: kvNamespace, Key: kvStateKey})
	if err != nil || !resp.GetFound() {
		return
	}
	var state persistedState
	if err := json.Unmarshal(resp.GetValue(), &state); err != nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return
	}
	r.accounts = state.Accounts
	if len(state.Tests) == 0 && len(state.Results) > 0 {
		state.Tests = map[string]persistedTest{
			config.TestCandy: {LastRunAt: state.LastRunAt, Summary: state.Summary, Results: state.Results},
		}
	}
	for name, saved := range state.Tests {
		status, ok := r.tests[name]
		if !ok {
			continue
		}
		// 不覆盖本次进程内已经产生的更新结果。
		if len(status.Results) > 0 {
			continue
		}
		status.Results = saved.Results
		status.Summary = saved.Summary
		status.LastRunAt = saved.LastRunAt
	}
}

func triggerLabel(trigger string) string {
	if trigger == "schedule" {
		return "定时"
	}
	return "手动"
}

func testLabel(test string) string {
	switch test {
	case config.TestFingerprint:
		return "指纹"
	case config.TestPelican:
		return "鹈鹕"
	case config.TestSIWC:
		return "SIWC 可用性"
	default:
		return "糖果"
	}
}

func stringsJoin(values []string) string {
	out := ""
	for i, value := range values {
		if i > 0 {
			out += "、"
		}
		out += testLabel(value)
	}
	return out
}
