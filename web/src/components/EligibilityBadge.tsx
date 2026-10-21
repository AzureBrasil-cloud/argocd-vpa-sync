import { formatReasons } from '../lib/reasons'

interface EligibilityBadgeProps {
  eligible: boolean
  reasons?: string[]
  /** Which resources are eligible, shown under the verdict (e.g. "memory"). */
  detail?: string
  /** The error that kept the current value from being read, if any. */
  error?: string
}

/**
 * The combined eligibility verdict with its reason underneath, so the "why"
 * is readable without hovering. The mark is solid for eligible and hatched
 * for not eligible, so the verdict doesn't rest on colour alone.
 */
export function EligibilityBadge({ eligible, reasons, detail, error }: EligibilityBadgeProps) {
  if (error) {
    return (
      <span className="verdict-cell" title={error}>
        <span className="eligibility eligibility-error">
          <span className="mark mark-error" aria-hidden="true" />
          Error
        </span>
        <span className="verdict-reason">current value unavailable</span>
      </span>
    )
  }
  if (eligible) {
    return (
      <span className="verdict-cell">
        <span className="eligibility eligibility-yes">
          <span className="mark mark-solid" aria-hidden="true" />
          Eligible
        </span>
        {detail && <span className="verdict-reason">{detail}</span>}
      </span>
    )
  }
  const label = formatReasons(reasons)
  return (
    <span className="verdict-cell" title={label}>
      <span className="eligibility eligibility-no">
        <span className="mark mark-hatched" aria-hidden="true" />
        Not eligible
      </span>
      {label && <span className="verdict-reason">{shortReason(reasons)}</span>}
    </span>
  )
}

/** A short form of the first reason, for the line under the verdict. */
function shortReason(reasons?: string[]): string | undefined {
  const first = reasons?.[0]
  switch (first) {
    case 'within-vpa-bounds':
      return 'within VPA range'
    case 'below-minimum-change-threshold':
      return 'below change threshold'
    case undefined:
      return undefined
    default:
      return formatReasons([first])
  }
}
