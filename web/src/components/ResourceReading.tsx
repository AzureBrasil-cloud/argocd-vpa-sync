import type { RecommendationDTO } from '../api/types'
import { formatCPU, formatMemory, formatPercent, parseCPUMilli, parseK8sQuantityBytes, roundPercent } from '../lib/format'
import { readingProps } from '../lib/reading'
import type { LimitResource } from '../lib/limits'
import { useTheme } from '../theme'
import { DeltaBadge, DeltaBreakdown } from './DeltaBadge'
import { Hint } from './Hint'

interface GaugeGeometry {
  current: number
  target: number
  band: { left: number; width: number } | null
  /** The live request sits outside the VPA's (headroom-scaled) range. */
  outOfBand: boolean
}

/**
 * Places current, target and the VPA range on one scale, padded so no mark
 * sits on the gauge's edge. Positions are percentages of the gauge width.
 */
function geometry(kind: LimitResource, current?: string, target?: string, lower?: string, upper?: string): GaugeGeometry | null {
  const parse = kind === 'cpu' ? parseCPUMilli : (raw?: string) => (raw ? parseK8sQuantityBytes(raw) : null)
  const c = parse(current)
  const t = parse(target)
  if (c === null || t === null) return null
  const lo = parse(lower)
  const hi = parse(upper)

  const values = [c, t, lo, hi].filter((v): v is number => v !== null)
  const min = Math.min(...values)
  const max = Math.max(...values)
  const pad = (max - min || max || 1) * 0.3
  const from = Math.max(0, min - pad)
  const to = max + pad
  const pos = (v: number) => ((v - from) / (to - from)) * 100

  const band = lo !== null && hi !== null ? { left: pos(lo), width: Math.max(pos(hi) - pos(lo), 0.8) } : null
  return {
    current: pos(c),
    target: pos(t),
    band,
    outOfBand: lo !== null && hi !== null && (c < lo || c > hi),
  }
}

interface GaugeProps {
  kind: LimitResource
  geo: GaugeGeometry
  eligible: boolean
  label: string
}

/**
 * The recommendation drawn against the VPA's range: the range as a band,
 * the live request as a tick, and the move to the recommendation as a bar
 * whose arrowhead points the way it goes. Direction is carried by shape and
 * eligibility by fill (solid vs hatched), so neither depends on colour.
 * fitting-room styles it as a tape measure, calibration as a tolerance bar.
 */
function Gauge({ geo, eligible, label }: GaugeProps) {
  const direction = geo.target > geo.current ? 'up' : geo.target < geo.current ? 'down' : 'flat'
  const moveLeft = Math.min(geo.current, geo.target)
  const moveWidth = Math.abs(geo.target - geo.current)
  return (
    <span
      className={`gauge gauge-${direction}${eligible ? ' gauge-eligible' : ''}${geo.outOfBand ? ' gauge-out' : ''}`}
      role="img"
      aria-label={label}
    >
      <span className="gauge-scale" />
      {geo.band && <span className="gauge-band" style={{ left: `${geo.band.left}%`, width: `${geo.band.width}%` }} />}
      {direction !== 'flat' && (
        <span className="gauge-move" style={{ left: `${moveLeft}%`, width: `${moveWidth}%` }} />
      )}
      <span className="gauge-target" style={{ left: `${geo.target}%` }} />
      <span className="gauge-current" style={{ left: `${geo.current}%` }} />
    </span>
  )
}

interface ResourceReadingProps {
  item: RecommendationDTO
  resource: LimitResource
  /** Larger gauge for the detail page. */
  size?: 'row' | 'detail'
}

/**
 * One resource's recommendation as the active theme reads it: delta badges
 * in the default theme, a tape-measure gauge in fitting-room, a tolerance
 * bar with an IN/OUT verdict in calibration. The breakdown is a tooltip in
 * all three.
 */
export function ResourceReading({ item, resource, size = 'row' }: ResourceReadingProps) {
  const theme = useTheme()
  const props = readingProps(item, resource)

  if (theme === 'default') return <DeltaBadge {...props} />

  const format = resource === 'cpu' ? formatCPU : formatMemory
  if (!props.current || !props.recommended) {
    return <span className="reading reading-empty">n/a</span>
  }

  const geo = geometry(resource, props.current, props.recommended, props.bandLower, props.bandUpper)
  const rounded = roundPercent(props.percent)
  const eligible = props.eligible ?? false
  const direction = rounded === undefined || rounded === 0 ? 'flat' : rounded > 0 ? 'up' : 'down'
  const resourceName = resource === 'cpu' ? 'CPU' : 'Memory'
  const range =
    props.bandLower && props.bandUpper ? `, VPA range ${format(props.bandLower)} to ${format(props.bandUpper)}` : ''
  const label = `${resourceName}: request ${format(props.current)}, recommended ${format(props.recommended)}${range}`

  const values = (
    <span className="reading-values">
      <span className="reading-current">{format(props.current)}</span>
      <span className="reading-arrow" aria-hidden="true">→</span>
      <span className="reading-target">{format(props.recommended)}</span>
      {rounded !== undefined && (
        <span className={`reading-delta reading-delta-${direction}${eligible ? ' reading-delta-eligible' : ''}`}>
          {formatPercent(rounded)}
        </span>
      )}
    </span>
  )

  return (
    <Hint
      className={`reading reading-${size}`}
      trigger={
        <span className="reading-body">
          {theme === 'calibration' && values}
          {geo && (
            <span className="reading-gauge-row">
              <Gauge kind={resource} geo={geo} eligible={eligible} label={label} />
              {theme === 'calibration' && props.bandLower && props.bandUpper && (
                <span className={`verdict ${geo.outOfBand ? 'verdict-out' : 'verdict-in'}`}>
                  {geo.outOfBand ? 'OUT' : 'IN'}
                </span>
              )}
            </span>
          )}
          {theme === 'fitting-room' && values}
        </span>
      }
    >
      <DeltaBreakdown {...props} />
    </Hint>
  )
}
