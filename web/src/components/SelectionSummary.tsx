import { useEffect, useRef, useState, type ReactNode } from 'react'
import type { RecommendationDTO } from '../api/types'
import { formatCPU, formatCPUMilli, formatMemory, formatMemoryBytes, parseCPUMilli, parseK8sQuantityBytes } from '../lib/format'
import { FORMAT, type LimitChange, type LimitResource } from '../lib/limits'
import { planSelection, type ResourcePlan } from '../lib/selectionPlan'
import type { LimitSettings } from '../lib/useLimitSettings'
import type { RequestHeadroomSettings } from '../lib/useRequestHeadroom'
import { useTheme } from '../theme'
import { Icon } from './Icon'
import { LimitSettingsInput, type LimitResourceCounts } from './LimitSettingsInput'
import { RequestHeadroomInput } from './RequestHeadroomInput'

export interface SelectedRow {
  item: RecommendationDTO
  cpu: boolean
  memory: boolean
}

interface SelectionSummaryProps {
  rows: SelectedRow[]
  busy: boolean
  error: string | null
  limits: LimitSettings
  headroom: RequestHeadroomSettings
  onRemove: (namespace: string, vpaName: string, containerName: string) => void
  onClear: () => void
  onApply: () => void
}

interface Totals {
  count: number
  current: number
  recommended: number
}

function addTotals(totals: Totals, current: number | null, recommended: number | null): Totals {
  if (current === null || recommended === null) return totals
  return { count: totals.count + 1, current: totals.current + current, recommended: totals.recommended + recommended }
}

function addLimitTotals(totals: Totals, change: LimitChange | null): Totals {
  if (!change) return totals
  return addTotals(totals, change.currentValue, change.nextValue)
}

export const UNMANAGED_EXCEEDED_TOOLTIP =
  'The new request exceeds the current limit, but the VpaGitOpsBinding sets no limit key path, so ' +
  'write-back cannot raise the limit. Kubernetes rejects a request greater than its limit. Set ' +
  'cpuLimitKeyPath/memoryLimitKeyPath on the binding, or raise the limit in Git by hand.'

/**
 * "current → proposed" for one resource's request, noting the VPA value and
 * headroom it came from when a headroom applies.
 */
export function RequestBadge({ item, resource, plan }: { item: RecommendationDTO; resource: LimitResource; plan: ResourcePlan }) {
  const format = resource === 'cpu' ? formatCPU : formatMemory
  const current = resource === 'cpu' ? item.currentCpu : item.currentMemory
  const recommended = resource === 'cpu' ? item.recommendedCpu : item.recommendedMemory
  const next = plan.request === null ? '?' : FORMAT[resource](plan.request)
  return (
    <span className="badge badge-muted">
      {resource === 'cpu' ? 'cpu' : 'mem'} {format(current) || '—'} → {next}
      {plan.headroom ? ` (VPA ${format(recommended)} +${plan.headroom}%)` : ''}
    </span>
  )
}

export function LimitBadge({ change }: { change: LimitChange | null }) {
  if (!change) return null
  if (change.unmanagedExceeded) {
    return (
      <span className="badge badge-warning" title={UNMANAGED_EXCEEDED_TOOLTIP}>
        <Icon name="warning" />
        limit {change.current}: {change.reason}
      </span>
    )
  }
  if (change.next === null) {
    return change.reason ? (
      <span className="badge badge-muted">
        limit{change.current ? ` ${change.current}` : ''}: {change.reason}
      </span>
    ) : null
  }
  if (change.belowRequest) {
    return (
      <span className="badge badge-error" title="Kubernetes rejects a request greater than its limit">
        <Icon name="error" />
        limit {change.next} &lt; request
      </span>
    )
  }
  return (
    <span
      className={change.decreases ? 'badge badge-warning' : 'badge badge-muted'}
      title={change.decreases ? 'The limit will be lowered. Check the workload can live with it.' : undefined}
    >
      {change.decreases && <Icon name="warning" />}
      limit {change.current} → {change.next}
    </span>
  )
}

/**
 * The scrolling list of selected containers, with a "+N more" cue while some
 * of them are below the fold, so a long queue never looks shorter than it is.
 */
function QueueList({ count, children }: { count: number; children: ReactNode }) {
  const ref = useRef<HTMLUListElement>(null)
  const [hidden, setHidden] = useState(0)

  useEffect(() => {
    const list = ref.current
    if (!list) return
    function measure() {
      if (!list) return
      const bottom = list.getBoundingClientRect().bottom
      const below = [...list.children].filter((li) => li.getBoundingClientRect().top >= bottom - 8).length
      setHidden(below)
    }
    measure()
    list.addEventListener('scroll', measure, { passive: true })
    const observer = new ResizeObserver(measure)
    observer.observe(list)
    return () => {
      list.removeEventListener('scroll', measure)
      observer.disconnect()
    }
  }, [count])

  return (
    <div className={`queue-list-wrap${hidden > 0 ? ' has-more' : ''}`}>
      <ul className="queue-list" ref={ref}>
        {children}
      </ul>
      {hidden > 0 && (
        <span className="queue-more" aria-hidden="true">
          +{hidden} more below
        </span>
      )}
    </div>
  )
}

/** A total that flashes when its value changes (remounted by key). */
function QueueValue({ children }: { children: string }) {
  return (
    <span className="queue-value" key={children}>
      {children}
    </span>
  )
}

/**
 * One line of the values diff a selected resource will commit: the key path
 * in the binding's manifest, and its value before and after.
 */
function DiffLine({ keyPath, from, to }: { keyPath?: string; from: string | null; to: string | null }) {
  if (!keyPath || to === null) return null
  return (
    <div className="diff-line">
      <span className="diff-key">{keyPath}</span>
      <span className="diff-from">{from ?? '—'}</span>
      <span className="diff-arrow" aria-hidden="true">→</span>
      <span className="diff-to">{to}</span>
    </div>
  )
}

/**
 * The write-back queue: a staging area for the current CPU/memory
 * selection across the table. It totals current requests vs. the values
 * about to be queued, previews the limit each one will get from the
 * settings entered here, lists exactly which containers are included (the
 * calibration theme as the values diff each one will commit), and is the
 * only place that actually calls the select API -- the table's checkboxes
 * and "Select eligible" buttons only ever change local state (see
 * RecommendationList), so nothing is queued for write-back until the user
 * reviews this panel and clicks Apply.
 */
export function SelectionSummary({ rows, busy, error, limits, headroom, onRemove, onClear, onApply }: SelectionSummaryProps) {
  const theme = useTheme()
  const changeCount = rows.reduce((n, row) => n + (row.cpu ? 1 : 0) + (row.memory ? 1 : 0), 0)
  // The count rolls up when changes are added and down when removed
  // (state adjusted during render, React's pattern for a previous value).
  const [count, setCount] = useState({ value: changeCount, direction: 'up' })
  if (count.value !== changeCount) {
    setCount({ value: changeCount, direction: changeCount > count.value ? 'up' : 'down' })
  }
  const countDirection = count.direction

  if (rows.length === 0) {
    return (
      <aside className="queue queue-empty" aria-label="Write-back queue">
        <h2 className="queue-title">Nothing selected</h2>
        <p className="queue-hint">
          Tick CPU or memory on an eligible row, or use Select eligible. The changes are reviewed here before anything is
          queued for write-back.
        </p>
        <p className="queue-hint">Write-back commits to Git; Argo CD rolls the change out. Nothing in the cluster is changed directly.</p>
      </aside>
    )
  }

  const plans = rows.map(({ item, cpu, memory }) => planSelection(item, { cpu, memory }, limits, headroom))
  const resources: LimitResourceCounts[] = (['cpu', 'memory'] as LimitResource[]).map((resource) => ({
    resource,
    required: plans.filter((p) => p[resource]?.limit.requirement === 'required').length,
    optional: plans.filter((p) => p[resource]?.limit.requirement === 'optional').length,
  }))
  const selectedResources = (['cpu', 'memory'] as LimitResource[]).filter((r) => rows.some((row) => row[r]))
  const invalidSettings = plans.some((p) => p.invalid)
  const branches = [...new Set(rows.map(({ item }) => item.binding?.branch).filter((b): b is string => !!b))]

  let cpuTotals: Totals = { count: 0, current: 0, recommended: 0 }
  let memoryTotals: Totals = { count: 0, current: 0, recommended: 0 }
  let cpuLimitTotals: Totals = { count: 0, current: 0, recommended: 0 }
  let memoryLimitTotals: Totals = { count: 0, current: 0, recommended: 0 }
  let decreasingLimits = 0
  let belowRequest = 0
  let unmanagedExceeded = 0

  plans.forEach(({ cpu, memory }, i) => {
    for (const c of [cpu?.limitChange, memory?.limitChange]) {
      if (c?.belowRequest) belowRequest++
      else if (c?.unmanagedExceeded) unmanagedExceeded++
      else if (c?.decreases) decreasingLimits++
    }
    const item = rows[i].item
    if (cpu) {
      cpuTotals = addTotals(cpuTotals, parseCPUMilli(item.currentCpu), cpu.request)
      cpuLimitTotals = addLimitTotals(cpuLimitTotals, cpu.limitChange)
    }
    if (memory) {
      memoryTotals = addTotals(memoryTotals, parseK8sQuantityBytes(item.currentMemory ?? ''), memory.request)
      memoryLimitTotals = addLimitTotals(memoryLimitTotals, memory.limitChange)
    }
  })

  return (
    <aside className="queue" aria-label="Write-back queue">
      <div className="queue-head">
        <h2 className="queue-title">
          <span className="queue-count" key={changeCount} data-direction={countDirection}>
            {changeCount}
          </span> change{changeCount === 1 ? '' : 's'} selected
        </h2>
        <p className="queue-sub">
          {rows.length} container{rows.length === 1 ? '' : 's'}
          {branches.length > 0 && <> · commits to {branches.join(', ')}</>}
        </p>
      </div>

      <dl className="queue-totals">
        {cpuTotals.count > 0 && (
          <div>
            <dt>CPU requests</dt>
            <dd>
              <QueueValue>{`${formatCPUMilli(cpuTotals.current)} → ${formatCPUMilli(cpuTotals.recommended)}`}</QueueValue>
              {cpuLimitTotals.count > 0 && (
                <span className="queue-totals-limit">
                  limits {formatCPUMilli(cpuLimitTotals.current)} → {formatCPUMilli(cpuLimitTotals.recommended)}
                </span>
              )}
            </dd>
          </div>
        )}
        {memoryTotals.count > 0 && (
          <div>
            <dt>Memory requests</dt>
            <dd>
              <QueueValue>{`${formatMemoryBytes(memoryTotals.current)} → ${formatMemoryBytes(memoryTotals.recommended)}`}</QueueValue>
              {memoryLimitTotals.count > 0 && (
                <span className="queue-totals-limit">
                  limits {formatMemoryBytes(memoryLimitTotals.current)} → {formatMemoryBytes(memoryLimitTotals.recommended)}
                </span>
              )}
            </dd>
          </div>
        )}
      </dl>

      <QueueList count={rows.length}>
        {rows.map(({ item }, i) => (
          <li key={`${item.namespace}/${item.vpaName}/${item.containerName}`} className="queue-item">
            <div className="queue-item-head">
              <span className="queue-item-name">
                {item.namespace}/{item.vpaName}
                <span className="queue-item-container"> · {item.containerName}</span>
                {theme === 'calibration' && item.binding?.manifestPath && (
                  <span className="queue-item-file">{item.binding.manifestPath}</span>
                )}
              </span>
              <button
                className="icon-button"
                aria-label={`Remove ${item.containerName} from selection`}
                disabled={busy}
                onClick={() => onRemove(item.namespace, item.vpaName, item.containerName)}
              >
                <Icon name="close" />
              </button>
            </div>
            {theme === 'calibration' ? (
              <div className="diff">
                {(['cpu', 'memory'] as const).map((resource) => {
                  const plan = plans[i][resource]
                  if (!plan) return null
                  const format = resource === 'cpu' ? formatCPU : formatMemory
                  const current = resource === 'cpu' ? item.currentCpu : item.currentMemory
                  const keyPath = resource === 'cpu' ? item.binding?.cpuKeyPath : item.binding?.memoryKeyPath
                  const limitKeyPath = resource === 'cpu' ? item.binding?.cpuLimitKeyPath : item.binding?.memoryLimitKeyPath
                  return (
                    <div key={resource}>
                      <DiffLine
                        keyPath={keyPath ?? `${resource} request`}
                        from={format(current) || null}
                        to={plan.request === null ? null : FORMAT[resource](plan.request)}
                      />
                      {plan.limitChange?.next && (
                        <DiffLine keyPath={limitKeyPath} from={plan.limitChange.current} to={plan.limitChange.next} />
                      )}
                      {plan.limitChange?.unmanagedExceeded && <LimitBadge change={plan.limitChange} />}
                    </div>
                  )
                })}
              </div>
            ) : (
              <div className="queue-item-values">
                {(['cpu', 'memory'] as const).map((resource) => {
                  const plan = plans[i][resource]
                  return (
                    plan && (
                      <span className="queue-resource" key={resource}>
                        <RequestBadge item={item} resource={resource} plan={plan} />
                        {plan.limitChange?.next !== null || plan.limitChange?.unmanagedExceeded ? (
                          <LimitBadge change={plan.limitChange} />
                        ) : null}
                      </span>
                    )
                  )
                })}
              </div>
            )}
          </li>
        ))}
      </QueueList>

      <div className="queue-settings">
        <RequestHeadroomInput settings={headroom} resources={selectedResources} disabled={busy} compact />
        <LimitSettingsInput settings={limits} resources={resources} disabled={busy} />
      </div>

      {belowRequest > 0 && (
        <div className="callout callout-error">
          <Icon name="error" />
          <span>
            {belowRequest} limit{belowRequest === 1 ? ' is' : 's are'} below the new request. Raise the value or remove{' '}
            {belowRequest === 1 ? 'that container' : 'those containers'}.
          </span>
        </div>
      )}
      {unmanagedExceeded > 0 && (
        <div className="callout callout-warning" title={UNMANAGED_EXCEEDED_TOOLTIP}>
          <Icon name="warning" />
          <span>
            {unmanagedExceeded} new request{unmanagedExceeded === 1 ? ' exceeds its' : 's exceed their'} current limit, but
            the binding sets no limit key path: write-back can't raise the limit and Kubernetes will reject the change. Set
            cpuLimitKeyPath/memoryLimitKeyPath on the VpaGitOpsBinding.
          </span>
        </div>
      )}
      {decreasingLimits > 0 && (
        <div className="callout callout-warning">
          <Icon name="warning" />
          <span>
            {decreasingLimits} limit{decreasingLimits === 1 ? '' : 's'} will be lowered.
          </span>
        </div>
      )}

      {error && (
        <div className="callout callout-error queue-error">
          <Icon name="error" />
          <span>{error}</span>
        </div>
      )}

      <div className="queue-actions">
        <button className="btn btn-primary btn-block" disabled={busy || invalidSettings || belowRequest > 0} onClick={onApply}>
          {busy ? 'Queuing…' : `Apply ${changeCount} change${changeCount === 1 ? '' : 's'}`}
        </button>
        <p className="queue-caption">Write-back commits to Git; Argo CD syncs from there.</p>
        <button className="btn-link" disabled={busy} onClick={onClear}>
          Clear selection
        </button>
      </div>
    </aside>
  )
}
