(() => {
  const cardID = "watchyourlan-selfcheck-card";
  let timer = 0;
  let loading = false;

  function escapeHTML(value) {
    return String(value || "")
      .replaceAll("&", "&amp;")
      .replaceAll("<", "&lt;")
      .replaceAll(">", "&gt;")
      .replaceAll('"', "&quot;")
      .replaceAll("'", "&#39;");
  }

  function badgeClass(status) {
    if (status === "error") {
      return "text-bg-danger";
    }
    if (status === "warn") {
      return "text-bg-warning";
    }
    return "text-bg-success";
  }

  function borderClass(result) {
    return result && result.OK ? "border-success" : "border-danger";
  }

  function mountPoint() {
    return document.querySelector("#root .container-lg > .row > .col-md");
  }

  function isConfigPage() {
    return window.location.pathname.startsWith("/config");
  }

  function selectedItems(result) {
    const items = result && Array.isArray(result.Items) ? result.Items : [];
    if (isConfigPage()) {
      return items;
    }
    return items.filter((item) => item.Status !== "ok");
  }

  function render(result) {
    const mount = mountPoint();
    if (!mount) {
      return;
    }

    const oldCard = document.getElementById(cardID);
    if (oldCard) {
      oldCard.remove();
    }

    const items = selectedItems(result);
    if (items.length === 0) {
      return;
    }

    const rows = items.map((item) => {
      const fix = item.Fix
        ? `<pre class="mb-0 mt-2 p-2 rounded bg-body-tertiary"><code>${escapeHTML(item.Fix)}</code></pre>`
        : "";

      return `
        <div class="mb-3">
          <div class="d-flex gap-2 align-items-center">
            <span class="badge ${badgeClass(item.Status)}">${escapeHTML(item.Status)}</span>
            <strong>${escapeHTML(item.Name)}</strong>
          </div>
          <div class="mt-1">${escapeHTML(item.Detail)}</div>
          ${fix}
        </div>
      `;
    }).join("");

    const wrapper = document.createElement("div");
    wrapper.id = cardID;
    wrapper.className = "mb-4";
    wrapper.innerHTML = `
      <div class="card ${borderClass(result)}">
        <div class="card-header d-flex justify-content-between align-items-center">
          <span>Self-check</span>
          <button class="btn btn-outline-primary btn-sm" type="button" title="Run self-check">
            <i class="bi bi-arrow-clockwise"></i>
          </button>
        </div>
        <div class="card-body">${rows}</div>
      </div>
    `;

    wrapper.querySelector("button").addEventListener("click", refresh);
    mount.prepend(wrapper);
  }

  async function refresh() {
    if (loading) {
      return;
    }

    loading = true;
    try {
      const response = await fetch("/api/selfcheck", { cache: "no-store" });
      if (response.ok) {
        render(await response.json());
      }
    } finally {
      loading = false;
    }
  }

  function schedule() {
    window.clearTimeout(timer);
    timer = window.setTimeout(refresh, 250);
  }

  const pushState = history.pushState;
  history.pushState = function (...args) {
    const result = pushState.apply(this, args);
    schedule();
    return result;
  };

  const replaceState = history.replaceState;
  history.replaceState = function (...args) {
    const result = replaceState.apply(this, args);
    schedule();
    return result;
  };

  window.addEventListener("popstate", schedule);
  window.addEventListener("load", schedule);
  window.setInterval(refresh, 60000);

  new MutationObserver(schedule).observe(document.body, {
    childList: true,
    subtree: true,
  });
})();
