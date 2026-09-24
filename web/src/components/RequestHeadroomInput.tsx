import type { LimitResource } from '../lib/limits'
import type { RequestHeadroomSettings } from '../lib/useRequestHeadroom'

const LABEL: Record<LimitResource, string> = { cpu: 'CPU request headroom', memory: 'Memory request headroom' }

function Tooltip() {
  return (
    <span className="hint-bubble" role="tooltip">
      <strong>Why?</strong> The VPA recommends what the pod actually uses. Written as-is, the request equals that
      usage, so a pod merely at rest already runs at ~100% of its request -- and an HPA, which scales on a
      percentage of the request, scales it out for no reason.
      <br />
      <br />
      With a headroom of N%, the request is written as recommendation ÷ (1 − N/100), so the pod's usage sits at
      (100 − N)% of it. E.g. 30% over a 100Mi recommendation writes 143Mi (100Mi is 70% of it).
      <br />
      <br />
      Once applied, the headroom is remembered for the container and later recommendations are compared against
      recommendation + headroom, so it isn't offered a change back down. Unticked, the bare recommendation is
      written (clearing any remembered headroom).
    </span>
  )
}

interface RequestHeadroomInputProps {
  settings: RequestHeadroomSettings
  /** Only these resources get a row (e.g. just memory when only memory is selected). */
  resources: LimitResource[]
  disabled?: boolean
}

/**
 * Per-resource, optional headroom the new request is written with over the
 * VPA recommendation: a checkbox turns it on, revealing the percentage.
 */
export function RequestHeadroomInput({ settings, resources, disabled }: RequestHeadroomInputProps) {
  if (resources.length === 0) return null

  return (
    <div className="limit-settings">
      {resources.map((resource) => {
        const enabled = settings.enabled[resource]
        const valid = settings.parsed[resource] !== null
        return (
          <div className="limit-settings-row" key={resource}>
            <label className="limit-settings-toggle limit-settings-label hint">
              <input
                type="checkbox"
                checked={enabled}
                disabled={disabled}
                onChange={(e) => settings.setEnabled(resource, e.target.checked)}
              />
              {LABEL[resource]}
              <span className="limit-settings-info" aria-hidden="true">
                ⓘ
              </span>
              <Tooltip />
            </label>
            {enabled ? (
              <>
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
                {!valid && <span className="limit-settings-error">must be ≥ 0 and &lt; 100</span>}
              </>
            ) : (
              <span className="muted">off -- writes the bare VPA recommendation</span>
            )}
          </div>
        )
      })}
    </div>
  )
}
