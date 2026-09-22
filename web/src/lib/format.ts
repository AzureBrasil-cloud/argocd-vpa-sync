// Display-only formatting helpers. These never touch the values the API
// actually returns (internal/api/dto.go keeps exact byte/milli precision,
// since a selected recommendation's exact value becomes the write-back
// idempotency key) -- they only affect what's rendered on screen.

const BINARY_UNITS = ['Ki', 'Mi', 'Gi', 'Ti', 'Pi', 'Ei'] as const
const DECIMAL_UNITS = ['k', 'M', 'G', 'T', 'P', 'E'] as const

/**
 * Parses a Kubernetes quantity string (e.g. "256Mi", "319376137", "1.5G")
 * into a plain byte count. Returns null if the string can't be parsed as a
 * quantity at all.
 */
export function parseK8sQuantityBytes(raw: string): number | null {
  const trimmed = raw.trim()
  if (trimmed === '') return null

  const match = trimmed.match(/^([+-]?[0-9]*\.?[0-9]+)([a-zA-Z]*)$/)
  if (!match) return null

  const value = Number(match[1])
  if (Number.isNaN(value)) return null

  const suffix = match[2]
  if (suffix === '') return value

  const binaryIdx = BINARY_UNITS.indexOf(suffix as (typeof BINARY_UNITS)[number])
  if (binaryIdx !== -1) return value * Math.pow(1024, binaryIdx + 1)

  const decimalIdx = DECIMAL_UNITS.indexOf(suffix as (typeof DECIMAL_UNITS)[number])
  if (decimalIdx !== -1) return value * Math.pow(1000, decimalIdx + 1)

  return null
}

/**
 * Formats a Kubernetes memory quantity in the same style kubectl/k8s tooling
 * uses (binary units: Ki/Mi/Gi/Ti), rounded to a human-readable precision --
 * e.g. "319376137" -> "305Mi", "256Mi" -> "256Mi", "512Mi" -> "512Mi".
 * Falls back to the raw string if it can't be parsed as a quantity.
 */
/** Formats a byte count the same way formatMemory formats a quantity string -- shared so aggregates (sums of several quantities) can be formatted without round-tripping through a string. */
export function formatMemoryBytes(bytes: number): string {
  let unitIndex = -1
  let scaled = Math.abs(bytes)
  while (scaled >= 1024 && unitIndex < BINARY_UNITS.length - 1) {
    scaled /= 1024
    unitIndex++
  }

  const sign = bytes < 0 ? '-' : ''
  if (unitIndex === -1) {
    return `${sign}${scaled}`
  }
  const rounded = scaled >= 10 ? Math.round(scaled) : Math.round(scaled * 10) / 10
  return `${sign}${rounded}${BINARY_UNITS[unitIndex]}`
}

export function formatMemory(raw: string | undefined): string {
  if (!raw) return ''
  const bytes = parseK8sQuantityBytes(raw)
  if (bytes === null) return raw
  return formatMemoryBytes(bytes)
}

/** CPU quantities from this API are already compact (e.g. "15m", "250m", "1"); no reformatting needed. */
export function formatCPU(raw: string | undefined): string {
  return raw ?? ''
}

/** Parses a Kubernetes CPU quantity ("500m", "1", "1.5") into millicores. Returns null if unparseable. */
export function parseCPUMilli(raw: string | undefined): number | null {
  if (!raw) return null
  const trimmed = raw.trim()
  if (trimmed === '') return null
  if (trimmed.endsWith('m')) {
    const n = Number(trimmed.slice(0, -1))
    return Number.isNaN(n) ? null : n
  }
  const n = Number(trimmed)
  return Number.isNaN(n) ? null : n * 1000
}

/** Formats a millicore count back into a compact CPU quantity ("500m", "1", "1.5"). */
export function formatCPUMilli(milli: number): string {
  if (milli % 1000 === 0) return `${milli / 1000}`
  return `${Math.round(milli)}m`
}

export function formatAge(seconds: number): string {
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h`
  return `${Math.floor(hours / 24)}d`
}
