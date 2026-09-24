import { useCallback, useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { listRecommendations, selectRecommendation } from "../api/client";
import type { RecommendationDTO, SelectRequest } from "../api/types";
import { DeltaBadge } from "../components/DeltaBadge";
import { EligibilityBadge } from "../components/EligibilityBadge";
import { RecommendationCard } from "../components/RecommendationCard";
import { ResourceCheckbox } from "../components/ResourceCheckbox";
import {
  SelectionSummary,
  type SelectedRow,
} from "../components/SelectionSummary";
import { StatusBadge } from "../components/StatusBadge";
import { planSelection } from "../lib/selectionPlan";
import { useLimitSettings } from "../lib/useLimitSettings";
import { useRequestHeadroom } from "../lib/useRequestHeadroom";

type RowKey = string;
type ViewMode = "table" | "cards";

interface RowSelection {
  cpu: boolean;
  memory: boolean;
}

function rowKey(
  item: Pick<RecommendationDTO, "namespace" | "vpaName" | "containerName">,
): RowKey {
  return `${item.namespace}/${item.vpaName}/${item.containerName}`;
}

const EMPTY_SELECTION: RowSelection = { cpu: false, memory: false };

type SortKey =
  | "namespace"
  | "vpaName"
  | "workload"
  | "updateMode"
  | "cpuDelta"
  | "memoryDelta"
  | "eligibility"
  | "status";
type SortDirection = "asc" | "desc";

interface SortState {
  key: SortKey;
  direction: SortDirection;
}

const SORT_COLUMNS: { key: SortKey; label: string }[] = [
  { key: "namespace", label: "Namespace" },
  { key: "vpaName", label: "VPA" },
  { key: "workload", label: "Workload" },
  { key: "updateMode", label: "Mode" },
  { key: "cpuDelta", label: "CPU" },
  { key: "memoryDelta", label: "Memory" },
  { key: "eligibility", label: "Eligibility" },
  { key: "status", label: "Status" },
];

function sortValue(
  item: RecommendationDTO,
  key: SortKey,
): string | number | null {
  switch (key) {
    case "namespace":
      return item.namespace;
    case "vpaName":
      return item.vpaName;
    case "workload":
      return `${item.workload.kind}/${item.workload.name}`;
    case "updateMode":
      return item.updateMode;
    case "cpuDelta":
      return item.deltaCpuPercent ?? null;
    case "memoryDelta":
      return item.deltaMemoryPercent ?? null;
    case "eligibility":
      return item.eligible ? 1 : 0;
    case "status":
      return item.status;
  }
}

// Rows with no value for the sorted column (e.g. no memory delta) always
// sort to the end, regardless of direction -- flipping direction reverses
// the ranked rows, not where the unranked ones land.
function compareBySort(
  a: RecommendationDTO,
  b: RecommendationDTO,
  sort: SortState,
): number {
  const av = sortValue(a, sort.key);
  const bv = sortValue(b, sort.key);

  let cmp: number;
  if (av === null && bv === null) {
    cmp = 0;
  } else if (av === null) {
    return 1;
  } else if (bv === null) {
    return -1;
  } else {
    const raw =
      typeof av === "number" && typeof bv === "number"
        ? av - bv
        : String(av).localeCompare(String(bv));
    cmp = sort.direction === "asc" ? raw : -raw;
  }

  if (cmp !== 0) return cmp;

  // Tie-breaker: rows equal on the sorted column keep a fixed relative
  // order regardless of which order the backend happened to return them in
  // (its cache is a Go map, so that order isn't guaranteed stable across
  // polls) -- without this, tied rows would visibly shuffle every few
  // seconds as the list re-polls in the background.
  return rowKey(a).localeCompare(rowKey(b));
}

export function RecommendationList() {
  const [items, setItems] = useState<RecommendationDTO[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  const [selection, setSelection] = useState<Record<RowKey, RowSelection>>({});
  const [rowBusy, setRowBusy] = useState<Record<RowKey, boolean>>({});
  const [rowError, setRowError] = useState<Record<RowKey, string>>({});

  const [summaryBusy, setSummaryBusy] = useState(false);
  const [summaryError, setSummaryError] = useState<string | null>(null);
  const limits = useLimitSettings();
  const headroom = useRequestHeadroom();

  const [sort, setSort] = useState<SortState>({
    key: "namespace",
    direction: "asc",
  });
  const [view, setView] = useState<ViewMode>("table");
  const [summaryExpanded, setSummaryExpanded] = useState(false);

  function toggleSort(key: SortKey) {
    setSort((prev) =>
      prev.key === key
        ? { key, direction: prev.direction === "asc" ? "desc" : "asc" }
        : { key, direction: "asc" },
    );
  }

  const sortedItems = useMemo(() => {
    if (!items) return [];
    return [...items].sort((a, b) => compareBySort(a, b, sort));
  }, [items, sort]);

  const load = useCallback(() => {
    return listRecommendations().then((res) => setItems(res.items));
  }, []);

  useEffect(() => {
    let cancelled = false;
    load().catch((err) => {
      if (!cancelled) setError(String(err));
    });
    return () => {
      cancelled = true;
    };
  }, [load]);

  // The write-back worker picks up a "selected" item asynchronously (up to
  // PollInterval later, see internal/writebackworker) and moves it through
  // applying -> applied/failed. Poll while anything is in one of those
  // in-progress states so that transition shows up without a manual refresh;
  // stop as soon as nothing is pending, rather than polling forever.
  const hasInFlightWriteBack = (items ?? []).some(
    (item) => item.status === "selected" || item.status === "applying",
  );
  useEffect(() => {
    if (!hasInFlightWriteBack) return;
    const id = setInterval(() => {
      load().catch(() => {
        // A transient poll failure isn't worth surfacing over the existing
        // view; the next tick (or a manual reload) will recover.
      });
    }, 4000);
    return () => clearInterval(id);
  }, [hasInFlightWriteBack, load]);

  // The request headroom and limit specs to send for one row's selection
  // (see planSelection). null while a setting that is to be sent is invalid.
  function limitSpecsFor(
    item: RecommendationDTO,
    sel: RowSelection,
  ): Omit<SelectRequest, "applyCPU" | "applyMemory"> | null {
    const plan = planSelection(item, sel, limits, headroom);
    return plan.invalid ? null : plan.body;
  }

  function toggle(key: RowKey, resource: "cpu" | "memory", checked: boolean) {
    setSelection((prev) => ({
      ...prev,
      [key]: { ...(prev[key] ?? EMPTY_SELECTION), [resource]: checked },
    }));
  }

  async function acceptRow(item: RecommendationDTO) {
    const key = rowKey(item);
    const sel = selection[key] ?? EMPTY_SELECTION;
    if (!sel.cpu && !sel.memory) return;
    const limitSpecs = limitSpecsFor(item, sel);
    if (!limitSpecs) {
      setRowError((prev) => ({ ...prev, [key]: "invalid headroom or limit setting" }));
      return;
    }

    setRowBusy((prev) => ({ ...prev, [key]: true }));
    setRowError((prev) => {
      const next = { ...prev };
      delete next[key];
      return next;
    });
    try {
      await selectRecommendation(
        item.namespace,
        item.vpaName,
        item.containerName,
        { applyCPU: sel.cpu, applyMemory: sel.memory, ...limitSpecs },
      );
      setSelection((prev) => {
        const next = { ...prev };
        delete next[key];
        return next;
      });
      await load();
    } catch (err) {
      setRowError((prev) => ({ ...prev, [key]: String(err) }));
    } finally {
      setRowBusy((prev) => ({ ...prev, [key]: false }));
    }
  }

  // Checks the CPU/memory boxes for every eligible row, same as ticking each
  // one by hand -- it never calls the API itself. Existing selections are
  // kept (not replaced), so "Select all CPU" after "Select all Memory"
  // doesn't undo the memory picks. A row whose resource isn't configured or
  // individually eligible is left alone, same as its checkbox being disabled.
  function selectAll(applyCPU: boolean, applyMemory: boolean) {
    if (!items) return;
    setSelection((prev) => {
      const next = { ...prev };
      for (const item of items) {
        const wantCPU = applyCPU && item.cpuConfigured && item.cpuEligible;
        const wantMemory =
          applyMemory && item.memoryConfigured && item.memoryEligible;
        if (!wantCPU && !wantMemory) continue;
        const key = rowKey(item);
        const existing = next[key] ?? EMPTY_SELECTION;
        next[key] = {
          cpu: existing.cpu || wantCPU,
          memory: existing.memory || wantMemory,
        };
      }
      return next;
    });
  }

  function removeFromSelection(
    namespace: string,
    vpaName: string,
    containerName: string,
  ) {
    const key = rowKey({ namespace, vpaName, containerName });
    setSelection((prev) => {
      const next = { ...prev };
      delete next[key];
      return next;
    });
  }

  function clearSelection() {
    setSelection({});
    setSummaryError(null);
  }

  // Queues every selected (row, resource) pair for write-back, one API call
  // per row (the bulk endpoint can't express "CPU here, memory there, both
  // over here"). Applies sequentially so a row that fails (e.g. it stopped
  // being eligible since the page loaded) is reported and left selected,
  // while everything that already succeeded is removed from the selection
  // rather than being silently retried or lost.
  async function applySelection() {
    if (!items) return;
    const entries = Object.entries(selection).filter(
      ([, sel]) => sel.cpu || sel.memory,
    );
    const planned = entries.flatMap(([key, sel]) => {
      const item = items.find((i) => rowKey(i) === key);
      return item ? [{ key, sel, item, limitSpecs: limitSpecsFor(item, sel) }] : [];
    });
    if (planned.length === 0 || planned.some((p) => p.limitSpecs === null))
      return;

    setSummaryBusy(true);
    const failures: string[] = [];
    for (const { key, sel, item, limitSpecs } of planned) {
      try {
        await selectRecommendation(
          item.namespace,
          item.vpaName,
          item.containerName,
          { applyCPU: sel.cpu, applyMemory: sel.memory, ...limitSpecs },
        );
        setSelection((prev) => {
          const next = { ...prev };
          delete next[key];
          return next;
        });
      } catch (err) {
        failures.push(
          `${item.namespace}/${item.vpaName}/${item.containerName}: ${String(err)}`,
        );
      }
    }
    setSummaryError(failures.length > 0 ? failures.join("\n") : null);
    setSummaryBusy(false);
    await load();
  }

  const selectedRows: SelectedRow[] = useMemo(() => {
    if (!items) return [];
    return Object.entries(selection)
      .filter(([, sel]) => sel.cpu || sel.memory)
      .map(([key, sel]) => ({
        item: items.find((i) => rowKey(i) === key),
        ...sel,
      }))
      .filter((row): row is SelectedRow => row.item !== undefined);
  }, [items, selection]);

  // Guards against a blank page: if the summary drains to empty while
  // expanded (Clear, or Apply succeeding on everything selected), fall back
  // to the table/cards view instead of rendering nothing.
  useEffect(() => {
    if (summaryExpanded && selectedRows.length === 0) {
      setSummaryExpanded(false);
    }
  }, [summaryExpanded, selectedRows.length]);

  if (error) {
    return (
      <div className="panel panel-error">
        Failed to load recommendations: {error}
      </div>
    );
  }
  if (items === null) {
    return <div className="panel panel-loading">Loading…</div>;
  }
  if (items.length === 0) {
    return (
      <div className="panel">
        <p>No VPA recommendations found.</p>
        <p className="muted">
          Create a <code>VpaGitOpsBinding</code> in the same namespace as a
          VerticalPodAutoscaler to configure it for GitOps write-back and have
          it show up here.
        </p>
      </div>
    );
  }

  const anyCPUEligible = items.some((i) => i.cpuEligible);
  const anyMemoryEligible = items.some((i) => i.memoryEligible);

  if (summaryExpanded) {
    return (
      <div>
        <SelectionSummary
          variant="full"
          rows={selectedRows}
          busy={summaryBusy}
          error={summaryError}
          limits={limits}
          headroom={headroom}
          onRemove={removeFromSelection}
          onClear={clearSelection}
          onApply={applySelection}
          onCollapse={() => setSummaryExpanded(false)}
        />
      </div>
    );
  }

  return (
    <div>
      <div className="toolbar">
        <div className="toolbar-actions">
          <button
            className="btn"
            disabled={!anyCPUEligible && !anyMemoryEligible}
            onClick={() => selectAll(true, true)}
          >
            Select all CPU + Memory
          </button>
          <button
            className="btn"
            disabled={!anyMemoryEligible}
            onClick={() => selectAll(false, true)}
          >
            Select all Memory
          </button>
          <button
            className="btn"
            disabled={!anyCPUEligible}
            onClick={() => selectAll(true, false)}
          >
            Select all CPU
          </button>
        </div>

        <div className="view-toggle" role="group" aria-label="View mode">
          <button
            className={`view-toggle-btn${view === "table" ? " active" : ""}`}
            aria-pressed={view === "table"}
            onClick={() => setView("table")}
          >
            Table
          </button>
          <button
            className={`view-toggle-btn${view === "cards" ? " active" : ""}`}
            aria-pressed={view === "cards"}
            onClick={() => setView("cards")}
          >
            Cards
          </button>
        </div>
      </div>

      {view === "cards" ? (
        <div className="recommendation-cards">
          {sortedItems.map((item) => {
            const key = rowKey(item);
            const sel = selection[key] ?? EMPTY_SELECTION;
            const busy = rowBusy[key] ?? false;

            return (
              <RecommendationCard
                key={key}
                item={item}
                cpuChecked={sel.cpu}
                memoryChecked={sel.memory}
                busy={busy}
                error={rowError[key]}
                onToggle={(resource, checked) => toggle(key, resource, checked)}
                onAccept={() => acceptRow(item)}
              />
            );
          })}
        </div>
      ) : (
        <div className="panel panel-table">
          <table className="recommendation-table">
            <thead>
              <tr>
                {SORT_COLUMNS.map((col) => (
                  <th key={col.key}>
                    <button
                      className="th-sort"
                      onClick={() => toggleSort(col.key)}
                    >
                      {col.label}
                      {sort.key === col.key && (
                        <span className="th-sort-arrow">
                          {sort.direction === "asc" ? "▲" : "▼"}
                        </span>
                      )}
                    </button>
                  </th>
                ))}
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {sortedItems.map((item) => {
                const key = rowKey(item);
                const sel = selection[key] ?? EMPTY_SELECTION;
                const busy = rowBusy[key] ?? false;
                const canAccept = (sel.cpu || sel.memory) && !busy;

                return (
                  <tr key={key}>
                    <td>{item.namespace}</td>
                    <td>
                      <Link
                        to={`/recommendations/${item.namespace}/${item.vpaName}/${item.containerName}`}
                      >
                        {item.vpaName}
                      </Link>
                    </td>
                    <td className="cell-muted">
                      {item.workload.kind}/{item.workload.name}
                    </td>
                    <td className="cell-muted">{item.updateMode}</td>
                    <td>
                      <div className="resource-cell">
                        <ResourceCheckbox
                          label={`Select CPU for ${item.containerName}`}
                          configured={item.cpuConfigured}
                          eligible={item.cpuEligible}
                          reasons={item.cpuEligibilityReasons}
                          checked={sel.cpu}
                          onChange={(checked) => toggle(key, "cpu", checked)}
                        />
                        <DeltaBadge
                          kind="cpu"
                          current={item.currentCpu}
                          recommended={item.targetCpu ?? item.recommendedCpu} vpa={item.recommendedCpu} headroomPercent={item.cpuRequestHeadroomPercent}
                          percent={item.deltaCpuPercent}
                        />
                      </div>
                    </td>
                    <td>
                      <div className="resource-cell">
                        <ResourceCheckbox
                          label={`Select memory for ${item.containerName}`}
                          configured={item.memoryConfigured}
                          eligible={item.memoryEligible}
                          reasons={item.memoryEligibilityReasons}
                          checked={sel.memory}
                          onChange={(checked) => toggle(key, "memory", checked)}
                        />
                        <DeltaBadge
                          kind="memory"
                          current={item.currentMemory}
                          recommended={item.targetMemory ?? item.recommendedMemory} vpa={item.recommendedMemory} headroomPercent={item.memoryRequestHeadroomPercent}
                          percent={item.deltaMemoryPercent}
                        />
                      </div>
                    </td>
                    <td>
                      {item.currentValueError ? (
                        <span
                          className="badge badge-ineligible"
                          title={item.currentValueError}
                        >
                          Error
                        </span>
                      ) : (
                        <EligibilityBadge
                          eligible={item.eligible}
                          reasons={item.eligibilityReasons}
                        />
                      )}
                    </td>
                    <td>
                      <StatusBadge status={item.status} operation={item.operation} />
                    </td>
                    <td>
                      <button
                        className="btn btn-primary btn-small"
                        disabled={!canAccept}
                        onClick={() => acceptRow(item)}
                      >
                        {busy ? "…" : "Accept"}
                      </button>
                      {rowError[key] && (
                        <div className="cell-error" title={rowError[key]}>
                          Failed
                        </div>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      <SelectionSummary
        variant="floating"
        rows={selectedRows}
        busy={summaryBusy}
        error={summaryError}
        limits={limits}
        headroom={headroom}
        onRemove={removeFromSelection}
        onClear={clearSelection}
        onApply={applySelection}
        onExpand={() => setSummaryExpanded(true)}
      />
    </div>
  );
}
