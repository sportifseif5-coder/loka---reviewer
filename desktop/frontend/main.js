"use strict";

const $ = (id) => document.getElementById(id);

function hasBackend() {
  return typeof window.go !== "undefined" && window.go.main;
}

async function init() {
  if (!hasBackend()) {
    $("version").textContent = "browser (standalone preview)";
    return;
  }
  const v = await window.go.main.App.Version();
  $("version").textContent = v;
}

$("run").addEventListener("click", async () => {
  const path = $("repo").value.trim();
  $("summary").textContent = "Running review...";
  $("findings").replaceChildren();
  try {
    const result = hasBackend()
      ? await window.go.main.App.Review(path)
      : { findings: [], mode: "offline" };
    $("summary").textContent =
      result.mode + " mode — " + result.findings.length + " findings";
    for (const f of result.findings) {
      const el = document.createElement("div");
      el.className = "finding";
      el.textContent =
        (f.file || f.rule || "finding") +
        " — " +
        (f.message || f.severity || "…");
      $("findings").appendChild(el);
    }
  } catch (err) {
    $("summary").textContent = "Error: " + err;
  }
});

init();
