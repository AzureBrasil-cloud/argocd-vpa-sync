import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { Search } from "lucide-react";
import { selectRecommendation } from "../api/client";
import type { RecommendationDTO, RecommendationStatus, SelectRequest } from "../api/types";
import { EligibilityBadge } from "../components/EligibilityBadge";
import { Icon } from "../components/Icon";
import { PageTitle } from "../components/PageTitle";
import { ResourceCheckbox } from "../components/ResourceCheckbox";
import { ResourceReading } from "../components/ResourceReading";
import { Segmented } from "../components/Segmented";
import { SelectionSummary, type SelectedRow } from "../components/SelectionSummary";
import { StatusBadge } from "../components/StatusBadge";
import { ageSince, useNow } from "../lib/time";
import { planSelection } from "../lib/selectionPlan";
import { useLimitSettings } from "../lib/useLimitSettings";
import { recommendationPath, rowKey, useRecommendations } from "../lib/useRecommendations";
import { useRequestHeadroom } from "../lib/useRequestHeadroom";
import { useTheme } from "../theme";

type RowKey = string;

interface RowSelection {
  cpu: boolean;
  memory: boolean;
}

const EMPTY_SELECTION: RowSelection = { cpu: false, memory: false };

type SortKey = "vpa" | "cpuDelta" | "memoryDelta" | "eligibility" | "status";
type SortDirection = "asc" | "desc";

interface SortState {
  key: SortKey;
  direction: SortDirection;
}

type EligibilityFilter = "all" | "eligible" | "not-eligible";

function plural(count: number, noun: string): string {
  return count === 1 ? noun : `${noun}s`;
}

const STATUSES: RecommendationStatus[] = ["new", "selected", "applying", "applied", "skipped", "failed"];

function sortValue(item: RecommendationDTO, key: SortKey): string | number | null {
  switch (key) {
    case "vpa":
      return `${item.namespace}/${item.vpaName}/${item.containerName}`;
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
function compareBySort(a: RecommendationDTO, b: RecommendationDTO, sort: SortState): number {
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
    const raw = typeof av === "number" && typeof bv === "number" ? av - bv : String(av).localeCompare(String(bv));
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

function matchesQuery(item: RecommendationDTO, query: string): boolean {
  if (!query) return true;
  const haystack = `${item.namespace} ${item.vpaName} ${item.containerName} ${item.workload.kind}/${item.workload.name}`;
  return haystack.toLowerCase().includes(query.toLowerCase());
}

/** Which resources of an eligible row are eligible, for the line under its verdict. */
function eligibleDetail(item: RecommendationDTO): string | undefined {
  const parts = [item.cpuConfigured && item.cpuEligible && "cpu", item.memoryConfigured && item.memoryEligible && "memory"];
  const list = parts.filter(Boolean).join(" · ");
  return list || undefined;
}

/**
 * The first and last namespace among the table rows currently on screen --
 * the "guide words" of the list, so a long scroll always says where it is.
 */
function useVisibleRange(rowCount: number) {
  const bodyRef = useRef<HTMLTableSectionElement>(null);
  const [range, setRange] = useState<{ first: string; last: string } | null>(null);

  useEffect(() => {
    let frame = 0;
    function measure() {
      frame = 0;
      const rows = bodyRef.current?.querySelectorAll<HTMLTableRowElement>("tr[data-namespace]");
      if (!rows || rows.length === 0) {
        setRange(null);
        return;
      }
      let first: string | null = null;
      let last: string | null = null;
      for (const row of rows) {
        const rect = row.getBoundingClientRect();
        if (rect.bottom > 0 && rect.top < window.innerHeight) {
          first ??= row.dataset.namespace ?? null;
          last = row.dataset.namespace ?? null;
        }
      }
      setRange(first && last ? { first, last } : null);
    }
    function schedule() {
      if (!frame) frame = requestAnimationFrame(measure);
    }
    measure();
    window.addEventListener("scroll", schedule, { passive: true });
    window.addEventListener("resize", schedule);
    return () => {
      window.removeEventListener("scroll", schedule);
      window.removeEventListener("resize", schedule);
      if (frame) cancelAnimationFrame(frame);
    };
  }, [rowCount]);

  return { bodyRef, range };
}

export function RecommendationList() {
  const theme = useTheme();
  const now = useNow();
  const { items, error, reload } = useRecommendations();

  const [selection, setSelection] = useState<Record<RowKey, RowSelection>>({});
  const [summaryBusy, setSummaryBusy] = useState(false);
  const [summaryError, setSummaryError] = useState<string | null>(null);
  const limits = useLimitSettings();
  const headroom = useRequestHeadroom();

  const [sort, setSort] = useState<SortState>({ key: "vpa", direction: "asc" });
  const [query, setQuery] = useState("");
  const [eligibility, setEligibility] = useState<EligibilityFilter>("all");
  const [status, setStatus] = useState<RecommendationStatus | "any">("any");

  function toggleSort(key: SortKey) {
    setSort((prev) =>
      prev.key === key ? { key, direction: prev.direction === "asc" ? "desc" : "asc" } : { key, direction: "asc" },
    );
  }

  // A stable number per container (by namespace/VPA/container), so the
  // calibration theme's row numbers don't change when the table is re-sorted.
  const serials = useMemo(() => {
    const keys = (items ?? []).map(rowKey).sort();
    return new Map(keys.map((key, i) => [key, String(i + 1).padStart(3, "0")]));
  }, [items]);

  const searched = useMemo(() => (items ?? []).filter((item) => matchesQuery(item, query)), [items, query]);
  const visibleItems = useMemo(
    () =>
      searched
        .filter((item) => eligibility === "all" || item.eligible === (eligibility === "eligible"))
        .filter((item) => status === "any" || item.status === status)
        .sort((a, b) => compareBySort(a, b, sort)),
    [searched, eligibility, status, sort],
  );

  const { bodyRef, range } = useVisibleRange(visibleItems.length);

  // The request headroom and limit specs to send for one row's selection
  // (see planSelection). null while a setting that is to be sent is invalid.
  function limitSpecsFor(item: RecommendationDTO, sel: RowSelection): Omit<SelectRequest, "applyCPU" | "applyMemory"> | null {
    const plan = planSelection(item, sel, limits, headroom);
    return plan.invalid ? null : plan.body;
  }

  function setRow(key: RowKey, sel: RowSelection) {
    setSelection((prev) => {
      const next = { ...prev };
      if (sel.cpu || sel.memory) next[key] = sel;
      else delete next[key];
      return next;
    });
  }

  function toggle(key: RowKey, resource: "cpu" | "memory", checked: boolean) {
    setRow(key, { ...(selection[key] ?? EMPTY_SELECTION), [resource]: checked });
  }

  const canCPU = (item: RecommendationDTO) => item.cpuConfigured && item.cpuEligible;
  const canMemory = (item: RecommendationDTO) => item.memoryConfigured && item.memoryEligible;

  // Ticks the CPU/memory boxes for every eligible row on screen, same as
  // ticking each one by hand -- it never calls the API itself. Existing
  // selections are kept (not replaced), so "CPU" after "Memory" doesn't undo
  // the memory picks. A resource that isn't configured or individually
  // eligible is left alone, same as its checkbox being disabled.
  function selectEligible(applyCPU: boolean, applyMemory: boolean) {
    setSelection((prev) => {
      const next = { ...prev };
      for (const item of visibleItems) {
        const wantCPU = applyCPU && canCPU(item);
        const wantMemory = applyMemory && canMemory(item);
        if (!wantCPU && !wantMemory) continue;
        const key = rowKey(item);
        const existing = next[key] ?? EMPTY_SELECTION;
        next[key] = { cpu: existing.cpu || wantCPU, memory: existing.memory || wantMemory };
      }
      return next;
    });
  }

  function removeFromSelection(namespace: string, vpaName: string, containerName: string) {
    setRow(rowKey({ namespace, vpaName, containerName }), EMPTY_SELECTION);
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
    const entries = Object.entries(selection).filter(([, sel]) => sel.cpu || sel.memory);
    const planned = entries.flatMap(([key, sel]) => {
      const item = items.find((i) => rowKey(i) === key);
      return item ? [{ key, sel, item, limitSpecs: limitSpecsFor(item, sel) }] : [];
    });
    if (planned.length === 0 || planned.some((p) => p.limitSpecs === null)) return;

    setSummaryBusy(true);
    const failures: string[] = [];
    for (const { key, sel, item, limitSpecs } of planned) {
      try {
        await selectRecommendation(item.namespace, item.vpaName, item.containerName, {
          applyCPU: sel.cpu,
          applyMemory: sel.memory,
          ...limitSpecs,
        });
        setRow(key, EMPTY_SELECTION);
      } catch (err) {
        failures.push(`${item.namespace}/${item.vpaName}/${item.containerName}: ${String(err)}`);
      }
    }
    setSummaryError(failures.length > 0 ? failures.join("\n") : null);
    setSummaryBusy(false);
    await reload().catch(() => undefined);
  }

  const selectedRows: SelectedRow[] = useMemo(() => {
    if (!items) return [];
    return Object.entries(selection)
      .filter(([, sel]) => sel.cpu || sel.memory)
      .map(([key, sel]) => ({ item: items.find((i) => rowKey(i) === key), ...sel }))
      .filter((row): row is SelectedRow => row.item !== undefined);
  }, [items, selection]);

  if (error) {
    return (
      <div className="page">
        <header className="page-head">
          <PageTitle section="recommendations" />
        </header>
        <div className="callout callout-error">
          <Icon name="error" />
          <span>Failed to load recommendations: {error}</span>
        </div>
      </div>
    );
  }
  if (items === null) {
    return (
      <div className="page">
        <header className="page-head">
          <PageTitle section="recommendations" />
        </header>
        <p className="loading">Loading recommendations…</p>
      </div>
    );
  }
  if (items.length === 0) {
    return (
      <div className="page">
        <header className="page-head">
          <PageTitle section="recommendations" />
        </header>
        <div className="empty">
          <h2>No VPA recommendations yet</h2>
          <p>
            Create a <code>VpaGitOpsBinding</code> in the same namespace as a VerticalPodAutoscaler (with{" "}
            <code>updateMode: "Off"</code>) to opt it in to GitOps write-back. Its containers show up here once the VPA
            has a recommendation.
          </p>
        </div>
      </div>
    );
  }

  const eligibleCount = items.filter((i) => i.eligible).length;
  const failedCount = items.filter((i) => i.status === "failed").length;
  const vpaCount = new Set(items.map((i) => `${i.namespace}/${i.vpaName}`)).size;
  const lastWriteBack = items
    .filter((i) => i.status === "applied" && i.operation?.commitSha)
    .sort((a, b) => (b.operation!.updatedAt ?? "").localeCompare(a.operation!.updatedAt ?? ""))[0]?.operation;
  const clusters = [...new Set(items.map((i) => i.cluster).filter(Boolean))];
  const statusCounts = new Map(STATUSES.map((s) => [s, searched.filter((i) => i.status === s).length]));

  const anyCPU = visibleItems.some(canCPU);
  const anyMemory = visibleItems.some(canMemory);
  const selectableVisible = visibleItems.filter((i) => canCPU(i) || canMemory(i));
  const allVisibleSelected =
    selectableVisible.length > 0 &&
    selectableVisible.every((i) => {
      const sel = selection[rowKey(i)];
      return sel && (!canCPU(i) || sel.cpu) && (!canMemory(i) || sel.memory);
    });

  // Bulk actions on what is on screen. The default theme puts them on the
  // guide line, which names exactly that set; the navbar themes keep them in
  // the filter row, where the guide line carries the gauge legend.
  const selectEligibleGroup = (
    <div className="select-eligible" role="group" aria-label="Select eligible changes on screen">
      <span className="select-eligible-label">Select eligible</span>
      <button className="btn btn-small" disabled={!anyCPU && !anyMemory} onClick={() => selectEligible(true, true)}>
        CPU + memory
      </button>
      <button className="btn btn-small" disabled={!anyMemory} onClick={() => selectEligible(false, true)}>
        Memory
      </button>
      <button className="btn btn-small" disabled={!anyCPU} onClick={() => selectEligible(true, false)}>
        CPU
      </button>
    </div>
  );

  function toggleAllVisible(checked: boolean) {
    if (checked) selectEligible(true, true);
    else for (const item of selectableVisible) setRow(rowKey(item), EMPTY_SELECTION);
  }

  function sortHeader(key: SortKey, label: string, className?: string) {
    const active = sort.key === key;
    return (
      <th className={className} aria-sort={active ? (sort.direction === "asc" ? "ascending" : "descending") : undefined}>
        <button className="th-sort" onClick={() => toggleSort(key)}>
          {label}
          {active && <Icon name={sort.direction === "asc" ? "sort-asc" : "sort-desc"} />}
        </button>
      </th>
    );
  }

  return (
    <div className="page page-with-queue">
      {theme === "calibration" ? (
        <section className="id-strip" aria-label="Summary">
          <div>
            <span className="id-label">Cluster</span>
            <span className="id-value">{clusters.join(", ") || "in-cluster"}</span>
          </div>
          <div>
            <span className="id-label">Bindings</span>
            <span className="id-value">
              {vpaCount} <small>VPAs · {items.length} containers</small>
            </span>
          </div>
          <div>
            <span className="id-label">Eligible</span>
            <span className="id-value">
              {eligibleCount} <small>outside VPA range</small>
            </span>
          </div>
          <div>
            <span className="id-label">Failed write-backs</span>
            <span className={`id-value${failedCount > 0 ? " id-value-bad" : ""}`}>{failedCount}</span>
          </div>
          <div>
            <span className="id-label">Last commit</span>
            <span className="id-value">
              {lastWriteBack?.commitSha ? (
                <>
                  <code className="sha">{lastWriteBack.commitSha.slice(0, 7)}</code>{" "}
                  <small>{ageSince(lastWriteBack.updatedAt, now)}</small>
                </>
              ) : (
                <small>none yet</small>
              )}
            </span>
          </div>
        </section>
      ) : (
        <header className="page-head">
          <PageTitle section="recommendations" />
          <p className="page-sub">
            <strong>{items.length}</strong> {plural(items.length, "container")} in <strong>{vpaCount}</strong>{" "}
            {plural(vpaCount, "VPA")} · <strong>{eligibleCount}</strong> eligible
            {failedCount > 0 && (
              <>
                {" "}
                · <strong className="text-bad">{failedCount}</strong> failed write-back{failedCount === 1 ? "" : "s"}
              </>
            )}
            {lastWriteBack?.commitSha && (
              <>
                {" "}
                · last commit <code className="sha">{lastWriteBack.commitSha.slice(0, 7)}</code>{" "}
                {ageSince(lastWriteBack.updatedAt, now)}
              </>
            )}
          </p>
        </header>
      )}

      <div className="work">
        <section className="work-main" aria-label="Recommendations">
          <div className="filters">
            <label className="search">
              <span className="sr-only">Filter by namespace, VPA, workload or container</span>
              {theme === "default" && <Search className="search-icon" size={16} strokeWidth={1.5} aria-hidden="true" />}
              <input
                type="search"
                placeholder="Filter namespace, VPA, workload…"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
              />
            </label>
            <Segmented
              label="Eligibility"
              value={eligibility}
              onChange={setEligibility}
              options={[
                { value: "all", label: "All", count: searched.length },
                { value: "eligible", label: "Eligible", count: searched.filter((i) => i.eligible).length },
                { value: "not-eligible", label: "Not eligible", count: searched.filter((i) => !i.eligible).length },
              ]}
            />
            <label className="select">
              <span>Status</span>
              <select value={status} onChange={(e) => setStatus(e.target.value as RecommendationStatus | "any")}>
                <option value="any">Any</option>
                {STATUSES.map((s) => (
                  <option key={s} value={s}>
                    {s} ({statusCounts.get(s)})
                  </option>
                ))}
              </select>
            </label>
            {theme !== "default" && selectEligibleGroup}
          </div>

          <div className="guide" aria-live="polite">
            <span className="guide-range">
              {range ? (range.first === range.last ? range.first : `${range.first} — ${range.last}`) : "No matches"}
            </span>
            <span className="guide-count">
              {visibleItems.length} of {items.length} {plural(items.length, "container")}
            </span>
            {theme === "default" && selectEligibleGroup}
            {theme !== "default" && (
              <span className="guide-legend" aria-hidden="true">
                <span className="legend-item">
                  <span className="legend-band" /> VPA range
                </span>
                <span className="legend-item">
                  <span className="legend-current" /> current request
                </span>
                <span className="legend-item">
                  <span className="legend-move" /> recommendation
                </span>
                <span className="legend-item legend-item-hatched">
                  <span className="legend-move legend-hatched" /> not eligible
                </span>
              </span>
            )}
          </div>

          <div className="table-wrap">
            <table className="rec-table">
              <thead>
                <tr>
                  <th className="col-check">
                    <input
                      type="checkbox"
                      className="resource-checkbox"
                      aria-label="Select every eligible change on screen"
                      checked={allVisibleSelected}
                      disabled={selectableVisible.length === 0}
                      onChange={(e) => toggleAllVisible(e.target.checked)}
                    />
                  </th>
                  {theme === "calibration" && <th className="col-serial">No.</th>}
                  {sortHeader("vpa", "VPA · namespace", "col-vpa")}
                  {sortHeader("cpuDelta", "CPU request → rec.", "col-resource")}
                  {sortHeader("memoryDelta", "Memory request → rec.", "col-resource")}
                  {sortHeader("eligibility", "Eligibility", "col-eligibility")}
                  {sortHeader("status", "Write-back", "col-status")}
                </tr>
              </thead>
              <tbody ref={bodyRef}>
                {visibleItems.length === 0 && (
                  <tr>
                    <td colSpan={theme === "calibration" ? 7 : 6} className="no-matches">
                      No containers match these filters.{" "}
                      <button
                        className="btn-link"
                        onClick={() => {
                          setQuery("");
                          setEligibility("all");
                          setStatus("any");
                        }}
                      >
                        Clear filters
                      </button>
                    </td>
                  </tr>
                )}
                {visibleItems.map((item) => {
                  const key = rowKey(item);
                  const sel = selection[key] ?? EMPTY_SELECTION;
                  const selectable = canCPU(item) || canMemory(item);
                  const rowChecked = selectable && (!canCPU(item) || sel.cpu) && (!canMemory(item) || sel.memory);
                  const selected = sel.cpu || sel.memory;
                  return (
                    <tr key={key} data-namespace={item.namespace} className={selected ? "is-selected" : undefined}>
                      <td className="col-check">
                        <input
                          type="checkbox"
                          className="resource-checkbox"
                          aria-label={`Select every eligible change for ${item.vpaName} · ${item.containerName}`}
                          checked={rowChecked}
                          disabled={!selectable}
                          onChange={(e) =>
                            setRow(key, e.target.checked ? { cpu: canCPU(item), memory: canMemory(item) } : EMPTY_SELECTION)
                          }
                        />
                      </td>
                      {theme === "calibration" && <td className="col-serial">{serials.get(key)}</td>}
                      <td className="col-vpa">
                        <Link className="vpa-link" to={recommendationPath(item)} title={`${item.namespace}/${item.vpaName}`}>
                          {item.vpaName}
                        </Link>
                        <span className="vpa-meta">
                          {item.namespace} · {item.workload.kind} · {item.containerName}
                        </span>
                      </td>
                      {(["cpu", "memory"] as const).map((resource) => (
                        <td className="col-resource" key={resource}>
                          <div className="resource-cell">
                            <ResourceCheckbox
                              label={`Select ${resource === "cpu" ? "CPU" : "memory"} for ${item.vpaName} · ${item.containerName}`}
                              configured={resource === "cpu" ? item.cpuConfigured : item.memoryConfigured}
                              eligible={resource === "cpu" ? item.cpuEligible : item.memoryEligible}
                              reasons={resource === "cpu" ? item.cpuEligibilityReasons : item.memoryEligibilityReasons}
                              checked={sel[resource]}
                              onChange={(checked) => toggle(key, resource, checked)}
                            />
                            <ResourceReading item={item} resource={resource} />
                          </div>
                        </td>
                      ))}
                      <td className="col-eligibility">
                        <EligibilityBadge
                          eligible={item.eligible}
                          reasons={item.validationErrors?.length ? ["invalid binding"] : item.eligibilityReasons}
                          detail={eligibleDetail(item)}
                          error={item.currentValueError}
                        />
                      </td>
                      <td className="col-status">
                        <StatusBadge status={item.status} operation={item.operation} />
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </section>

        <SelectionSummary
          rows={selectedRows}
          busy={summaryBusy}
          error={summaryError}
          limits={limits}
          headroom={headroom}
          onRemove={removeFromSelection}
          onClear={clearSelection}
          onApply={applySelection}
        />
      </div>
    </div>
  );
}
