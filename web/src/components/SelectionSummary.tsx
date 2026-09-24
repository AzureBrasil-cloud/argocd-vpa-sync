import type { RecommendationDTO } from '../api/types'
import { formatCPUMilli, formatMemory, formatMemoryBytes, parseCPUMilli, parseK8sQuantityBytes } from '../lib/format'
import { decideLimit, limitChange, type LimitChange, type LimitResource } from '../lib/limits'
import type { LimitSettings } from '../lib/useLimitSettings'
import { LimitSettingsInput, type LimitResourceCounts } from './LimitSettingsInput'

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
  onRemove: (namespace: string, vpaName: string, containerName: string) => void
  onClear: () => void
  onApply: () => void
  variant?: 'floating' | 'full'
  onExpand?: () => void
  onCollapse?: () => void
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
  'write-back cannot raise the limit. Kubernetes rejects a request greater than its limit -- set ' +
  'cpuLimitKeyPath/memoryLimitKeyPath on the binding, or raise the limit in Git by hand.'

export function LimitBadge({ change }: { change: LimitChange | null }) {
  if (!change) return null
  if (change.unmanagedExceeded) {
    return (
      <span className="badge badge-increase" title={UNMANAGED_EXCEEDED_TOOLTIP}>
        limit {change.current}: {change.reason} ⚠
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
        limit {change.next} &lt; request ✕
      </span>
    )
  }
  return (
    <span
      className={change.decreases ? 'badge badge-increase' : 'badge badge-muted'}
      title={change.decreases ? 'The limit will be lowered -- check the workload can live with it' : undefined}
    >
      limit {change.current} → {change.next}
      {change.decreases && ' ⚠'}
    </span>
  )
}

/**
 * A staging area for the current cpu/memory checkbox selection across the
 * table: totals current usage vs. the recommended values about to be
 * queued, previews the limit each one will get from the limit settings
 * entered here, lists exactly which containers are included, and is the
 * only place that actually calls the select API -- the table's checkboxes
 * and the toolbar's "Select all" buttons only ever change local state (see
 * RecommendationList), so nothing is queued for write-back until the user
 * reviews this card and clicks Apply.
 */
export function SelectionSummary({
  rows,
  busy,
  error,
  limits,
  onRemove,
  onClear,
  onApply,
  variant = 'floating',
  onExpand,
  onCollapse,
}: SelectionSummaryProps) {
  if (rows.length === 0) return null

  const decisions = rows.map(({ item, cpu, memory }) => ({
    cpu: cpu ? decideLimit(item, 'cpu', limits) : null,
    memory: memory ? decideLimit(item, 'memory', limits) : null,
  }))
  const resources: LimitResourceCounts[] = (['cpu', 'memory'] as LimitResource[]).map((resource) => ({
    resource,
    required: decisions.filter((d) => d[resource]?.requirement === 'required').length,
    optional: decisions.filter((d) => d[resource]?.requirement === 'optional').length,
  }))
  const invalidSettings = decisions.some((d) => d.cpu?.invalid || d.memory?.invalid)

  let cpuTotals: Totals = { count: 0, current: 0, recommended: 0 }
  let memoryTotals: Totals = { count: 0, current: 0, recommended: 0 }
  let cpuLimitTotals: Totals = { count: 0, current: 0, recommended: 0 }
  let memoryLimitTotals: Totals = { count: 0, current: 0, recommended: 0 }
  let decreasingLimits = 0
  let belowRequest = 0
  let unmanagedExceeded = 0

  const limitChanges = rows.map(({ item }, i) => {
    const { cpu, memory } = decisions[i]
    const cpuLimit = cpu && !cpu.invalid ? limitChange(item, 'cpu', cpu.parsed) : null
    const memoryLimit = memory && !memory.invalid ? limitChange(item, 'memory', memory.parsed) : null
    for (const c of [cpuLimit, memoryLimit]) {
      if (c?.belowRequest) belowRequest++
      else if (c?.unmanagedExceeded) unmanagedExceeded++
      else if (c?.decreases) decreasingLimits++
    }
    return { cpuLimit, memoryLimit }
  })

  rows.forEach((row, i) => {
    if (row.cpu) {
      cpuTotals = addTotals(cpuTotals, parseCPUMilli(row.item.currentCpu), parseCPUMilli(row.item.recommendedCpu))
      cpuLimitTotals = addLimitTotals(cpuLimitTotals, limitChanges[i].cpuLimit)
    }
    if (row.memory) {
      memoryTotals = addTotals(
        memoryTotals,
        parseK8sQuantityBytes(row.item.currentMemory ?? ''),
        parseK8sQuantityBytes(row.item.recommendedMemory ?? ''),
      )
      memoryLimitTotals = addLimitTotals(memoryLimitTotals, limitChanges[i].memoryLimit)
    }
  })

  return (
    <div className={variant === 'full' ? 'selection-summary selection-summary-full' : 'selection-summary'}>
      <div className="selection-summary-header">
        <div>
          <strong>{rows.length}</strong> container{rows.length === 1 ? '' : 's'} selected
          {cpuTotals.count > 0 && (
            <span className="selection-summary-total">
              CPU: {formatCPUMilli(cpuTotals.current)} → {formatCPUMilli(cpuTotals.recommended)}
              {cpuLimitTotals.count > 0 &&
                ` · limit ${formatCPUMilli(cpuLimitTotals.current)} → ${formatCPUMilli(cpuLimitTotals.recommended)}`}
            </span>
          )}
          {memoryTotals.count > 0 && (
            <span className="selection-summary-total">
              Memory: {formatMemoryBytes(memoryTotals.current)} → {formatMemoryBytes(memoryTotals.recommended)}
              {memoryLimitTotals.count > 0 &&
                ` · limit ${formatMemoryBytes(memoryLimitTotals.current)} → ${formatMemoryBytes(memoryLimitTotals.recommended)}`}
            </span>
          )}
        </div>
        <LimitSettingsInput settings={limits} resources={resources} disabled={busy} />
        {belowRequest > 0 && (
          <div className="selection-summary-blocker">
            ✕ {belowRequest} limit{belowRequest === 1 ? ' is' : 's are'} below the new request -- raise the value or
            remove {belowRequest === 1 ? 'that container' : 'those containers'}.
          </div>
        )}
        {unmanagedExceeded > 0 && (
          <div className="selection-summary-warning" title={UNMANAGED_EXCEEDED_TOOLTIP}>
            ⚠ {unmanagedExceeded} new request{unmanagedExceeded === 1 ? ' exceeds its' : 's exceed their'} current
            limit, but the binding sets no limit key path -- write-back can't raise the limit and Kubernetes will
            reject the change. Set cpuLimitKeyPath/memoryLimitKeyPath on the VpaGitOpsBinding.
          </div>
        )}
        {decreasingLimits > 0 && (
          <div className="selection-summary-warning">
            ⚠ {decreasingLimits} limit{decreasingLimits === 1 ? '' : 's'} will be lowered.
          </div>
        )}
        <div className="selection-summary-actions">
          {variant === 'floating' && onExpand && (
            <button className="btn btn-small" onClick={onExpand} aria-label="Expand selection summary">
              Expand
            </button>
          )}
          {variant === 'full' && onCollapse && (
            <button className="btn btn-small" onClick={onCollapse}>
              &larr; Back
            </button>
          )}
          <button className="btn" disabled={busy} onClick={onClear}>
            Clear
          </button>
          <button className="btn btn-primary" disabled={busy || invalidSettings || belowRequest > 0} onClick={onApply}>
            {busy ? 'Applying…' : `Apply ${rows.length}`}
          </button>
        </div>
      </div>

      {error && <div className="panel panel-error selection-summary-error">{error}</div>}

      <ul className="selection-summary-list">
        {rows.map(({ item, cpu, memory }, i) => (
          <li key={`${item.namespace}/${item.vpaName}/${item.containerName}`}>
            <span className="selection-summary-item-name">
              {item.namespace}/{item.vpaName} · {item.containerName}
            </span>
            <span className="selection-summary-item-values">
              {cpu && (
                <span className="selection-summary-resource">
                  <span className="badge badge-muted">
                    cpu {item.currentCpu} → {item.recommendedCpu}
                  </span>
                  <LimitBadge change={limitChanges[i].cpuLimit} />
                </span>
              )}
              {memory && (
                <span className="selection-summary-resource">
                  <span className="badge badge-muted">
                    mem {formatMemory(item.currentMemory)} → {formatMemory(item.recommendedMemory)}
                  </span>
                  <LimitBadge change={limitChanges[i].memoryLimit} />
                </span>
              )}
            </span>
            <button
              className="selection-summary-remove"
              aria-label={`Remove ${item.containerName} from selection`}
              onClick={() => onRemove(item.namespace, item.vpaName, item.containerName)}
            >
              ×
            </button>
          </li>
        ))}
      </ul>
    </div>
  )
}
