import { formatCPU, formatMemory } from '../lib/format'
import { formatReasons } from '../lib/reasons'
import { Hint } from './Hint'

interface DeltaBadgeProps {
  current?: string
  /** What the delta is computed against: the VPA recommendation plus any recorded request headroom. */
  recommended?: string
  percent?: number
  kind: 'cpu' | 'memory'
  /** The recorded request headroom behind `recommended`, and the bare VPA value it was applied to. */
  headroomPercent?: number
  vpa?: string
  /**
   * Whether this change can actually be selected. Only then is the badge
   * colored by direction -- a green "decrease" on a resource that isn't
   * eligible reads as an invitation to apply it. Otherwise it's muted, with
   * the reasons in the tooltip.
   */
  eligible?: boolean
  reasons?: string[]
}

/**
 * The value a resource would move to, and by how much, with the full
 * breakdown (current value, VPA recommendation, headroom, and how the target
 * follows from them) in a hover tooltip rather than crammed into the badge.
 */
export function DeltaBadge({ current, recommended, percent, kind, headroomPercent, vpa, eligible = true, reasons }: DeltaBadgeProps) {
  const format = kind === 'memory' ? formatMemory : formatCPU

  if (!current || !recommended) {
    return <span className="badge badge-muted">n/a</span>
  }

  const rounded = percent === undefined ? undefined : Math.round(percent * 10) / 10
  const alreadyMatches = rounded === 0
  const badgeKind =
    !eligible || rounded === undefined
      ? 'badge-muted'
      : rounded > 0
        ? 'badge-increase'
        : rounded < 0
          ? 'badge-decrease'
          : 'badge-muted'
  const change = rounded === undefined ? '' : ` (${rounded > 0 ? '+' : ''}${rounded}%)`

  const breakdown = (
    <>
      Current request: <strong>{format(current)}</strong>
      <br />
      VPA recommendation: <strong>{format(vpa ?? recommended)}</strong>
      {headroomPercent ? (
        <>
          <br />
          Request headroom (last applied): <strong>{headroomPercent}%</strong>
          <br />
          Target = {format(vpa)} ÷ {(1 - headroomPercent / 100).toFixed(2)} = <strong>{format(recommended)}</strong>, so
          the VPA value is {100 - headroomPercent}% of the request.
        </>
      ) : null}
      {rounded !== undefined && (
        <>
          <br />
          Change from current: <strong>{`${rounded > 0 ? '+' : ''}${rounded}%`}</strong>
        </>
      )}
      {!eligible && !alreadyMatches && (
        <>
          <br />
          <br />
          <strong>Not eligible</strong>: {formatReasons(reasons) ?? 'not selectable'}
        </>
      )}
    </>
  )

  return (
    <>
      <Hint className="delta-hint" trigger={<span className={`badge ${badgeKind}`}>{format(recommended)}{change}</span>}>
        {breakdown}
      </Hint>
      {alreadyMatches && (
        <span className="badge badge-ok" aria-label="Already matches the target">
          ✓
        </span>
      )}
    </>
  )
}
