import type { RecommendationDTO } from '../api/types'
import { formatCPUMilli, formatMemory, formatMemoryBytes, parseCPUMilli, parseK8sQuantityBytes } from '../lib/format'

export interface SelectedRow {
  item: RecommendationDTO
  cpu: boolean
  memory: boolean
}

interface SelectionSummaryProps {
  rows: SelectedRow[]
  busy: boolean
  error: string | null
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

/**
 * A staging area for the current cpu/memory checkbox selection across the
 * table: totals current usage vs. the recommended values about to be
 * queued, lists exactly which containers are included, and is the only
 * place that actually calls the select API -- the table's checkboxes and
 * the toolbar's "Select all" buttons only ever change local state (see
 * RecommendationList), so nothing is queued for write-back until the user
 * reviews this card and clicks Apply.
 */
export function SelectionSummary({
  rows,
  busy,
  error,
  onRemove,
  onClear,
  onApply,
  variant = 'floating',
  onExpand,
  onCollapse,
}: SelectionSummaryProps) {
  if (rows.length === 0) return null

  let cpuTotals: Totals = { count: 0, current: 0, recommended: 0 }
  let memoryTotals: Totals = { count: 0, current: 0, recommended: 0 }
  for (const row of rows) {
    if (row.cpu) {
      cpuTotals = addTotals(cpuTotals, parseCPUMilli(row.item.currentCpu), parseCPUMilli(row.item.recommendedCpu))
    }
    if (row.memory) {
      memoryTotals = addTotals(
        memoryTotals,
        parseK8sQuantityBytes(row.item.currentMemory ?? ''),
        parseK8sQuantityBytes(row.item.recommendedMemory ?? ''),
      )
    }
  }

  return (
    <div className={variant === 'full' ? 'selection-summary selection-summary-full' : 'selection-summary'}>
      <div className="selection-summary-header">
        <div>
          <strong>{rows.length}</strong> container{rows.length === 1 ? '' : 's'} selected
          {cpuTotals.count > 0 && (
            <span className="selection-summary-total">
              CPU: {formatCPUMilli(cpuTotals.current)} → {formatCPUMilli(cpuTotals.recommended)}
            </span>
          )}
          {memoryTotals.count > 0 && (
            <span className="selection-summary-total">
              Memory: {formatMemoryBytes(memoryTotals.current)} → {formatMemoryBytes(memoryTotals.recommended)}
            </span>
          )}
        </div>
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
          <button className="btn btn-primary" disabled={busy} onClick={onApply}>
            {busy ? 'Applying…' : `Apply ${rows.length}`}
          </button>
        </div>
      </div>

      {error && <div className="panel panel-error selection-summary-error">{error}</div>}

      <ul className="selection-summary-list">
        {rows.map(({ item, cpu, memory }) => (
          <li key={`${item.namespace}/${item.vpaName}/${item.containerName}`}>
            <span className="selection-summary-item-name">
              {item.namespace}/{item.vpaName} · {item.containerName}
            </span>
            <span className="selection-summary-item-values">
              {cpu && (
                <span className="badge badge-muted">
                  cpu {item.currentCpu} → {item.recommendedCpu}
                </span>
              )}
              {memory && (
                <span className="badge badge-muted">
                  mem {formatMemory(item.currentMemory)} → {formatMemory(item.recommendedMemory)}
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
