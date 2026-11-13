// Run: node --test tests/model-guard-dom.test.cjs
// Full embedded HTML + shipped scripts in JSDOM. No production code is replaced.
// JSDOM is supplied by the frontend; tests/ is ignored and needs git add -f.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { test } = require('node:test');
const { JSDOM, VirtualConsole } = require('../frontend/node_modules/jsdom');
const web = path.resolve(__dirname, '../backend/internal/codexeval/web');
const read = (name) => fs.readFileSync(path.join(web, name), 'utf8');
const BASE = '/api/v1/admin/codex-eval';
const SETTINGS = '/api/v1/admin/settings/codex-eval-inspection';
const PREF = 'sub2api-codex-eval.prefs';
const clone = (value) => JSON.parse(JSON.stringify(value));
const flush = async () => { for (let i = 0; i < 12; i++) await new Promise(setImmediate); };

function embeddedHTML() {
  // Keep the actual template's script order, matching web.go's replacements.
  const scripts = {
    MARKDOWN: 'vendor/markdown-it.min.js', CJK_FRIENDLY: 'vendor/markdown-it-cjk-friendly.min.js',
    TEXMATH: 'vendor/texmath.js', TEMML: 'vendor/temml.min.js', STORAGE: 'storage.js',
    CREDENTIALS: 'credentials.js', CATALOG: 'catalog.js', COMPONENTS: 'components.js',
    CANDY: 'candy.js', FINGERPRINT: 'fingerprint.js', HISTORY: 'history.js',
    INSPECTION: 'inspection.js', APP: 'app.js',
  };
  return read('ui.html').replace(/\/\*([A-Z_]+)_SCRIPT\*\//g, (_, key) => {
    assert.ok(scripts[key], `unknown embedded script ${key}`);
    return read(scripts[key]).replace('/*FINGERPRINT_CONFIG*/{}', JSON.stringify({
      requests: 3, aliases: { 'gpt-6.1-sol': 'gpt-6-astra' },
    }));
  });
}

function fixtureSettings() {
  return {
    enabled: true, group_id: 101, account_type: 'all', account_plan: 'all', enabled_only: false,
    model: 'saved/literal', effort: 'high', test_type: 'fingerprint', max_tests_per_account: 3,
    min_wait_minutes: 15, max_wait_minutes: 35, adjust_priority: true, normal_priority: 10,
    degraded_priority: 99, control_models: true, degraded_remove_models: ['saved/blocked'],
    include_patterns: ['codex-*'], exclude_patterns: ['*-backup'], notify_enabled: true,
    ntfy_server: 'https://ntfy.sh', ntfy_topic: 'model-guard', ntfy_username: 'operator',
    ntfy_password_configured: true, bark_url: 'https://bark.invalid/device',
  };
}
function fixtureState() {
  return { active: [], candy: { '1': [{ time: '2026-10-01T00:00:00Z', model: 'saved/literal',
    effort: 'high', answer: '**答案：**21 <img src=x onerror=alert(1)>', ok: true }] },
  fingerprint: { '1': [{ id: 'record/one', time: '2026-10-02T00:00:00Z', model: 'gpt-6.1-sol',
    status: 'completed', duration_ms: 20, concurrency: 3,
    attribution: { prediction: 'gpt-6-astra', prediction_name: 'GPT-6 Astra', probability: null, results: [] } }] },
  pelican: { '1': [{ time: '2026-10-03T00:00:00Z', model: 'UNSUPPORTED_SENTINEL' }] } };
}
async function harness(t, options = {}) {
  const calls = [], errors = [], timers = new Map();
  let nextTimer = 0;
  const state = options.state || fixtureState();
  let settings = { ...fixtureSettings(), ...options.settings };
  const accounts = options.accounts || [
    { id: 1, name: 'Account <one>', platform: 'openai', type: 'apikey', status: 'active', schedulable: true, group_ids: [101] },
    { id: 2, name: 'Disabled account', platform: 'openai', type: 'oauth', status: 'active', schedulable: false, group_ids: [101] },
    { id: 3, name: 'Other group', platform: 'openai', type: 'apikey', status: 'active', schedulable: true, group_ids: [202] },
  ];
  const virtualConsole = new VirtualConsole();
  virtualConsole.on('jsdomError', (error) => errors.push(error));
  virtualConsole.on('error', (...args) => errors.push(new Error(args.join(' '))));
  const dom = new JSDOM(embeddedHTML(), {
    url: options.url || 'https://admin.example/codex-eval?theme=dark', runScripts: 'dangerously',
    virtualConsole, beforeParse(window) {
      // Only missing browser primitives are shimmed; app DOM/events/validation stay real.
      window.AbortController = AbortController;
      window.AbortSignal = AbortSignal;
      window.matchMedia = () => ({ matches: false, addEventListener() {}, removeEventListener() {} });
      window.CSS = { escape: (value) => String(value).replace(/[^a-zA-Z0-9_-]/g, (c) => `\\${c}`) };
      window.HTMLDialogElement.prototype.showModal = function () { this.open = true; };
      window.HTMLDialogElement.prototype.close = function (value) {
        this.open = false; if (value !== undefined) this.returnValue = value;
        this.dispatchEvent(new window.Event('close'));
      };
      window.setTimeout = (fn, delay, ...args) => {
        const id = ++nextTimer; timers.set(id, { fn: () => fn(...args), delay }); return id;
      };
      window.clearTimeout = (id) => timers.delete(id);
      if (options.token !== null) window.localStorage.setItem('auth_token', options.token || 'initial-admin');
      for (const [key, value] of Object.entries(options.storage || {})) window.localStorage.setItem(key, JSON.stringify(value));
      window.fetch = async (url, init) => {
        const request = { url: String(url), init, body: init.body ? JSON.parse(init.body) : undefined };
        calls.push(request);
        const custom = await options.respond?.(request);
        if (custom) return { ok: custom.status >= 200 && custom.status < 300, status: custom.status,
          text: async () => custom.text ?? JSON.stringify({ data: custom.data }) };
        const parsed = new URL(url, window.location.origin);
        let data;
        if (parsed.pathname === SETTINGS) {
          if (init.method === 'PUT') settings = { ...settings, ...request.body };
          data = settings;
        } else if (parsed.pathname === BASE + '/inspection/state') data = { running: false, results: [] };
        else if (parsed.pathname === '/api/v1/admin/groups/all') data = [{ id: 101, name: 'Primary' }, { id: 202, name: 'Other' }];
        else if (parsed.pathname === '/api/v1/admin/accounts') data = { items: accounts.filter((a) => a.group_ids.includes(Number(parsed.searchParams.get('group')))), pages: 1 };
        else if (/\/accounts\/\d+\/codex-eval\/models$/.test(parsed.pathname)) data = [{ id: 'gpt-6.1-sol' }, { id: 'provider/raw' }, { id: 'image-gen' }, { id: 'gpt-6.1-sol(high)' }];
        else if (parsed.pathname === BASE + '/state') data = state;
        else if (parsed.pathname === BASE + '/start') data = { started: [1], busy: [], unavailable: [] };
        else if (parsed.pathname === BASE + '/records/fingerprint/1/record%2Fone') data = { fingerprint: {
          ...state.fingerprint['1'][0], samples: [{ prompt: 'private challenge', text: '<script>bad()</script>', attempts: 1, accepted: true, parsed_numbers: 2, expected_count: 2 }],
        } };
        else throw new Error(`Unmocked request: ${init.method} ${url}`);
        return { ok: true, status: 200, text: async () => JSON.stringify({ data: clone(data) }) };
      };
    },
  });
  t.after(() => { dom.window.close(); assert.deepEqual(errors.map((e) => e.message), [], 'no JSDOM/runtime errors'); });
  const window = dom.window, document = window.document;
  const h = { dom, window, document, calls, errors, timers, $: (id) => document.getElementById(id),
    field: (name) => document.getElementById('inspection-settings').elements.namedItem(name),
    event: (element, type) => element.dispatchEvent(new window.Event(type, { bubbles: true, cancelable: true })),
    async tick(delay) {
      for (const [id, task] of [...timers]) if (task.delay === delay && timers.delete(id)) await task.fn();
      await flush();
    },
    async auth(token) {
      if (token === null) window.localStorage.removeItem('auth_token'); else window.localStorage.setItem('auth_token', token);
      window.dispatchEvent(new window.StorageEvent('storage', { key: 'auth_token', newValue: token }));
      await flush();
    },
  };
  await flush();
  return h;
}

test('full inline startup renders inventory, records, supported navigation and authenticated APIs', async (t) => {
  const h = await harness(t);
  assert.equal(h.$('app').hidden, false);
  assert.equal(h.$('login').hidden, true);
  assert.equal(h.document.documentElement.dataset.theme, 'dark');
  assert.equal(h.$('rows').querySelectorAll('[data-row]').length, 2);
  assert.equal(h.$('fp-rows').querySelectorAll('[data-row]').length, 2);
  assert.match(h.$('rows').textContent, /Account <one>/);
  assert.doesNotMatch(h.$('rows').textContent, /Other group/);
  assert.equal(h.$('run-batch').disabled, false);
  assert.match(h.$('run-batch').textContent, /\(1\)/);
  for (const call of h.calls) {
    assert.equal(call.init.headers.Authorization, 'Bearer initial-admin');
    const url = new URL(call.url, h.window.location.origin);
    assert.equal(url.origin, h.window.location.origin);
    assert.match(url.pathname, /^\/api\/v1\/admin\//);
    assert.equal(url.searchParams.has('token'), false);
  }
  assert.equal(h.calls.some((c) => /\/accounts\/2\/codex-eval\/models/.test(c.url)), false, 'disabled accounts are not model-discovery targets');
});

test('public shell without auth rejects URL token and sends no API request', async (t) => {
  const h = await harness(t, { token: null, url: 'https://admin.example/codex-eval?token=not-auth' });
  assert.equal(h.$('app').hidden, true);
  assert.equal(h.$('login').hidden, false);
  assert.match(h.$('login-error').textContent, /请登录/);
  assert.equal(h.calls.length, 0);
});

test('real tab click and keyboard navigation hide unsupported test types everywhere', async (t) => {
  const h = await harness(t);
  assert.deepEqual([...h.document.querySelectorAll('[role=tab]')].map((e) => e.id), ['tab-candy', 'tab-fingerprint', 'tab-history', 'tab-settings']);
  assert.deepEqual([...h.$('history-type').options].map((e) => e.value), ['all', 'candy', 'fingerprint']);
  assert.deepEqual([...h.field('test_type').options].map((e) => e.value), ['candy', 'fingerprint']);
  h.$('tab-history').click();
  assert.equal(h.$('history-panel').hidden, false);
  assert.equal(h.$('candy-panel').hidden, true);
  h.$('tab-history').dispatchEvent(new h.window.KeyboardEvent('keydown', { key: 'End', bubbles: true }));
  await flush();
  assert.equal(h.$('settings-panel').hidden, false);
  assert.equal(h.$('tab-settings').getAttribute('aria-selected'), 'true');
  assert.equal(h.document.querySelector('.page-actions').hidden, true);
  assert.equal(h.document.activeElement, h.$('tab-settings'));
  assert.doesNotMatch(h.$('app').textContent, /UNSUPPORTED_SENTINEL|pelican|siwc|鹈鹕/i);
});

test('history DOM orders records, filters, expands safe markdown and opens authenticated detail', async (t) => {
  const h = await harness(t);
  h.$('tab-history').click();
  assert.equal(h.$('history-count').textContent, '2 条记录');
  const history = h.$('history-records');
  assert.ok(history.querySelector('[data-history-fp-detail]'));
  assert.match(history.firstElementChild.textContent, /GPT-6 Astra/);
  assert.equal(history.querySelector('img,script,a'), null);
  assert.equal(history.querySelector('strong').textContent, '答案：');
  history.querySelector('[data-history-answer]').click();
  assert.equal(history.querySelector('[data-history-answer]').getAttribute('aria-expanded'), 'true');
  h.$('history-search').value = 'saved/literal'; h.event(h.$('history-search'), 'input');
  assert.equal(h.$('history-count').textContent, '1 条记录');
  h.$('history-type').value = 'fingerprint'; h.event(h.$('history-type'), 'change');
  assert.equal(h.$('history-count').textContent, '0 条记录');
  h.$('history-search').value = ''; h.event(h.$('history-search'), 'input');
  history.querySelector('[data-history-fp-detail]').click(); await flush();
  assert.equal(h.$('fp-detail').open, true);
  assert.match(h.$('fp-detail-body').textContent, /private challenge/);
  assert.equal(h.$('fp-detail-body').querySelector('script'), null);
  assert.equal(h.calls.at(-1).url, BASE + '/records/fingerprint/1/record%2Fone');
  h.$('fp-detail-close').click();
  assert.equal(h.$('fp-detail').open, false);
  assert.equal(h.document.activeElement.dataset.historyFpDetail, 'record/one');
});

test('every new request uses refreshed auth token; logout clears rows, secrets and dialogs', async (t) => {
  const h = await harness(t);
  h.window.localStorage.setItem('auth_token', 'refreshed-admin');
  const before = h.calls.length;
  h.$('refresh').click(); await flush();
  assert.ok(h.calls.length > before);
  for (const call of h.calls.slice(before)) assert.equal(call.init.headers.Authorization, 'Bearer refreshed-admin');
  h.$('tab-history').click(); h.$('history-records').querySelector('[data-history-fp-detail]').click(); await flush();
  h.field('ntfy_password').value = 'typed-secret';
  await h.auth(null);
  assert.equal(h.$('app').hidden, true);
  assert.equal(h.$('login').hidden, false);
  assert.equal(h.$('fp-detail').open, false);
  assert.equal(h.field('ntfy_password').value, '');
  assert.equal(h.$('rows').querySelector('[data-row]'), null);
  assert.equal(h.$('history-count').textContent, '0 条记录');
  assert.equal([...h.timers.values()].some((task) => [400, 2500, 20000].includes(task.delay)), false);
  const loggedOutCalls = h.calls.length;
  await h.tick(20000);
  assert.equal(h.calls.length, loggedOutCalls);
  await h.auth('new-session-admin');
  assert.equal(h.$('app').hidden, false);
  assert.equal(h.$('rows').querySelectorAll('[data-row]').length, 2);
  for (const call of h.calls.slice(loggedOutCalls)) assert.equal(call.init.headers.Authorization, 'Bearer new-session-admin');
});

test('401 and 403 during startup show login/permission errors instead of usable controls', async (t) => {
  for (const status of [401, 403]) {
    const h = await harness(t, { respond: () => ({ status, data: {} }) });
    assert.equal(h.$('app').hidden, true);
    assert.equal(h.$('login').hidden, false);
    assert.match(h.$('login-error').textContent, status === 401 ? /请登录/ : /无管理员权限/);
    assert.equal(h.$('run-batch').disabled, true);
  }
});

test('settings populate real form including stored custom model and password placeholder', async (t) => {
  const h = await harness(t);
  h.$('tab-settings').click(); await flush();
  assert.equal(h.field('model').value, 'saved/literal');
  assert.equal(h.$('inspection-model-select').value, 'saved/literal');
  assert.equal(h.field('test_type').value, 'fingerprint');
  assert.equal(h.field('effort').value, 'high');
  assert.equal(h.field('ntfy_password').value, '');
  assert.match(h.field('ntfy_password').placeholder, /已配置/);
  assert.equal(h.field('bark_url').value, 'https://bark.invalid/device');
  for (const id of ['inspection-auto-fields', 'inspection-priority-fields', 'inspection-model-fields', 'inspection-notification-fields']) assert.equal(h.$(id).hidden, false, id);
  assert.equal(h.$('inspection-status').closest('section').id, 'fp-credentials');
  assert.equal(h.$('inspection-start').disabled, false);
});

test('model control DOM picker preserves literal IDs and autosaves select/add/remove changes', async (t) => {
  const h = await harness(t);
  h.$('tab-settings').click(); await flush();
  assert.match(h.$('inspection-model-tags').textContent, /saved\/blocked/);
  h.$('inspection-model-trigger').click();
  assert.equal(h.$('inspection-model-dropdown').hidden, false);
  h.$('inspection-model-options').querySelector('[data-inspection-model="provider/raw"]').click();
  h.$('inspection-model-custom').value = 'custom/restricted'; h.event(h.$('inspection-model-custom'), 'input');
  h.$('inspection-model-add').click();
  h.$('inspection-model-tags').querySelector('[data-remove-model="saved/blocked"]').click();
  await h.tick(400);
  const save = h.calls.filter((c) => c.url === SETTINGS && c.init.method === 'PUT').at(-1);
  assert.ok(save);
  assert.deepEqual(save.body.degraded_remove_models, ['provider/raw', 'custom/restricted']);
  assert.equal(save.body.control_models, true);
  assert.equal(save.body.ntfy_password, '', 'blank unchanged password delegates secret preservation to backend');
  assert.equal(save.body.bark_url, 'https://bark.invalid/device');
  assert.equal(save.body.group_id, 101);
  h.field('control_models').checked = false; h.event(h.field('control_models'), 'change');
  assert.equal(h.$('inspection-model-fields').hidden, true);
  assert.equal(h.$('inspection-model-dropdown').hidden, true);
  await h.tick(400);
  const disabled = h.calls.filter((c) => c.url === SETTINGS && c.init.method === 'PUT').at(-1);
  assert.equal(disabled.body.control_models, false);
  assert.deepEqual(disabled.body.degraded_remove_models, ['provider/raw', 'custom/restricted']);
});

test('invalid custom disabled-model names do not mutate selection or send writes', async (t) => {
  const h = await harness(t);
  for (const value of ['blacklist', 'wild*card', 'wild?card']) {
    h.$('inspection-model-custom').value = value; h.event(h.$('inspection-model-custom'), 'input');
    h.$('inspection-model-add').click();
    assert.equal(h.$('inspection-model-custom').checkValidity(), false);
    assert.deepEqual(JSON.parse(h.field('degraded_remove_models').value), ['saved/blocked']);
  }
  await h.tick(400);
  assert.equal(h.calls.some((c) => c.init.method === 'PUT'), false);
});

test('form autosave serializes switches, notification secrets and patterns; real validity blocks bad saves', async (t) => {
  const h = await harness(t);
  h.field('ntfy_password').value = 'new-secret'; h.event(h.field('ntfy_password'), 'input');
  h.field('include_patterns').value = 'codex-*, prod-*'; h.event(h.field('include_patterns'), 'input');
  h.field('test_type').value = 'candy'; h.event(h.field('test_type'), 'change');
  assert.equal(h.$('inspection-status').closest('section').id, 'candy-credentials');
  await h.tick(400);
  const saved = h.calls.filter((c) => c.init.method === 'PUT').at(-1).body;
  assert.equal(saved.ntfy_password, 'new-secret');
  assert.deepEqual(saved.include_patterns, ['codex-*', 'prod-*']);
  assert.equal(saved.test_type, 'candy');
  assert.equal(saved.adjust_priority, true); assert.equal(saved.normal_priority, 10); assert.equal(saved.degraded_priority, 99);
  const before = h.calls.length;
  h.field('min_wait_minutes').value = '100'; h.event(h.field('min_wait_minutes'), 'input');
  await h.tick(400);
  assert.equal(h.calls.length, before);
  assert.match(h.field('max_wait_minutes').validationMessage, /最长等待/);
  h.field('min_wait_minutes').value = '15'; h.event(h.field('min_wait_minutes'), 'input');
  h.field('include_patterns').value = 'broken['; h.event(h.field('include_patterns'), 'input');
  await h.tick(400);
  assert.equal(h.calls.length, before);
  assert.match(h.$('inspection-save-status').textContent, /通配符/);
});

test('manual candy and fingerprint submissions send supported types with current group and literal model', async (t) => {
  const h = await harness(t, { storage: { [PREF]: { model: 'saved/literal', effort: 'high', runs: 2 }, [`${PREF}.fingerprint`]: { model: 'custom/fp', concurrency: 2 } } });
  h.event(h.$('toolbar'), 'submit'); await flush();
  const candy = h.calls.find((c) => c.url === BASE + '/start');
  assert.deepEqual(candy.body, { type: 'candy', targets: [{ group_id: 101, account_id: 1 }], model: 'saved/literal', effort: 'high', runs: 2 });
  h.$('tab-fingerprint').click(); h.event(h.$('fp-toolbar'), 'submit'); await flush();
  const fp = h.calls.filter((c) => c.url === BASE + '/start').at(-1);
  assert.deepEqual(fp.body, { type: 'fingerprint', targets: [{ group_id: 101, account_id: 1 }], model: 'custom/fp', concurrency: 2 });
  assert.equal([...h.$('model-select').options].some((o) => o.value === 'image-gen'), false);
  assert.equal([...h.$('fp-model-select').options].some((o) => /[()]/.test(o.value)), false);
});

test('logout aborts in-flight state refresh and stale detail cannot reopen a closed dialog', async (t) => {
  let releaseState, releaseDetail;
  let holdState = false, holdDetail = false;
  const h = await harness(t, { respond(request) {
    if (holdState && request.url === BASE + '/state') return new Promise((resolve) => { releaseState = resolve; });
    if (holdDetail && request.url.includes('/records/')) return new Promise((resolve) => { releaseDetail = resolve; });
  } });
  holdDetail = true;
  h.$('tab-history').click(); h.$('history-records').querySelector('[data-history-fp-detail]').click(); await flush();
  holdState = true;
  h.$('refresh').click(); await flush();
  assert.equal(typeof releaseState, 'function'); assert.equal(typeof releaseDetail, 'function');
  await h.auth(null);
  releaseState({ status: 200, data: fixtureState() });
  releaseDetail({ status: 200, data: { fingerprint: fixtureState().fingerprint['1'][0] } });
  await flush();
  assert.equal(h.$('fp-detail').open, false);
  assert.equal(h.$('app').hidden, true);
  assert.equal(h.$('rows').querySelector('[data-row]'), null);
  assert.equal(h.$('history-count').textContent, '0 条记录');
  assert.equal([...h.timers.values()].some((timer) => timer.delay === 20000), false);
});

test('a settings response from the logged-out session must not overwrite the next session', async (t) => {
  let releaseOldSettings;
  const h = await harness(t, { respond(request) {
    if (request.url === SETTINGS && request.init.headers.Authorization === 'Bearer initial-admin') {
      return new Promise((resolve) => { releaseOldSettings = resolve; });
    }
  } });
  assert.equal(typeof releaseOldSettings, 'function');
  await h.auth(null);
  await h.auth('new-session-admin');
  assert.equal(h.$('app').hidden, false);
  assert.equal(h.$('group').value, '101');
  releaseOldSettings({ status: 200, data: { ...fixtureSettings(), group_id: 202, model: 'old-session/private-model' } });
  await flush();
  assert.deepEqual({ model: h.field('model').value, group: h.$('group').value }, { model: 'saved/literal', group: '101' });
});

test('a late settings 401 from the previous session must not log out a valid new session', async (t) => {
  let releaseOldSettings;
  const h = await harness(t, { respond(request) {
    if (request.url === SETTINGS && request.init.headers.Authorization === 'Bearer initial-admin') {
      return new Promise((resolve) => { releaseOldSettings = resolve; });
    }
  } });
  await h.auth(null);
  await h.auth('new-session-admin');
  assert.equal(h.$('app').hidden, false);
  releaseOldSettings({ status: 401, data: {} });
  await flush();
  assert.equal(h.window.localStorage.getItem('auth_token'), 'new-session-admin');
  assert.equal(h.$('app').hidden, false, 'stale unauthorized response must not block the new authenticated session');
  assert.equal(h.$('login').hidden, true);
});
