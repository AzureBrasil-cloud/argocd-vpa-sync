import { DEFAULT_LIMIT_HEADROOM_PERCENT, LIMIT_VALUE_HINT, type LimitMode, type LimitResource } from '../lib/limits'
import type { LimitSettings } from '../lib/useLimitSettings'

const HEADROOM_TOOLTIP =
  'Headroom is how much room the limit leaves above the request: the new request becomes ' +
  '(100 − N)% of the limit, i.e. limit = request ÷ (1 − N/100), rounded up. ' +
  'E.g. 20% with a 305Mi request gives a 382Mi limit (the request is 80% of it). ' +
  'Must be ≥ 0 and < 100 -- at 100% the limit would be infinite.'

const ABSOLUTE_TOOLTIP =
  'The limit is set to exactly this value, whatever the request. It must not be below the new request -- ' +
  'Kubernetes rejects a request greater than its limit.'

const REQUIRED_TOOLTIP =
  'The new request exceeds the current limit, and Kubernetes rejects a request greater than its limit, ' +
  'so a new limit has to be written along with it.'

const LABEL: Record<LimitResource, string> = { cpu: 'CPU limit', memory: 'Memory limit' }

/** How many of the selected containers need (or may take) a new limit for one resource. */
export interface LimitResourceCounts {
  resource: LimitResource
  required: number
  optional: number
}

interface LimitSettingsInputProps {
  settings: LimitSettings
  /** Resources with no required or optional container get no row. */
  resources: LimitResourceCounts[]
  disabled?: boolean
}

/**
 * Per-resource choice of how write-back sets the limit alongside the new
 * request: by headroom percentage or to an absolute Kubernetes quantity.
 * Where the new request exceeds the current limit a new limit is required,
 * so the fields are shown with no opt-out; where it fits, updating the limit
 * is optional behind a checkbox (off by default, leaving the limit
 * untouched). A limit the manifest doesn't declare is never added either way.
 */
export function LimitSettingsInput({ settings, resources, disabled }: LimitSettingsInputProps) {
  const rows = resources.filter((r) => r.required + r.optional > 0)
  if (rows.length === 0) return null

  return (
    <div className="limit-settings">
      {rows.map(({ resource, required, optional }) => {
        const input = settings.inputs[resource]
        const valid = settings.parsed[resource] !== null
        const tooltip = input.mode === 'headroom' ? HEADROOM_TOOLTIP : ABSOLUTE_TOOLTIP
        const showFields = required > 0 || settings.update[resource]
        const total = required + optional

        function setMode(mode: LimitMode) {
          settings.setInput(resource, { mode, raw: mode === 'headroom' ? String(DEFAULT_LIMIT_HEADROOM_PERCENT) : '' })
        }

        return (
          <div className="limit-settings-row" key={resource}>
            <span className="limit-settings-label">{LABEL[resource]}</span>
            {required > 0 && (
              <span className="badge badge-increase" title={REQUIRED_TOOLTIP}>
                required{total > 1 ? ` for ${required} of ${total}` : ''}
              </span>
            )}
            {optional > 0 && (
              <label className="limit-settings-toggle">
                <input
                  type="checkbox"
                  checked={settings.update[resource]}
                  disabled={disabled}
                  onChange={(e) => settings.setUpdate(resource, e.target.checked)}
                />
                {required > 0 ? `also update the other ${optional}` : 'update'}
              </label>
            )}
            {showFields ? (
              <>
                <select
                  value={input.mode}
                  disabled={disabled}
                  aria-label={`${LABEL[resource]} mode`}
                  onChange={(e) => setMode(e.target.value as LimitMode)}
                >
                  <option value="headroom">Headroom %</option>
                  <option value="absolute">Absolute value</option>
                </select>
                <input
                  type="text"
                  inputMode={input.mode === 'headroom' ? 'decimal' : 'text'}
                  className={input.mode === 'headroom' ? 'limit-settings-percent' : 'limit-settings-value'}
                  value={input.raw}
                  disabled={disabled}
                  aria-invalid={!valid}
                  aria-label={`${LABEL[resource]} ${input.mode === 'headroom' ? 'headroom percent' : 'value'}`}
                  placeholder={input.mode === 'headroom' ? '20' : resource === 'memory' ? '512Mi' : '500m'}
                  onChange={(e) => settings.setInput(resource, { mode: input.mode, raw: e.target.value })}
                />
                {input.mode === 'headroom' && <span>%</span>}
                <span className="limit-settings-info" title={tooltip} aria-label={tooltip} role="img">
                  ⓘ
                </span>
                {!valid && (
                  <span className="limit-settings-error">
                    {input.mode === 'headroom' ? 'must be ≥ 0 and < 100' : `invalid -- ${LIMIT_VALUE_HINT[resource]}`}
                  </span>
                )}
              </>
            ) : (
              <span className="muted">unchanged</span>
            )}
          </div>
        )
      })}
    </div>
  )
}
