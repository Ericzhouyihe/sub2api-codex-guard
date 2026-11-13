// Sub2API 插件配置页：糖果、指纹、鹈鹕、SIWC 检测的配置、状态、结果与日志。
(function () {
  "use strict";

  var DEFAULTS = {
    models: ["gpt-6.1-sol"],
    runs: 3,
    concurrency: 4,
    effort: "",
    account_ids: [],
    schedulable_only: false,
    schedule_enabled: false,
    schedule_minutes: 60,
    schedule_tests: ["candy"],
    active_test: "candy",
    fingerprint_concurrency: 3,
    pelican_variant: "original"
  };

  var TESTS = {
    candy: { label: "糖果", short: "糖果" },
    fingerprint: { label: "指纹", short: "指纹" },
    pelican: { label: "鹈鹕", short: "鹈鹕" },
    siwc: { label: "SIWC 可用性", short: "SIWC" }
  };
  var TEST_ORDER = ["candy", "fingerprint", "pelican", "siwc"];
  var POLL_INTERVAL_MS = 3000;
  var MAX_RESULTS = 100;
  var MAX_LOGS = 30;
  var TEST_PENDING_MS = 15000;
  var EFFORT_VALUES = ["", "low", "medium", "high"];
  var PELICAN_VARIANTS = ["original", "all-in-one"];
  var FIELD_IDS = ["models", "runs", "concurrency", "effort", "account_ids", "schedulable_only", "schedule_enabled", "schedule_minutes", "schedule_tests", "active_test", "fingerprint_concurrency", "pelican_variant"];

  var RESULT_STATUS = {
    correct: { label: "正确", className: "badge-correct" },
    wrong: { label: "错误", className: "badge-wrong" },
    completed: { label: "完成", className: "badge-correct" },
    partial: { label: "部分", className: "badge-warn" },
    available: { label: "可用", className: "badge-correct" },
    failed: { label: "失败", className: "badge-failed" },
    skipped: { label: "跳过", className: "badge-skipped" },
    auth_error: { label: "认证失败", className: "badge-failed" },
    restricted: { label: "受限", className: "badge-warn" },
    limited: { label: "限流", className: "badge-warn" }
  };
  var ACCOUNT_STATUS = {
    active: { label: "正常", className: "acct-active" },
    inactive: { label: "停用", className: "acct-off" },
    disabled: { label: "停用", className: "acct-off" },
    error: { label: "异常", className: "acct-error" },
    banned: { label: "封禁", className: "acct-error" },
    limited: { label: "受限", className: "acct-warn" }
  };

  var bridge = window.Sub2APIBridge.create();
  var baseConfig = {};
  var actionBusy = false;
  var statusInFlight = false;
  var statusTimer = null;
  var testPending = false;
  var testPendingTimer = null;
  var disposed = false;
  var resizeQueued = false;
  var currentTest = "candy";
  var latestStatus = {};

  function $(id) { return document.getElementById(id); }

  function setActionStatus(message) { $("action-status").textContent = message || ""; }

  function setBanner(kind, message) {
    var banner = $("banner");
    banner.hidden = !message;
    banner.className = "banner" + (kind ? " banner-" + kind : "");
    banner.textContent = message || "";
    resize();
  }

  function setFieldError(id, message) {
    var error = $(id + "-error");
    if (error) { error.textContent = message || ""; error.hidden = !message; }
    var input = $(id);
    if (input) {
      if (message) input.setAttribute("aria-invalid", "true");
      else input.removeAttribute("aria-invalid");
    }
  }

  function clearFieldErrors() { FIELD_IDS.forEach(function (id) { setFieldError(id, ""); }); }

  function validationError(message, fieldId) {
    var error = new Error(message);
    error.fieldId = fieldId || "";
    return error;
  }

  function markDirty(value) { $("dirty").hidden = !value; }

  function resize() {
    if (resizeQueued || !bridge.hasToken) return;
    resizeQueued = true;
    requestAnimationFrame(function () {
      resizeQueued = false;
      bridge.resize(document.documentElement.scrollHeight);
    });
  }

  function configFromReply(reply) {
    if (!reply || reply.ok !== true) return null;
    var result = reply.result;
    if (result && result.config && typeof result.config === "object") return result.config;
    if (reply.config && typeof reply.config === "object") return reply.config;
    return null;
  }

  function toCount(value) { var n = Number(value); return isFinite(n) && n > 0 ? Math.floor(n) : 0; }
  function pad2(value) { return value < 10 ? "0" + value : String(value); }

  function formatTime(value) {
    if (!value) return "—";
    var date = new Date(value);
    if (isNaN(date.getTime())) return String(value);
    return date.getFullYear() + "-" + pad2(date.getMonth() + 1) + "-" + pad2(date.getDate()) +
      " " + pad2(date.getHours()) + ":" + pad2(date.getMinutes()) + ":" + pad2(date.getSeconds());
  }

  function formatDuration(value) {
    if (value === null || value === undefined || value === "") return "—";
    var ms = Number(value);
    if (!isFinite(ms) || ms < 0) return "—";
    if (ms < 1000) return Math.round(ms) + " ms";
    if (ms < 10000) return (ms / 1000).toFixed(2) + " s";
    return (ms / 1000).toFixed(1) + " s";
  }

  function parseAccountIds(text) {
    var value = text.trim();
    if (!value) return [];
    var tokens = value.split(/[\s,，、;；]+/).filter(Boolean);
    var ids = [];
    tokens.forEach(function (token) {
      if (!/^[1-9][0-9]*$/.test(token)) throw validationError("账号 ID 必须是正整数（用逗号或空格分隔）：" + token, "account_ids");
      var id = Number(token);
      if (!Number.isSafeInteger(id)) throw validationError("账号 ID 超出安全整数范围：" + token, "account_ids");
      if (ids.indexOf(id) === -1) ids.push(id);
    });
    return ids;
  }

  function readInteger(id, label, min, max) {
    var raw = $(id).value.trim();
    if (!/^[0-9]+$/.test(raw)) throw validationError(label + "必须是整数。", id);
    var value = Number(raw);
    if (value < min || value > max) throw validationError(label + "必须在 " + min + "–" + max + " 之间。", id);
    return value;
  }

  function selectedScheduleTests() {
    return TEST_ORDER.filter(function (test) {
      var element = $("test_" + test);
      return element && element.checked;
    });
  }

  function collectConfig() {
    var models = $("models").value.split(/\r?\n/).map(function (line) { return line.trim(); }).filter(Boolean);
    if (models.length === 0) throw validationError("请至少填写 1 个模型。", "models");
    if (models.length > 10) throw validationError("最多只能填写 10 个模型（当前 " + models.length + " 个）。", "models");

    var effort = $("effort").value;
    if (EFFORT_VALUES.indexOf(effort) === -1) effort = "";
    var scheduleTests = selectedScheduleTests();
    if (scheduleTests.length === 0) {
      if ($("schedule_enabled").checked) throw validationError("启用定时检测时至少选择一个检测项目。", "schedule_tests");
      scheduleTests = ["candy"];
    }
    var activeTest = $("active_test").value;
    if (!TESTS[activeTest]) activeTest = "candy";
    var pelicanVariant = $("pelican_variant").value;
    if (PELICAN_VARIANTS.indexOf(pelicanVariant) === -1) pelicanVariant = "original";

    var config = Object.assign({}, baseConfig);
    config.models = models;
    config.runs = readInteger("runs", "糖果检测次数", 1, 10);
    config.concurrency = readInteger("concurrency", "账号并发数", 1, 16);
    config.effort = effort;
    config.account_ids = parseAccountIds($("account_ids").value);
    config.schedulable_only = $("schedulable_only").checked;
    config.schedule_enabled = $("schedule_enabled").checked;
    config.schedule_minutes = readInteger("schedule_minutes", "定时检测间隔", 5, 10080);
    config.schedule_tests = scheduleTests;
    config.active_test = activeTest;
    config.fingerprint_concurrency = readInteger("fingerprint_concurrency", "指纹并发数", 1, 3);
    config.pelican_variant = pelicanVariant;
    return config;
  }

  function applyConfig(config) {
    baseConfig = Object.assign({}, config || {});
    var cfg = Object.assign({}, DEFAULTS, config || {});
    $("models").value = Array.isArray(cfg.models) ? cfg.models.join("\n") : "";
    $("runs").value = cfg.runs == null ? String(DEFAULTS.runs) : String(cfg.runs);
    $("concurrency").value = cfg.concurrency == null ? String(DEFAULTS.concurrency) : String(cfg.concurrency);
    $("effort").value = EFFORT_VALUES.indexOf(cfg.effort) === -1 ? "" : cfg.effort;
    $("account_ids").value = Array.isArray(cfg.account_ids) ? cfg.account_ids.join(", ") : "";
    $("schedulable_only").checked = cfg.schedulable_only === true;
    $("schedule_enabled").checked = cfg.schedule_enabled === true;
    $("schedule_minutes").value = cfg.schedule_minutes == null ? String(DEFAULTS.schedule_minutes) : String(cfg.schedule_minutes);
    var scheduleTests = Array.isArray(cfg.schedule_tests) && cfg.schedule_tests.length ? cfg.schedule_tests : ["candy"];
    TEST_ORDER.forEach(function (test) { $("test_" + test).checked = scheduleTests.indexOf(test) !== -1; });
    $("active_test").value = TESTS[cfg.active_test] ? cfg.active_test : "candy";
    $("fingerprint_concurrency").value = cfg.fingerprint_concurrency == null ? String(DEFAULTS.fingerprint_concurrency) : String(cfg.fingerprint_concurrency);
    $("pelican_variant").value = PELICAN_VARIANTS.indexOf(cfg.pelican_variant) === -1 ? "original" : cfg.pelican_variant;
    currentTest = $("active_test").value;
    clearFieldErrors();
    markDirty(false);
    renderTabs();
    resize();
  }

  function anyRunning() {
    return TEST_ORDER.some(function (test) {
      var status = (latestStatus.tests || {})[test] || latestStatus[test] || {};
      return status.running === true || status.stopping === true;
    });
  }

  function updateButtons() {
    var available = bridge.hasToken;
    var testing = anyRunning() || testPending;
    $("save").disabled = !available || actionBusy;
    $("refresh").disabled = !available || actionBusy;
    $("test").disabled = !available || actionBusy || testing;
    $("test").textContent = testing ? "检测进行中…" : "立即检测";
  }

  function setBusy(value, message) {
    actionBusy = value;
    if (typeof message === "string") setActionStatus(message);
    updateButtons();
  }

  function clearTestPending() {
    testPending = false;
    if (testPendingTimer !== null) { clearTimeout(testPendingTimer); testPendingTimer = null; }
    updateButtons();
  }

  function markTestPending() {
    testPending = true;
    if (testPendingTimer !== null) clearTimeout(testPendingTimer);
    testPendingTimer = setTimeout(function () { testPendingTimer = null; testPending = false; updateButtons(); }, TEST_PENDING_MS);
    updateButtons();
  }

  function setHealth(healthy, detail) {
    $("health-dot").className = "status-dot " + (healthy ? "ok" : "off");
    $("health-text").textContent = healthy ? "插件运行中" : "插件尚未启用";
    var detailEl = $("health-detail");
    detailEl.textContent = detail || "";
    detailEl.hidden = !detail;
  }

  function setHealthError(message) {
    $("health-dot").className = "status-dot err";
    $("health-text").textContent = "状态读取失败";
    var detailEl = $("health-detail");
    detailEl.textContent = message || "";
    detailEl.hidden = !message;
  }

  function statusFor(test) {
    var tests = latestStatus.tests && typeof latestStatus.tests === "object" ? latestStatus.tests : {};
    var status = tests[test] || latestStatus[test];
    return status && typeof status === "object" ? status : {};
  }

  function renderTestStatusGrid(healthy) {
    var grid = $("test-status-grid");
    grid.textContent = "";
    TEST_ORDER.forEach(function (test) {
      var status = statusFor(test);
      var card = document.createElement("div");
      card.className = "test-status-card";
      var head = document.createElement("div");
      head.className = "test-status-head";
      var name = document.createElement("span");
      name.className = "test-status-name";
      name.textContent = TESTS[test].label;
      var dot = document.createElement("span");
      dot.className = "pulse-dot" + (status.running === true ? " running" : "") + (!healthy ? " off" : "");
      var text = document.createElement("span");
      text.className = "test-status-text";
      if (!healthy) text.textContent = "未运行";
      else if (status.stopping === true) text.textContent = "正在停止…";
      else if (status.running === true) text.textContent = "检测中";
      else text.textContent = "空闲";
      head.appendChild(name); head.appendChild(dot); head.appendChild(text);
      var meta = document.createElement("div");
      meta.className = "test-status-meta";
      meta.textContent = "上次：" + formatTime(status.last_run_at);
      if (status.last_error) {
        var err = document.createElement("div");
        err.className = "test-status-error";
        err.textContent = status.last_error;
        card.appendChild(err);
      }
      card.appendChild(head);
      card.appendChild(meta);
      grid.appendChild(card);
    });
  }

  function renderSummary(summary, test) {
    var data = summary && typeof summary === "object" ? summary : {};
    var chips = document.querySelectorAll(".summary-chips .chip");
    if (chips.length < 5) return;
    var labels;
    if (test === "candy") labels = ["正确", "错误", "失败", "跳过"];
    else if (test === "fingerprint") labels = ["完成", "部分", "失败", "跳过"];
    else if (test === "pelican") labels = ["完成", "—", "失败", "—"];
    else labels = ["可用", "—", "失败", "—"];
    for (var i = 0; i < 4; i++) {
      var span = chips[i].querySelector("span");
      if (span) span.textContent = labels[i];
    }
    var values;
    if (test === "candy") values = [data.correct, data.wrong, data.failed, data.skipped];
    else if (test === "fingerprint") values = [data.completed, data.partial, data.failed, data.skipped];
    else if (test === "pelican") values = [data.completed, 0, data.failed, 0];
    else values = [data.available, 0, data.failed, 0];
    for (var j = 0; j < 4; j++) {
      var strong = chips[j].querySelector("strong");
      if (strong) strong.textContent = String(toCount(values[j]));
    }
    var total = data.total == null ? values.reduce(function (a, b) { return a + toCount(b); }, 0) : toCount(data.total);
    $("sum-total").textContent = String(total);
  }

  function appendCell(row, text, className) {
    var cell = document.createElement("td");
    if (className) cell.className = className;
    cell.textContent = text == null ? "" : String(text);
    row.appendChild(cell);
    return cell;
  }

  function makeBadge(label, className) {
    var badge = document.createElement("span");
    badge.className = "status-badge " + className;
    badge.textContent = label;
    return badge;
  }

  function appendEmptyRow(tbody, columns, text) {
    var row = document.createElement("tr");
    row.className = "empty-row";
    var cell = document.createElement("td");
    cell.colSpan = columns;
    cell.textContent = text;
    row.appendChild(cell);
    tbody.appendChild(row);
  }

  var FINGERPRINT_ALIASES = { "gpt-6.1-sol": "gpt-6-astra", "deepseek-flash": "deepseek-v4.1-flash-1017" };

  function fingerprintModelID(model) {
    var id = String(model || "").trim().toLowerCase();
    id = id.split("/").pop();
    return FINGERPRINT_ALIASES[id] || id;
  }

  function detailText(entry, test) {
    if (entry.error) return String(entry.error);
    if (test === "candy") return entry.answer ? String(entry.answer) : "";
    if (test === "fingerprint") {
      var attribution = entry.attribution && typeof entry.attribution === "object" ? entry.attribution : null;
      if (attribution) {
        if (attribution.decision === "not_model") return "答案不像模型输出";
        var prediction = attribution.prediction ? String(attribution.prediction) : "未知";
        var probability = typeof attribution.probability === "number" ? " (" + Math.round(attribution.probability * 100) + "%)" : "";
        var used = typeof attribution.used_outputs === "number" ? " · " + attribution.used_outputs + "/3 题有效" : "";
        var mismatch = attribution.prediction && fingerprintModelID(entry.model) !== fingerprintModelID(attribution.prediction) ? " · 与测试模型不一致" : "";
        return "判定：" + prediction + probability + used + mismatch;
      }
      return entry.answer ? String(entry.answer) : "";
    }
    if (test === "pelican") return entry.html ? "已生成可预览页面" : (entry.answer ? String(entry.answer) : "");
    return entry.answer ? String(entry.answer) : "";
  }

  function renderResults(results, test) {
    var tbody = $("results-body");
    tbody.textContent = "";
    var rows = Array.isArray(results) ? results.slice(-MAX_RESULTS) : [];
    $("results-meta").textContent = rows.length ? "最近 " + rows.length + " 条（最新在前）" : "暂无";
    if (!rows.length) { appendEmptyRow(tbody, 6, "暂无检测结果"); return; }
    rows.slice().reverse().forEach(function (item) {
      var entry = item && typeof item === "object" ? item : {};
      var row = document.createElement("tr");
      appendCell(row, formatTime(entry.time), "num");

      var accountCell = document.createElement("td");
      var name = document.createElement("span");
      name.className = "cell-strong";
      name.textContent = entry.account_name ? String(entry.account_name) : (entry.account_id != null ? "账号 #" + entry.account_id : "未知账号");
      accountCell.appendChild(name);
      if (entry.account_name && entry.account_id != null) {
        var accountId = document.createElement("span");
        accountId.className = "cell-sub";
        accountId.textContent = "#" + entry.account_id;
        accountCell.appendChild(accountId);
      }
      row.appendChild(accountCell);

      var modelText = entry.model || "—";
      if (test === "pelican" && entry.variant) modelText += " / " + entry.variant;
      if (test === "candy" && entry.run != null) modelText += " #" + entry.run;
      appendCell(row, modelText);

      var statusCell = document.createElement("td");
      var statusKey = String(entry.status || "");
      var statusMeta = RESULT_STATUS[statusKey] || { label: statusKey || "未知", className: "badge-unknown" };
      statusCell.appendChild(makeBadge(statusMeta.label, statusMeta.className));
      row.appendChild(statusCell);
      appendCell(row, formatDuration(entry.duration_ms), "num");

      var detailCell = document.createElement("td");
      var text = detailText(entry, test);
      if (test === "pelican" && entry.html) {
        var button = document.createElement("button");
        button.type = "button";
        button.className = "btn btn-ghost btn-small";
        button.textContent = "查看图";
        button.addEventListener("click", function () { openPreview(String(entry.html)); });
        detailCell.appendChild(button);
        if (entry.attempts) {
          var attempts = document.createElement("span");
          attempts.className = "cell-sub";
          attempts.textContent = "尝试 " + entry.attempts + " 次";
          detailCell.appendChild(attempts);
        }
      } else {
        var detail = document.createElement("span");
        detail.className = "cell-ellipsis";
        detail.textContent = text || "—";
        if (text) detail.title = text;
        detailCell.appendChild(detail);
      }
      row.appendChild(detailCell);
      tbody.appendChild(row);
    });
  }

  function renderAccounts(accounts) {
    var tbody = $("accounts-body");
    tbody.textContent = "";
    var rows = Array.isArray(accounts) ? accounts : [];
    $("accounts-meta").textContent = rows.length ? rows.length + " 个账号" : "暂无";
    if (!rows.length) { appendEmptyRow(tbody, 4, "暂无账号数据"); return; }
    rows.forEach(function (item) {
      var entry = item && typeof item === "object" ? item : {};
      var row = document.createElement("tr");
      appendCell(row, entry.id == null ? "—" : entry.id, "num");
      appendCell(row, entry.name || "—");
      var statusCell = document.createElement("td");
      var statusKey = String(entry.status || "").toLowerCase();
      var statusMeta = ACCOUNT_STATUS[statusKey] || { label: entry.status ? String(entry.status) : "未知", className: "badge-unknown" };
      statusCell.appendChild(makeBadge(statusMeta.label, statusMeta.className));
      row.appendChild(statusCell);
      var schedulableCell = document.createElement("td");
      var schedulable = document.createElement("span");
      schedulable.className = "sched-flag " + (entry.schedulable === true ? "sched-yes" : "sched-no");
      schedulable.textContent = entry.schedulable === true ? "是" : "否";
      schedulableCell.appendChild(schedulable);
      row.appendChild(schedulableCell);
      tbody.appendChild(row);
    });
  }

  function renderLogs(logs) {
    var list = $("logs");
    list.textContent = "";
    var entries = Array.isArray(logs) ? logs.slice(-MAX_LOGS) : [];
    $("logs-meta").textContent = entries.length ? "最近 " + entries.length + " 条（最新在前）" : "暂无";
    if (!entries.length) {
      var empty = document.createElement("li");
      empty.className = "empty-log";
      empty.textContent = "暂无日志";
      list.appendChild(empty);
      return;
    }
    entries.slice().reverse().forEach(function (item) {
      var entry = item && typeof item === "object" ? item : {};
      var line = document.createElement("li");
      var time = document.createElement("span");
      time.className = "log-time";
      time.textContent = formatTime(entry.time);
      var levelName = String(entry.level || "info").toLowerCase();
      var level = document.createElement("span");
      level.className = "log-level " + levelName;
      level.textContent = levelName;
      var message = document.createElement("span");
      message.className = "log-message";
      message.textContent = entry.message ? String(entry.message) : "";
      line.appendChild(time); line.appendChild(level); line.appendChild(message);
      list.appendChild(line);
    });
  }

  function renderTabs() {
    var tabs = document.querySelectorAll("#result-tabs .tab");
    for (var i = 0; i < tabs.length; i++) {
      var active = tabs[i].getAttribute("data-test") === currentTest;
      tabs[i].classList.toggle("active", active);
      tabs[i].setAttribute("aria-selected", active ? "true" : "false");
    }
  }

  function renderCurrentTest() {
    var status = statusFor(currentTest);
    var chips = document.querySelectorAll(".summary-chips .chip");
    var healthy = latestStatus.__healthy === true;
    renderSummary(status.summary, currentTest);
    renderResults(status.results, currentTest);
    $("last-run").textContent = formatTime(status.last_run_at);
    $("next-run").textContent = formatTime(latestStatus.next_run_at);
    renderTestStatusGrid(healthy);
    updateButtons();
    resize();
  }

  function applyStatusReply(reply) {
    if (!reply || reply.ok !== true) {
      setHealthError((reply && reply.error) || "无法读取插件状态");
      updateButtons();
      resize();
      return;
    }
    var result = reply.result && typeof reply.result === "object" ? reply.result : {};
    var healthy = result.healthy === true;
    var status = {};
    var raw = result.status_json;
    if (typeof raw === "string" && raw.trim()) {
      try { status = JSON.parse(raw); } catch (error) {
        setHealth(healthy, "状态数据解析失败");
        updateButtons(); resize(); return;
      }
    } else if (raw && typeof raw === "object" && !Array.isArray(raw)) {
      status = raw;
    }
    if (!status || typeof status !== "object" || Array.isArray(status)) status = {};
    status.__healthy = healthy;
    latestStatus = status;
    setHealth(healthy, result.message || "");
    if (anyRunning()) clearTestPending();
    renderCurrentTest();
    renderAccounts(status.accounts);
    renderLogs(status.logs);
    if (status.version) $("version-badge").textContent = "v" + status.version;
  }

  function applyStatusFailure(error) {
    setHealthError((error && error.message) || "无法读取插件状态");
    updateButtons(); resize();
  }

  function scheduleStatusPoll(delay) {
    if (statusTimer !== null) { clearTimeout(statusTimer); statusTimer = null; }
    if (disposed || document.hidden) return;
    statusTimer = setTimeout(function () { statusTimer = null; requestStatus(false); }, delay);
  }

  function requestStatus(manual) {
    if (disposed) return;
    if (statusInFlight) { if (manual) setActionStatus("状态正在刷新…"); return; }
    statusInFlight = true;
    if (manual) setActionStatus("正在刷新状态…");
    bridge.request("plugin.status").then(function (reply) {
      applyStatusReply(reply);
      if (manual) setActionStatus(reply && reply.ok === true ? "状态已刷新" : "状态读取失败");
    }).catch(function (error) {
      applyStatusFailure(error);
      if (manual) setActionStatus("状态读取失败");
    }).finally(function () { statusInFlight = false; scheduleStatusPoll(POLL_INTERVAL_MS); });
  }

  function saveConfig(config) {
    return bridge.request("config.save", { config: config }).then(function (reply) {
      if (!reply || reply.ok !== true) throw new Error((reply && reply.error) || "保存失败");
      var saved = configFromReply(reply) || config;
      applyConfig(saved);
      return saved;
    });
  }

  function showValidationError(error) {
    if (error.fieldId === "schedule_tests") setFieldError("schedule_tests", error.message);
    else if (error.fieldId) setFieldError(error.fieldId, error.message);
    setBanner("error", error.message);
    setActionStatus("请先修正配置");
    var field = error.fieldId && error.fieldId !== "schedule_tests" ? $(error.fieldId) : null;
    if (field && field.focus) field.focus();
  }

  function onSave() {
    if (actionBusy) return;
    clearFieldErrors(); setBanner("", "");
    var config;
    try { config = collectConfig(); } catch (error) { showValidationError(error); return; }
    setBusy(true, "正在保存配置…");
    saveConfig(config).then(function () {
      setBanner("ok", "配置已保存并生效");
      setActionStatus("配置已保存并生效");
      bridge.notify("success", "配置已保存并生效");
    }).catch(function (error) {
      setBanner("error", error.message); setActionStatus("保存失败");
      bridge.notify("error", "保存失败：" + error.message);
    }).finally(function () { setBusy(false); });
  }

  function onTest() {
    if (actionBusy || anyRunning() || testPending) return;
    clearFieldErrors(); setBanner("", "");
    var config;
    try { config = collectConfig(); } catch (error) { showValidationError(error); return; }
    setBusy(true, "正在保存配置…");
    saveConfig(config).then(function () {
      setBusy(false, "");
      setActionStatus("检测已开始，结果会自动刷新");
      markTestPending();
      bridge.notify("success", "检测已开始，结果会自动刷新");
      requestStatus(false);
      bridge.request("config.test").then(function (reply) {
        if (!reply || reply.ok !== true) throw new Error((reply && reply.error) || "检测启动失败");
        var message = reply.result && reply.result.message ? String(reply.result.message) : "检测已开始，结果会自动刷新";
        setActionStatus(message);
      }).catch(function (error) {
        var message = String((error && error.message) || error || "");
        if (message.indexOf("超时") !== -1) setActionStatus("检测请求超时；插件可能仍在运行，结果会自动刷新。");
        else {
          setBanner("error", "检测启动失败：" + message);
          setActionStatus("检测启动失败");
          bridge.notify("error", "检测启动失败：" + message);
        }
      });
    }).catch(function (error) {
      setBanner("error", error.message); setActionStatus("保存失败");
      bridge.notify("error", "保存失败：" + error.message);
      setBusy(false);
    });
  }

  function onRefresh() {
    if (!bridge.hasToken || actionBusy) return;
    requestStatus(true);
  }

  function handleFieldChanged(event) {
    markDirty(true);
    if (event.target && event.target.id) setFieldError(event.target.id, "");
  }

  function openPreview(html) {
    $("preview-frame").srcdoc = html;
    $("preview-modal").hidden = false;
    resize();
  }

  function closePreview() {
    $("preview-modal").hidden = true;
    $("preview-frame").srcdoc = "";
    resize();
  }

  function shutdown() {
    if (disposed) return;
    disposed = true;
    if (statusTimer !== null) clearTimeout(statusTimer);
    if (testPendingTimer !== null) clearTimeout(testPendingTimer);
    statusTimer = null; testPendingTimer = null;
    bridge.dispose();
  }

  function init() {
    applyConfig({});
    updateButtons();
    renderTestStatusGrid(false);
    renderSummary({}, "candy");
    renderResults([], "candy");
    renderAccounts([]);
    renderLogs([]);
    if (!bridge.hasToken) {
      setBanner("error", "缺少 Bridge Token，请从 Sub2API 插件管理页打开。");
      $("health-text").textContent = "无法连接宿主";
      return;
    }
    bridge.ready();
    setBusy(true, "正在读取配置…");
    bridge.request("config.load").then(function (reply) {
      if (!reply || reply.ok !== true) throw new Error((reply && reply.error) || "读取配置失败");
      var config = configFromReply(reply);
      if (!config) throw new Error("配置数据格式异常");
      applyConfig(config);
      setActionStatus("配置已加载");
    }).catch(function (error) {
      setBanner("error", error.message); setActionStatus("配置读取失败");
    }).finally(function () {
      setBusy(false);
      requestStatus(false);
      resize();
    });
  }

  FIELD_IDS.forEach(function (id) {
    var element = $(id);
    if (!element) return;
    element.addEventListener("input", handleFieldChanged);
    element.addEventListener("change", handleFieldChanged);
  });
  TEST_ORDER.forEach(function (test) {
    var checkbox = $("test_" + test);
    if (checkbox) checkbox.addEventListener("change", handleFieldChanged);
  });
  $("active_test").addEventListener("change", function () {
    currentTest = $("active_test").value;
    renderTabs();
    renderCurrentTest();
  });
  $("result-tabs").addEventListener("click", function (event) {
    var button = event.target.closest("button[data-test]");
    if (!button) return;
    currentTest = button.getAttribute("data-test");
    $("active_test").value = currentTest;
    markDirty(true);
    renderTabs();
    renderCurrentTest();
  });
  $("save").addEventListener("click", onSave);
  $("test").addEventListener("click", onTest);
  $("refresh").addEventListener("click", onRefresh);
  $("preview-close").addEventListener("click", closePreview);
  $("preview-modal").addEventListener("click", function (event) { if (event.target === $("preview-modal")) closePreview(); });
  window.addEventListener("resize", resize);
  document.addEventListener("visibilitychange", function () {
    if (document.hidden) { if (statusTimer !== null) { clearTimeout(statusTimer); statusTimer = null; } }
    else requestStatus(false);
  });
  window.addEventListener("pagehide", shutdown);
  window.addEventListener("unload", shutdown);

  init();
})();
