"use strict";

const $ = (id) => document.getElementById(id);

const SEVERITIES = ["critical", "error", "warning", "info"];

const state = {
  repo: "",
  repos: [],
  reviews: [],
  review: null,
  findings: [],
  selectedId: "",
  filter: { severity: "", source: "" },
};

function backend() {
  return (window.go && window.go.main && window.go.main.App) || null;
}

async function call(method, ...args) {
  const api = backend();
  if (api) {
    return api[method].apply(api, args);
  }
  return MOCK[method](...args);
}

function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined && text !== null) node.textContent = String(text);
  return node;
}

function formatTime(value) {
  if (!value) return "";
  const d = new Date(value);
  if (isNaN(d.getTime())) return String(value);
  return d.toLocaleString();
}

async function init() {
  let version = "";
  try {
    version = await call("Version");
  } catch (err) {
    version = "unknown";
  }
  $("version").textContent = version + (backend() ? "" : " (standalone preview)");
  wireEvents();
  await refreshHistory();
}

function wireEvents() {
  $("open").addEventListener("click", () => openRepo($("repo").value.trim()));
  $("run").addEventListener("click", runReview);
  $("severity").addEventListener("change", (e) => {
    state.filter.severity = e.target.value;
    renderFindings();
  });
  $("source").addEventListener("change", (e) => {
    state.filter.source = e.target.value;
    renderFindings();
  });
}

function setBusy(busy) {
  $("open").disabled = busy;
  $("run").disabled = busy;
}

function setSummary(text) {
  $("summary").textContent = text;
}

function setMode(mode) {
  const badge = $("mode");
  if (!mode) {
    badge.hidden = true;
    return;
  }
  badge.textContent = mode;
  badge.hidden = false;
}

function resetFilters() {
  state.filter = { severity: "", source: "" };
  $("severity").value = "";
  $("source").value = "";
}

function clearDetail() {
  state.selectedId = "";
  const detail = $("detail");
  detail.replaceChildren();
  detail.appendChild(el("p", "muted", "Select a finding to see its evidence and code context."));
}

function hideNotes() {
  const notes = $("notes");
  notes.hidden = true;
  notes.replaceChildren();
}

async function refreshHistory() {
  try {
    state.repos = (await call("ListRepos")) || [];
  } catch (err) {
    state.repos = [];
  }
  renderHistory();
}

async function openRepo(path) {
  if (!path) {
    setSummary("Enter a repository path.");
    return;
  }
  state.repo = path;
  state.review = null;
  state.findings = [];
  state.reviews = [];
  $("repo").value = path;
  $("filters").hidden = true;
  resetFilters();
  clearDetail();
  hideNotes();
  renderFindings();
  try {
    setMode(await call("RepoMode", path));
    setSummary("Repository " + path + " ready. Run a review.");
  } catch (err) {
    setSummary("Error: " + err);
  }
  try {
    state.reviews = (await call("ListReviews", path)) || [];
  } catch (err) {
    state.reviews = [];
  }
  renderHistory();
}

async function runReview() {
  const path = state.repo || $("repo").value.trim();
  if (!path) {
    setSummary("Enter a repository path first.");
    return;
  }
  state.repo = path;
  setBusy(true);
  setSummary("Running review on " + path + " ...");
  try {
    const review = await call("Review", path);
    showReview(review);
    await refreshHistory();
    try {
      state.reviews = (await call("ListReviews", path)) || [];
    } catch (err) {
      state.reviews = [];
    }
    renderHistory();
  } catch (err) {
    setSummary("Error: " + err);
  } finally {
    setBusy(false);
  }
}

async function loadReview(id) {
  setBusy(true);
  try {
    const review = await call("GetReview", id);
    showReview(review);
  } catch (err) {
    setSummary("Error: " + err);
  } finally {
    setBusy(false);
  }
}

function showReview(review) {
  state.review = review || null;
  state.findings = (review && review.findings) || [];
  state.repo = (review && review.repo_path) || state.repo;
  if (review && review.mode) setMode(review.mode);
  clearDetail();
  setSummary(summaryText(review));
  renderNotes(review);
  $("filters").hidden = state.findings.length === 0;
  renderFindings();
}

function summaryText(review) {
  if (!review) return "No review loaded.";
  const findings = review.findings || [];
  const counts = {};
  for (const f of findings) counts[f.severity] = (counts[f.severity] || 0) + 1;
  const parts = [];
  for (const sev of SEVERITIES) {
    if (counts[sev]) parts.push(counts[sev] + " " + sev);
  }
  const detail = parts.length ? " (" + parts.join(", ") + ")" : "";
  const analyzers = (review.analyzers_run || []).length;
  return (
    (review.mode || "offline") +
    " mode - " +
    findings.length +
    " findings" +
    detail +
    " - " +
    analyzers +
    " analyzers"
  );
}

function renderNotes(review) {
  const notes = $("notes");
  notes.replaceChildren();
  const messages = []
    .concat(((review && review.warnings) || []).map((m) => "Warning: " + m))
    .concat(((review && review.degradations) || []).map((m) => "Degradation: " + m));
  if (messages.length === 0) {
    notes.hidden = true;
    return;
  }
  notes.appendChild(el("strong", null, "Review notes"));
  const list = el("ul");
  for (const m of messages) list.appendChild(el("li", null, m));
  notes.appendChild(list);
  notes.hidden = false;
}

function visibleFindings() {
  return state.findings.filter((f) => {
    if (state.filter.severity && f.severity !== state.filter.severity) return false;
    if (state.filter.source && f.source !== state.filter.source) return false;
    return true;
  });
}

function renderFindings() {
  const root = $("findings");
  root.replaceChildren();
  if (!state.review) {
    return;
  }
  const findings = visibleFindings();
  if (findings.length === 0) {
    root.appendChild(el("p", "muted", "No findings match the current filters."));
    return;
  }
  for (const f of findings) {
    const card = el("div", "finding" + (f.id === state.selectedId ? " active" : ""));
    const head = el("div", "finding-head");
    head.appendChild(el("span", "sev sev-" + f.severity, f.severity));
    head.appendChild(el("span", null, f.source));
    if (f.rule_id) head.appendChild(el("span", null, f.rule_id));
    if (f.demoted) head.appendChild(el("span", null, "demoted"));
    card.appendChild(head);
    card.appendChild(el("p", "finding-msg", f.message));
    const loc = f.location || {};
    card.appendChild(el("div", "finding-loc", (loc.file || "?") + ":" + (loc.line_start || "?")));
    card.addEventListener("click", () => selectFinding(f));
    root.appendChild(card);
  }
}

async function selectFinding(finding) {
  state.selectedId = finding.id;
  renderFindings();
  const detail = $("detail");
  detail.replaceChildren();
  detail.appendChild(el("h2", null, finding.message || "Finding"));
  const loc = finding.location || {};
  detail.appendChild(
    el("div", "finding-loc", (loc.file || "?") + ":" + (loc.line_start || "?") + (finding.rule_id ? " - " + finding.rule_id : "")),
  );
  if (finding.reasoning) {
    detail.appendChild(el("p", null, finding.reasoning));
  }
  if (finding.evidence && finding.evidence.length) {
    detail.appendChild(el("strong", null, "Evidence"));
    const list = el("ul", "evidence");
    for (const item of finding.evidence) list.appendChild(el("li", null, item));
    detail.appendChild(list);
  }
  if (!loc.file) return;
  const code = el("div", "code");
  code.appendChild(el("p", "muted", "Loading context..."));
  detail.appendChild(code);
  try {
    const lines = await call("ReviewContext", state.repo, loc.file, loc.line_start || 0, loc.line_end || 0, 3);
    code.replaceChildren();
    for (const line of lines || []) {
      const hit = line.number >= (loc.line_start || 0) && line.number <= (loc.line_end || loc.line_start || 0);
      const row = el("div", "code-line" + (hit ? " hit" : ""));
      row.appendChild(el("span", "ln", line.number));
      row.appendChild(el("span", "tx", line.text));
      code.appendChild(row);
    }
    if (!lines || lines.length === 0) code.appendChild(el("p", "muted", "No lines available."));
  } catch (err) {
    code.replaceChildren(el("p", "error", "Could not load context: " + err));
  }
}

function renderHistory() {
  const root = $("repos");
  root.replaceChildren();
  if (!state.repos || state.repos.length === 0) {
    root.appendChild(el("p", "muted", "No reviews yet."));
    return;
  }
  for (const repo of state.repos) {
    const item = el("div", "repo-item");
    const btn = el("button", "repo" + (repo.repo_path === state.repo ? " active" : ""));
    btn.type = "button";
    btn.appendChild(el("span", "repo-path", repo.repo_path));
    btn.appendChild(
      el(
        "span",
        "repo-meta",
        (repo.mode || "offline") +
          " - " +
          repo.review_count +
          " reviews - " +
          repo.last_findings +
          " findings - " +
          formatTime(repo.last_started_at),
      ),
    );
    btn.addEventListener("click", () => openRepo(repo.repo_path));
    item.appendChild(btn);

    if (repo.repo_path === state.repo && state.reviews) {
      const list = el("div", "repo-reviews");
      if (state.reviews.length === 0) {
        list.appendChild(el("p", "muted", "No stored reviews."));
      }
      for (const rev of state.reviews) {
        const rb = el("button", "review" + (state.review && state.review.id === rev.id ? " active" : ""));
        rb.type = "button";
        rb.appendChild(
          el(
            "span",
            "review-meta",
            (rev.mode || "offline") + " - " + formatTime(rev.started_at) + " - " + rev.findings_count + " findings",
          ),
        );
        rb.addEventListener("click", () => loadReview(rev.id));
        list.appendChild(rb);
      }
      item.appendChild(list);
    }
    root.appendChild(item);
  }
}

const MOCK_FINDINGS = [
  {
    id: "f1",
    rule_id: "secret-detection",
    category: "security",
    severity: "critical",
    source: "analyzer",
    location: { file: "internal/config/config.go", line_start: 42, line_end: 42 },
    message: "Hardcoded provider API key",
    reasoning: "Credentials in source are exfiltrable and must live in the OS keyring or a local env file.",
    evidence: ['apiKey = "AKIA...REDACTED"'],
    confidence: 1,
    demoted: false,
  },
  {
    id: "f2",
    rule_id: "",
    category: "correctness",
    severity: "warning",
    source: "llm",
    location: { file: "internal/review/engine.go", line_start: 88, line_end: 90 },
    message: "Analyzer error path drops the finding context",
    reasoning: "The degradation note records the analyzer failure but not which files were mid-analysis.",
    evidence: [],
    confidence: 0.6,
    demoted: false,
  },
  {
    id: "f3",
    rule_id: "no-merge-conflict-markers",
    category: "correctness",
    severity: "error",
    source: "rule",
    location: { file: "README.md", line_start: 12, line_end: 12 },
    message: "Merge conflict marker left in the file",
    reasoning: "Conflict markers must be resolved before commit.",
    evidence: [],
    confidence: 1,
    demoted: false,
  },
];

function mockReview(repoPath) {
  const now = new Date().toISOString();
  return {
    id: "preview-review",
    repo_path: repoPath || "/home/dev/acme",
    mode: "offline",
    started_at: now,
    finished_at: now,
    findings: MOCK_FINDINGS,
    analyzers_run: ["secret-detection", "go-vet", "rules"],
    warnings: [],
    degradations: ["LLM stage skipped: no local model reachable"],
  };
}

const MOCK = {
  Version: async () => "0.1.0-preview",
  RepoMode: async () => "offline",
  ListRepos: async () => [
    {
      repo_path: "/home/dev/acme",
      mode: "offline",
      last_started_at: new Date().toISOString(),
      last_findings: 3,
      review_count: 5,
    },
    {
      repo_path: "/home/dev/site",
      mode: "offline",
      last_started_at: new Date(Date.now() - 86400000).toISOString(),
      last_findings: 0,
      review_count: 1,
    },
  ],
  ListReviews: async () => [
    { id: "preview-review", mode: "offline", started_at: new Date().toISOString(), finished_at: new Date().toISOString(), findings_count: 3 },
    { id: "older-review", mode: "offline", started_at: new Date(Date.now() - 86400000).toISOString(), finished_at: new Date(Date.now() - 86400000).toISOString(), findings_count: 0 },
  ],
  Review: async (repoPath) => mockReview(repoPath),
  GetReview: async () => mockReview(),
  ReviewContext: async (repoPath, file, lineStart, lineEnd, radius) => {
    const total = 60;
    const r = radius > 0 ? radius : 3;
    let start = Math.max(1, (lineStart || 1) - r);
    let end = Math.min(total, (lineEnd || lineStart || 1) + r);
    if (start > end) {
      start = Math.max(1, total - r);
      end = total;
    }
    const lines = [];
    for (let n = start; n <= end; n++) {
      lines.push({ number: n, text: "// preview: " + file + " line " + n });
    }
    return lines;
  },
};

init();
