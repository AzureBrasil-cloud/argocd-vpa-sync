import { formatReasons } from '../lib/reasons'
import { Hint } from './Hint'

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

  const checkbox = (
    <input
      type="checkbox"
      className="resource-checkbox"
      checked={checked}
      disabled={!eligible}
      aria-label={label}
      onChange={(e) => onChange(e.target.checked)}
    />
  )
  if (eligible) return checkbox

  // A disabled input gets no mouse events in most browsers, so its own title
  // never shows: the wrapping Hint is what's hovered.
  return (
    <Hint className="resource-checkbox-hint" trigger={checkbox}>
      Not eligible: {formatReasons(reasons) ?? 'not eligible'}
    </Hint>
  )
}
