import { formatReasons } from '../lib/reasons'

interface ResourceCheckboxProps {
  configured: boolean
  eligible: boolean
  reasons?: string[]
  checked: boolean
  onChange: (checked: boolean) => void
  label: string
}

/**
 * A selection checkbox for one resource (CPU or memory) of one
 * recommendation. Renders nothing (a placeholder dash) if the binding
 * doesn't even configure this resource; renders a disabled checkbox with an
 * explanatory tooltip if configured but not individually eligible; renders
 * a normal checkbox otherwise.
 */
export function ResourceCheckbox({ configured, eligible, reasons, checked, onChange, label }: ResourceCheckboxProps) {
  if (!configured) {
    return <span className="cell-muted resource-checkbox-placeholder">—</span>
  }

  const title = eligible ? undefined : formatReasons(reasons) ?? 'Not eligible'

  return (
    <input
      type="checkbox"
      className="resource-checkbox"
      checked={checked}
      disabled={!eligible}
      title={title}
      aria-label={label}
      onChange={(e) => onChange(e.target.checked)}
    />
  )
}
