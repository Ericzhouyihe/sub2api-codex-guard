// Run: node --test tests/model-guard-ui.test.cjs
// Dependency-free coverage for the native embedded UI; kept outside go:embed.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { test } = require('node:test');
const root = path.resolve(__dirname, '..');
const web = path.join(root, 'backend/internal/codexeval/web');
const read = (name) => fs.readFileSync(path.join(web, name), 'utf8');

function harness() {
  const nodes = new Map();
  const node = (id) => {
    if (!nodes.has(id)) nodes.set(id, { value: '', checked: false, hidden: false, dataset: {}, innerHTML: '', textContent: '',
      setAttribute(name, value) { this[name] = value; }, classList: { toggle() {} } });
    return nodes.get(id);
  };
  const values = new Map();
  const context = vm.createContext({ console, URL, URLSearchParams, AbortSignal, AbortController, atob, btoa,
    setTimeout, clearTimeout, document: { getElementById: node, compatMode: 'CSS1Compat' },
    localStorage: { getItem: (key) => values.get(key) ?? null, setItem: (key, value) => values.set(key, value) } });
  context.window = context;
  const files = ['vendor/markdown-it.min.js', 'vendor/markdown-it-cjk-friendly.min.js', 'vendor/texmath.js', 'vendor/temml.min.js',
    'storage.js', 'credentials.js', 'components.js', 'catalog.js', 'candy.js', 'fingerprint.js', 'history.js', 'inspection.js'];
  vm.runInContext(files.map((name) => read(name).replace('/*FINGERPRINT_CONFIG*/{}', JSON.stringify({ requests: 3, aliases: { 'gpt-6.1-sol': 'gpt-6-astra' } })))
    .concat(read('app.js').split('// Events and initialization.')[0]).join('\n'), context);
  return { node, values, context, run: (code) => vm.runInContext(code, context) };
}

test('only supported tabs, assets and explicit embed files are shipped', () => {
  const html = read('ui.html');
  assert.match(html, /模型哨兵 \/ Model Guard/);
  assert.deepEqual([...html.matchAll(/id="tab-([^"]+)"/g)].map((m) => m[1]), ['candy', 'fingerprint', 'history', 'settings']);
  const go = fs.readFileSync(path.join(web, '../web.go'), 'utf8');
  for (const file of fs.readdirSync(web).filter((name) => /\.(js|html|css)$/.test(name))) {
    assert.doesNotMatch(read(file), /pelican|siwc|鹈鹕|SIWC/i, file);
  }
  assert.doesNotMatch(go, /pelican|siwc|web\/\*/i);
  assert.match(go, /go:embed web\/history.js/);
  for (const [, file] of go.matchAll(/\/\/go:embed (\S+)/g)) assert.ok(fs.existsSync(path.join(web, '..', file)), file);
  assert.equal(fs.existsSync(path.join(web, 'model_guard_ui_test.cjs')), false);
});

test('same-origin iframe uses current helper signature without URL credentials', () => {
  const view = fs.readFileSync(path.join(root, 'frontend/src/views/admin/CodexEvalView.vue'), 'utf8');
  assert.match(view, /new URL\('\/codex-eval', window.location.origin\)/);
  assert.match(view, /undefined,\s+pageTheme.value,\s+locale.value,/);
  assert.doesNotMatch(view, /authStore\.token|searchParams\.set\(['"]token/);
  const shared = fs.readFileSync(path.join(root, 'frontend/src/utils/embedded-url.ts'), 'utf8');
  assert.match(shared, /searchParams.delete\('token'\)/);
});

test('admin API reads latest auth-store token for every request, never URL token', async () => {
  const h = harness();
  h.values.set('auth_token', 'first-admin-session');
  const calls = [];
  h.context.fetch = async (url, options) => { calls.push({ url, options }); return { ok: true, status: 200, text: async () => '{"data":{"ok":true}}' }; };
  assert.equal((await h.run('api(BASE + "/state")')).ok, true);
  h.values.set('auth_token', 'refreshed-admin-session');
  await h.run('api(BASE + "/start", {method: "POST", body: {type: "candy"}})');
  assert.equal(calls[0].options.headers.Authorization, 'Bearer first-admin-session');
  assert.equal(calls[1].options.headers.Authorization, 'Bearer refreshed-admin-session');
  assert.equal(calls[1].options.headers['Content-Type'], 'application/json');
  assert.equal(new URL(calls[0].url, 'https://example.invalid').searchParams.has('token'), false);
  h.values.delete('auth_token');
  await assert.rejects(h.run('api(BASE + "/state")'), /请登录/);
  assert.equal(calls.length, 2);
});

test('admin API rejects forbidden and invalid responses', async () => {
  const h = harness(); h.values.set('auth_token', 'admin');
  h.context.fetch = async () => ({ ok: false, status: 403, text: async () => '{}' });
  await assert.rejects(h.run('api(BASE + "/state")'), /无管理员权限/);
  h.context.fetch = async () => ({ ok: true, status: 200, text: async () => 'not json' });
  await assert.rejects(h.run('api(BASE + "/state")'), /无效数据/);
});

test('batch selection respects visible filters, selection, account availability and group', () => {
  const h = harness();
  h.node('credential-type').value = 'all'; h.node('credential-plan').value = 'all';
  h.run(`selectedGroup=101; accountListReady=true; accountListGroup=101;
    accountList=[{id:1,group_ids:[101]},{id:2,group_ids:[202]},{id:3,group_ids:[101]}];
    credentials=[{id:'1',type:'apikey',disabled:false},{id:'2',type:'oauth',disabled:false},
      {id:'3',type:'apikey',disabled:true},{id:'4',type:'apikey',unavailable:true},
      {id:'5',type:'apikey',running:{done:0,total:1}}];`);
  assert.equal(h.run('batchCredentials("", candySelected).map(a=>a.id).sort().join(",")'), '1,2');
  h.node('credential-type').value = 'apikey';
  assert.equal(h.run('batchCredentials("", candySelected).map(a=>a.id).join(",")'), '1');
  h.run('candySelected.add("2")');
  assert.equal(h.run('batchCredentials("", candySelected).map(a=>a.id).join(",")'), '1', 'hidden selections must not leak into batches');
  assert.equal(h.run('targetsFor(["1","2","3"]).map(t=>t.account_id).join(",")'), '1,3');
});

test('current-group state preserves candy and fingerprint records and busy status', () => {
  const h = harness();
  const result = h.run(`selectedGroup=101; accountList=[{id:1,name:'one',group_ids:[101],status:'active'}, {id:2,group_ids:[202],status:'active'}];
    stateCredentials({active:[{id:'run',type:'fingerprint',target:{account_id:1,group_id:101},progress:{done:1,total:3},status:'cancelling'}],
    candy:{'1':[{answer:'21',ok:true}]},fingerprint:{'1':[{id:'fp-1'}]}})`);
  assert.equal(result.length, 1);
  assert.equal(result[0].results[0].answer, '21');
  assert.equal(result[0].fingerprints[0].id, 'fp-1');
  assert.equal(result[0].fingerprint_running.phase, 'cancelling');
  assert.equal(result[0].autoInspectionEnabled, true);
});

test('history combines supported records newest first and filters account/model/type', () => {
  const h = harness(); h.node('history-type').value = 'all';
  h.run(`credentials=[{id:'1',name:'Account <one>',results:[{time:'2026-10-01',model:'saved/literal',answer:'21',ok:true}],
    fingerprints:[{id:'fp-1',time:'2026-10-02',model:'gpt-6.1-sol',status:'completed',attribution:{prediction:'gpt-6-astra',prediction_name:'GPT-6 Astra'}}]}]`);
  assert.equal(h.run('historyRecords().map(r=>r.type).join(",")'), 'fingerprint,candy');
  h.run('renderHistory()');
  assert.match(h.node('history-records').innerHTML, /data-history-fp-detail="fp-1"/);
  assert.match(h.node('history-records').innerHTML, /Account &lt;one&gt;/);
  assert.match(h.node('history-records').innerHTML, /data-history-answer=/);
  h.node('history-type').value = 'candy';
  assert.equal(h.run('historyRecords().length'), 1);
  h.node('history-search').value = 'saved/literal';
  assert.equal(h.run('historyRecords().length'), 1);
  h.node('history-search').value = 'missing';
  h.run('renderHistory()');
  assert.equal(h.node('history-count').textContent, '0 条记录');
});

test('candy answers retain safe markdown and math without active upstream HTML', () => {
  const h = harness();
  const html = h.run('answerHTML({answer:"<script>alert(1)</script> **答案：**21 [click](https://evil.invalid) ![x](https://evil.invalid/x)"})');
  assert.doesNotMatch(html, /<script|<a |<img /);
  assert.match(html, /<strong>答案：<\/strong>21/);
  assert.match(html, /&lt;script&gt;/);
});

test('fingerprint aliases, null probability and cancelled detail remain supported', () => {
  const h = harness();
  assert.match(h.run('fpOutcome({model:"openai/gpt-6.1-sol",status:"partial",attribution:{prediction:"gpt-6-astra",prediction_name:"GPT",probability:null}})'), /与测试模型一致/);
  h.run('showFingerprintDetail({model:"custom",status:"cancelled",time:"2026-10-02",duration_ms:12,samples:[],attribution:{results:[]}})');
  assert.match(h.node('fp-detail-body').innerHTML, /测试已停止/);
  assert.doesNotMatch(h.node('fp-detail-body').innerHTML, /候选模型排行/);
});

test('custom model pickers preserve literal IDs absent from discovered options', () => {
  const h = harness();
  h.run('modelPickers.set("model", {input:document.getElementById("model"),select:document.getElementById("model-select"),models:[],custom:false})');
  h.run('updateModelPicker("model", ["provider/raw"], "saved/literal")');
  assert.equal(h.node('model').value, 'saved/literal');
  assert.match(h.node('model-select').innerHTML, /saved\/literal/);
  assert.match(h.node('model-select').innerHTML, /provider\/raw/);
  h.run('updateModelPicker("model", ["new/model"])');
  assert.equal(h.node('model').value, 'saved/literal');
});

test('inspection settings retain model control, priority, notification and validation', () => {
  const html = read('ui.html');
  for (const name of ['test_type','control_models','degraded_remove_models','adjust_priority','normal_priority','degraded_priority',
    'notify_enabled','ntfy_server','ntfy_topic','ntfy_username','ntfy_password','bark_url','include_patterns','exclude_patterns']) {
    assert.match(html, new RegExp(`name="${name}"`));
  }
  const h = harness();
  assert.equal(h.run('validWildcardPatterns("codex-*, *@example.com")'), true);
  assert.equal(h.run('validWildcardPatterns("broken[")'), false);
  assert.equal(h.run('JSON.stringify(parseInspectionPatterns("codex-*, test-*\\n foo"))'), '["codex-*","test-*","foo"]');
});

test('inspection save serializes automatic control/priority/notification and preserves stored password', async () => {
  const h = harness();
  const fields = [
    ['model', 'custom/model'], ['effort', 'high'], ['test_type', 'fingerprint'],
    ['max_tests_per_account', '3', 'number'], ['min_wait_minutes', '15', 'number'], ['max_wait_minutes', '35', 'number'],
    ['adjust_priority', '', 'checkbox', true], ['normal_priority', '10', 'number'], ['degraded_priority', '99', 'number'],
    ['control_models', '', 'checkbox', true], ['degraded_remove_models', '["custom/restricted"]', 'hidden'],
    ['include_patterns', 'codex-*, prod-*'], ['exclude_patterns', '*-backup'],
    ['notify_enabled', '', 'checkbox', true], ['ntfy_server', 'https://ntfy.sh'], ['ntfy_topic', 'model-guard'],
    ['ntfy_username', 'operator'], ['ntfy_password', '', 'password'], ['bark_url', 'https://bark.invalid/device', 'password'],
  ].map(([name, value, type = 'text', checked = false]) => ({ name, value, type, checked,
    setCustomValidity(message) { this.validationMessage = message; }, willValidate: true, checkValidity() { return !this.validationMessage; } }));
  fields.namedItem = (name) => fields.find((field) => field.name === name);
  h.node('inspection-settings').elements = fields;
  h.node('inspection-enabled').checked = true;
  h.node('credential-type').value = 'all'; h.node('credential-plan').value = 'all'; h.node('enabled-only').checked = true;
  let saved;
  h.context.capture = (body) => { saved = body; };
  h.run(`selectedGroup=101; inspectionSettingsLoaded=true; inspectionDirtyFields.set('model',1);
    fillInspectionSettings=()=>{}; loadInspectionState=async()=>{};
    api=async(path,options)=>{capture(options.body);return options.body}`);
  assert.equal(await h.run('saveInspectionSettings()'), true);
  assert.equal(saved.model, 'custom/model'); assert.equal(saved.test_type, 'fingerprint');
  assert.equal(saved.group_id, 101); assert.equal(saved.enabled_only, true);
  assert.equal(saved.adjust_priority, true); assert.equal(saved.normal_priority, 10); assert.equal(saved.degraded_priority, 99);
  assert.equal(saved.control_models, true); assert.equal(Array.from(saved.degraded_remove_models).join(','), 'custom/restricted');
  assert.equal(saved.notify_enabled, true); assert.equal(saved.ntfy_topic, 'model-guard');
  assert.equal(saved.ntfy_password, '', 'untouched blank password preserves backend secret');
  assert.equal(saved.bark_url, 'https://bark.invalid/device');
  assert.equal(Array.from(saved.include_patterns).join(','), 'codex-*,prod-*');
});

test('fingerprint history detail uses authenticated record endpoint and ignores stale dialog responses', async () => {
  const h = harness();
  const dialog = h.node('fp-detail'); dialog.open = true;
  let requestPath;
  h.context.fixture = async (url) => { requestPath = url; return { fingerprint: { id: 'record/one' } }; };
  h.run('api=window.fixture');
  let shown;
  h.context.showFixture = (record) => { shown = record; };
  await h.run('openResultDetail("fp", "fingerprint", "11", "record/one", window.showFixture)');
  assert.equal(requestPath, '/api/v1/admin/codex-eval/records/fingerprint/11/record%2Fone');
  assert.equal(shown.id, 'record/one');
  shown = undefined;
  h.context.fixture = async () => { dialog.dataset.record = 'newer-record'; return { fingerprint: { id: 'stale' } }; };
  h.run('api=window.fixture');
  await h.run('openResultDetail("fp", "fingerprint", "11", "old-record", window.showFixture)');
  assert.equal(shown, undefined);
});
