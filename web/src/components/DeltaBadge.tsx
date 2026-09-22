import { formatCPU, formatMemory } from '../lib/format'

interface DeltaBadgeProps {
  current?: string
  recommended?: string
  percent?: number
  kind: 'cpu' | 'memory'
}

export function DeltaBadge({ current, recommended, percent, kind }: DeltaBadgeProps) {
  const format = kind === 'memory' ? formatMemory : formatCPU

  if (!current || !recommended) {
    return <span className="badge badge-muted">n/a</span>
  }

  const currentLabel = format(current)
  const recommendedLabel = format(recommended)
  const title = `${current} → ${recommended}`

  if (percent === undefined) {
    return (
      <span className="badge badge-muted" title={title}>
        {currentLabel} → {recommendedLabel}
      </span>
    )
  }

  const rounded = Math.round(percent * 10) / 10
  const sign = rounded > 0 ? '+' : ''
  const alreadyMatches = rounded === 0
  const badgeKind = rounded > 0 ? 'badge-increase' : rounded < 0 ? 'badge-decrease' : 'badge-muted'

  return (
    <>
      <span className={`badge ${badgeKind}`} title={title}>
        {currentLabel} → {recommendedLabel} ({sign}
        {rounded}%)
      </span>
      {alreadyMatches && (
        <span className="badge badge-ok" title="Already matches the recommendation" aria-label="Already matches the recommendation">
          ✓
        </span>
      )}
    </>
  )
}
