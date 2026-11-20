// Package config 解析并规范化 Codex 降智测试插件的配置。
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	// DefaultModel 是社区目前常用的 Codex 模型；用户可在 UI 中改成任意模型名。
	DefaultModel = "gpt-6.1-sol"
	maxModels    = 10
)

// 支持的检测类型。
const (
	TestCandy       = "candy"
	TestFingerprint = "fingerprint"
	TestPelican     = "pelican"
	TestSIWC        = "siwc"
)

var validTests = map[string]bool{
	TestCandy:       true,
	TestFingerprint: true,
	TestPelican:     true,
	TestSIWC:        true,
}

// Config 是插件通过 config.load / config.save 读写的完整配置。
type Config struct {
	Models                 []string `json:"models"`
	Runs                   int      `json:"runs"`
	Concurrency            int      `json:"concurrency"`
	Effort                 string   `json:"effort"`
	AccountIDs             []int64  `json:"account_ids"`
	SchedulableOnly        bool     `json:"schedulable_only"`
	ScheduleEnabled        bool     `json:"schedule_enabled"`
	ScheduleMinutes        int      `json:"schedule_minutes"`
	ScheduleTests          []string `json:"schedule_tests"`
	ActiveTest             string   `json:"active_test,omitempty"`
	FingerprintConcurrency int      `json:"fingerprint_concurrency"`
	PelicanVariant         string   `json:"pelican_variant"`
}

// Default 返回可直接运行的默认配置。
func Default() Config {
	return Config{
		Models:                 []string{DefaultModel},
		Runs:                   3,
		Concurrency:            4,
		Effort:                 "",
		AccountIDs:             []int64{},
		SchedulableOnly:        false,
		ScheduleEnabled:        false,
		ScheduleMinutes:        60,
		ScheduleTests:          []string{TestCandy},
		ActiveTest:             TestCandy,
		FingerprintConcurrency: 3,
		PelicanVariant:         "original",
	}
}

// Parse 解析 JSON 配置并规范化。空配置表示使用默认值，方便首次安装。
func Parse(raw []byte) (Config, error) {
	cfg := Default()
	if len(bytes.TrimSpace(raw)) == 0 {
		return cfg, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("配置格式错误：%w", err)
	}

	models := make([]string, 0, len(cfg.Models))
	seen := map[string]bool{}
	for _, model := range cfg.Models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if len(model) > 200 {
			return Config{}, errors.New("模型名过长")
		}
		if seen[model] {
			continue
		}
		seen[model] = true
		models = append(models, model)
	}
	if len(models) == 0 {
		models = []string{DefaultModel}
	}
	if len(models) > maxModels {
		return Config{}, fmt.Errorf("模型数量不能超过 %d 个", maxModels)
	}
	cfg.Models = models

	if cfg.Runs == 0 {
		cfg.Runs = 3
	}
	if cfg.Runs < 1 || cfg.Runs > 10 {
		return Config{}, errors.New("每个账号答题次数需在 1-10 之间")
	}
	if cfg.Concurrency == 0 {
		cfg.Concurrency = 4
	}
	if cfg.Concurrency < 1 || cfg.Concurrency > 16 {
		return Config{}, errors.New("并发数需在 1-16 之间")
	}
	cfg.Effort = strings.ToLower(strings.TrimSpace(cfg.Effort))
	switch cfg.Effort {
	case "", "low", "medium", "high":
	default:
		return Config{}, errors.New("推理强度只能是 low、medium、high 或留空")
	}

	ids := make([]int64, 0, len(cfg.AccountIDs))
	idSeen := map[int64]bool{}
	for _, id := range cfg.AccountIDs {
		if id <= 0 {
			return Config{}, errors.New("账号 ID 必须是正整数")
		}
		if idSeen[id] {
			continue
		}
		idSeen[id] = true
		ids = append(ids, id)
	}
	cfg.AccountIDs = ids

	if cfg.ScheduleMinutes == 0 {
		cfg.ScheduleMinutes = 60
	}
	if cfg.ScheduleMinutes < 5 || cfg.ScheduleMinutes > 10080 {
		return Config{}, errors.New("定时检测间隔需在 5-10080 分钟之间")
	}

	cfg.ActiveTest = strings.ToLower(strings.TrimSpace(cfg.ActiveTest))
	if cfg.ActiveTest == "" {
		cfg.ActiveTest = TestCandy
	}
	if !validTests[cfg.ActiveTest] {
		return Config{}, errors.New("active_test 只能是 candy、fingerprint、pelican 或 siwc")
	}

	scheduleTests := make([]string, 0, len(cfg.ScheduleTests))
	testSeen := map[string]bool{}
	for _, test := range cfg.ScheduleTests {
		test = strings.ToLower(strings.TrimSpace(test))
		if test == "" || testSeen[test] {
			continue
		}
		if !validTests[test] {
			return Config{}, errors.New("schedule_tests 包含不支持的检测类型")
		}
		testSeen[test] = true
		scheduleTests = append(scheduleTests, test)
	}
	if len(scheduleTests) == 0 {
		scheduleTests = []string{TestCandy}
	}
	cfg.ScheduleTests = scheduleTests

	if cfg.FingerprintConcurrency == 0 {
		cfg.FingerprintConcurrency = 3
	}
	if cfg.FingerprintConcurrency < 1 || cfg.FingerprintConcurrency > 3 {
		return Config{}, errors.New("指纹检测并发需在 1-3 之间")
	}
	cfg.PelicanVariant = strings.ToLower(strings.TrimSpace(cfg.PelicanVariant))
	switch cfg.PelicanVariant {
	case "":
		cfg.PelicanVariant = "original"
	case "original", "all-in-one":
	default:
		return Config{}, errors.New("pelican_variant 只能是 original 或 all-in-one")
	}
	return cfg, nil
}

// JSON 返回规范化后的 JSON，用于 ValidateConfig 的回传与持久化。
func (c Config) JSON() []byte {
	if c.Models == nil {
		c.Models = []string{}
	}
	if c.AccountIDs == nil {
		c.AccountIDs = []int64{}
	}
	if c.ScheduleTests == nil {
		c.ScheduleTests = []string{}
	}
	raw, _ := json.Marshal(c)
	return raw
}
