import { For, Show, createSignal, onMount } from "solid-js";
import { apiGetSelfCheck } from "../functions/api";
import type { SelfCheck as SelfCheckResult } from "../functions/exports";

function SelfCheck(props: { compact?: boolean }) {
  const [check, setCheck] = createSignal<SelfCheckResult | null>(null);
  const [loading, setLoading] = createSignal(false);

  const refresh = async () => {
    setLoading(true);
    try {
      setCheck(await apiGetSelfCheck());
    } finally {
      setLoading(false);
    }
  };

  onMount(() => {
    refresh();
  });

  const visibleItems = () => {
    const result = check();
    if (result === null) {
      return [];
    }
    if (props.compact) {
      return result.Items.filter(item => item.Status !== "ok");
    }
    return result.Items;
  };

  const hasVisibleItems = () => visibleItems().length > 0;
  const borderClass = () => check()?.OK ? "border-success" : "border-danger";
  const badgeClass = (status: string) => {
    switch (status) {
      case "error":
        return "text-bg-danger";
      case "warn":
        return "text-bg-warning";
      default:
        return "text-bg-success";
    }
  };

  return (
    <Show when={hasVisibleItems()}>
      <div class={"card " + borderClass()}>
        <div class="card-header d-flex justify-content-between align-items-center">
          <span>Self-check</span>
          <button class="btn btn-outline-primary btn-sm" title="Run self-check" onClick={refresh} disabled={loading()}>
            <i class={loading() ? "spinner-border spinner-border-sm" : "bi bi-arrow-clockwise"}></i>
          </button>
        </div>
        <div class="card-body">
          <For each={visibleItems()}>{item =>
            <div class="mb-3">
              <div class="d-flex gap-2 align-items-center">
                <span class={"badge " + badgeClass(item.Status)}>{item.Status}</span>
                <strong>{item.Name}</strong>
              </div>
              <div class="mt-1">{item.Detail}</div>
              <Show when={item.Fix !== ""}>
                <pre class="mb-0 mt-2 p-2 rounded bg-body-tertiary"><code>{item.Fix}</code></pre>
              </Show>
            </div>
          }</For>
        </div>
      </div>
    </Show>
  );
}

export default SelfCheck;
