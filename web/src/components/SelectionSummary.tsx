import type { RecommendationDTO } from '../api/types'
import { formatCPU, formatCPUMilli, formatMemory, formatMemoryBytes, parseCPUMilli, parseK8sQuantityBytes } from '../lib/format'
import { FORMAT, type LimitChange, type LimitResource } from '../lib/limits'
import { planSelection, type ResourcePlan } from '../lib/selectionPlan'
import type { LimitSettings } from '../lib/useLimitSettings'
import type { RequestHeadroomSettings } from '../lib/useRequestHeadroom'
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
  headroom,
  onRemove,
  onClear,
  onApply,
  variant = 'floating',
  onExpand,
  onCollapse,
}: SelectionSummaryProps) {
  if (rows.length === 0) return null

  const plans = rows.map(({ item, cpu, memory }) => planSelection(item, { cpu, memory }, limits, headroom))
  const resources: LimitResourceCounts[] = (['cpu', 'memory'] as LimitResource[]).map((resource) => ({
    resource,
    required: plans.filter((p) => p[resource]?.limit.requirement === 'required').length,
    optional: plans.filter((p) => p[resource]?.limit.requirement === 'optional').length,
  }))
  const selectedResources = (['cpu', 'memory'] as LimitResource[]).filter((r) => rows.some((row) => row[r]))
  const invalidSettings = plans.some((p) => p.invalid)

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
    <div className={variant === 'full' ? 'selection-summary selection-summary-full' : 'selection-summary'}>
      {variant === 'full' && onCollapse && (
        <button className="btn selection-summary-back" onClick={onCollapse}>
          &larr; Back
        </button>
      )}
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
        <RequestHeadroomInput settings={headroom} resources={selectedResources} disabled={busy} />
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
        {rows.map(({ item }, i) => (
          <li key={`${item.namespace}/${item.vpaName}/${item.containerName}`}>
            <span className="selection-summary-item-name">
              {item.namespace}/{item.vpaName} · {item.containerName}
            </span>
            <span className="selection-summary-item-values">
              {(['cpu', 'memory'] as const).map((resource) => {
                const plan = plans[i][resource]
                return (
                  plan && (
                    <span className="selection-summary-resource" key={resource}>
                      <RequestBadge item={item} resource={resource} plan={plan} />
                      <LimitBadge change={plan.limitChange} />
                    </span>
                  )
                )
              })}
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
