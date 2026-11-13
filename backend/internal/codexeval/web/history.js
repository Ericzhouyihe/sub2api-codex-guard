"use strict";

// History uses the same current-group inventory as the manual test pages. Full
// fingerprint samples remain on the authenticated records endpoint, not in HTML.
function historyRecords() {
  const type = $("history-type").value;
  const query = $("history-search").value.trim().toLowerCase();
  return credentials.flatMap((account) => [
    ...(type === "fingerprint" ? [] : account.results.map((record) => ({ account, record, type: "candy" }))),
    ...(type === "candy" ? [] : account.fingerprints.map((record) => ({ account, record, type: "fingerprint" }))),
  ]).filter(({ account, record }) => !query || `${account.name} ${record.model || ""}`.toLowerCase().includes(query))
    .sort((a, b) => (Date.parse(b.record.time) || 0) - (Date.parse(a.record.time) || 0));
}
function renderHistory() {
  const records = historyRecords();
  $("history-count").textContent = `${records.length} 条记录`;
  $("history-records").innerHTML = records.map(({ account, record, type }, index) => {
    const title = `<span class="history-account name">${esc(account.name)}</span><span>${type === "candy" ? "糖果" : "指纹"}</span>`;
    if (type === "fingerprint") return historyEntry("history-fp", account.id, record, fpOutcome(record), title);
    const key = answerKey(account.id, record), open = candyAnswersExpanded.has(key);
    return historyCard(record, `<div class="answer ${open ? "expanded" : ""}">${verdict(record)}
      <button class="answer-toggle" type="button" data-history-answer="${esc(key)}" aria-expanded="${open}" aria-controls="history-answer-${index}" aria-label="${open ? "收起文本" : "展开文本"}">${answerToggle(open)}</button>
      <div id="history-answer-${index}" class="answer-content ${record.error ? "error" : "markdown"}">${answerHTML(record)}</div></div>`, title);
  }).join("") || '<div class="empty">暂无符合条件的糖果或指纹记录。</div>';
}
function initializeHistory() {
  $("history-type").addEventListener("change", renderHistory);
  $("history-search").addEventListener("input", renderHistory);
  $("fp-detail").addEventListener("close", () => {
    if ($("history-panel").hidden) return;
    const { record, credential } = $("fp-detail").dataset;
    [...$("history-records").querySelectorAll("[data-history-fp-detail]")]
      .find((button) => button.dataset.historyFpDetail === record && button.dataset.historyFpCredential === credential)?.focus();
  });
  $("history-records").addEventListener("click", (event) => {
    const detail = event.target.closest("[data-history-fp-detail]");
    if (detail) return openResultDetail("fp", "fingerprint", detail.dataset.historyFpCredential, detail.dataset.historyFpDetail, showFingerprintDetail);
    const toggle = event.target.closest("[data-history-answer]");
    if (!toggle) return;
    const key = toggle.dataset.historyAnswer;
    candyAnswersExpanded.has(key) ? candyAnswersExpanded.delete(key) : candyAnswersExpanded.add(key);
    renderHistory();
  });
}
