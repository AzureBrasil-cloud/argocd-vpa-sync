import { formatReasons } from '../lib/reasons'

interface EligibilityBadgeProps {
  eligible: boolean
  reasons?: string[]
}

export function EligibilityBadge({ eligible, reasons }: EligibilityBadgeProps) {
  if (eligible) {
    return <span className="badge badge-eligible">Eligible</span>
  }
  const label = formatReasons(reasons)
  return (
    <span className="badge badge-ineligible" title={label}>
      Not eligible
    </span>
  )
}
