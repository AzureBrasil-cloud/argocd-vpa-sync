import type { LimitResource } from '../lib/limits'
import type { RequestHeadroomSettings } from '../lib/useRequestHeadroom'

const TOOLTIP =
  'Request headroom writes the request above the VPA recommendation, so the recommendation -- ' +
  'what the pod actually uses -- becomes (100 − N)% of the request: request = recommendation ÷ (1 − N/100), ' +
  'rounded up. E.g. 30% with a 100Mi recommendation writes 143Mi. This keeps a utilization-based HPA ' +
  "from scaling out a pod that's just at rest. The headroom is remembered per container once applied, " +
  "and later recommendations are compared against recommendation + headroom. 0 writes the bare recommendation."

const LABEL: Record<LimitResource, string> = { cpu: 'CPU request headroom', memory: 'Memory request headroom' }

interface RequestHeadroomInputProps {
  settings: RequestHeadroomSettings
  /** Only these resources get a row (e.g. just memory when only memory is selected). */
  resources: LimitResource[]
  disabled?: boolean
}

/** Per-resource headroom percentage the new request is written with over the VPA recommendation. */
export function RequestHeadroomInput({ settings, resources, disabled }: RequestHeadroomInputProps) {
  if (resources.length === 0) return null

  return (
    <div className="limit-settings">
      {resources.map((resource) => {
        const valid = settings.parsed[resource] !== null
        return (
          <div className="limit-settings-row" key={resource}>
            <span className="limit-settings-label">{LABEL[resource]}</span>
            <input
              type="text"
              inputMode="decimal"
              className="limit-settings-percent"
              value={settings.inputs[resource]}
              disabled={disabled}
              aria-invalid={!valid}
              aria-label={`${LABEL[resource]} percent`}
              placeholder="30"
              onChange={(e) => settings.setInput(resource, e.target.value)}
            />
            <span>%</span>
            <span className="limit-settings-info" title={TOOLTIP} aria-label={TOOLTIP} role="img">
              ⓘ
            </span>
            {!valid && <span className="limit-settings-error">must be ≥ 0 and &lt; 100</span>}
          </div>
        )
      })}
    </div>
  )
}
