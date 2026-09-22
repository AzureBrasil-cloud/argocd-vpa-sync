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

const LABEL: Record<LimitResource, string> = { cpu: 'CPU limit', memory: 'Memory limit' }

interface LimitSettingsInputProps {
  settings: LimitSettings
  /** Only these resources get a row (e.g. just memory when only memory is selected). */
  resources: LimitResource[]
  disabled?: boolean
}

/**
 * Per-resource choice of how write-back sets the limit alongside the new
 * request: by headroom percentage or to an absolute Kubernetes quantity.
 * A limit the manifest doesn't declare is never added either way.
 */
export function LimitSettingsInput({ settings, resources, disabled }: LimitSettingsInputProps) {
  if (resources.length === 0) return null

  return (
    <div className="limit-settings">
      {resources.map((resource) => {
        const input = settings.inputs[resource]
        const valid = settings.parsed[resource] !== null
        const tooltip = input.mode === 'headroom' ? HEADROOM_TOOLTIP : ABSOLUTE_TOOLTIP

        function setMode(mode: LimitMode) {
          settings.setInput(resource, { mode, raw: mode === 'headroom' ? String(DEFAULT_LIMIT_HEADROOM_PERCENT) : '' })
        }

        return (
          <div className="limit-settings-row" key={resource}>
            <span className="limit-settings-label">{LABEL[resource]}</span>
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
          </div>
        )
      })}
    </div>
  )
}
