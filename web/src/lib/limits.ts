// Limit write-back settings and their preview -- mirrors
// internal/domain/limits.go. Each resource's limit is set either by
// headroom (limit = request / (1 - headroom/100), rounded up) or to an
// absolute quantity. Display-only for the headroom mode: the server derives
// the real value from the request as it's actually written to the file, so
// the committed limit can differ from this preview by a unit of rounding.
import type { LimitSpecRequest, RecommendationDTO } from '../api/types'
import { formatCPUMilli, formatMemoryBytes, parseCPUMilli, parseK8sQuantityBytes } from './format'

export const DEFAULT_LIMIT_HEADROOM_PERCENT = 20

export type LimitResource = 'cpu' | 'memory'
export type LimitMode = 'headroom' | 'absolute'

/** What the user typed for one resource's limit. */
export interface LimitInput {
  mode: LimitMode
  raw: string
}

/** A validated LimitInput: the request body to send, plus what it means numerically. */
export type ParsedLimit =
  | { mode: 'headroom'; percent: number; spec: LimitSpecRequest }
  | { mode: 'absolute'; value: number; spec: LimitSpecRequest }

// Kubernetes quantity grammar, restricted to what makes sense per resource:
// memory takes binary (Ki..Ei) or decimal (k..E) suffixes, a plain byte
// count, or an exponent (129e6) -- never "m" (millibytes); CPU takes cores
// ("0.5", "2") or millicores ("500m").
const MEMORY_QUANTITY = /^(\d+(\.\d+)?|\.\d+)([KMGTPE]i|[kMGTPE]|[eE][+-]?\d+)?$/
const CPU_QUANTITY = /^(\d+(\.\d+)?|\.\d+)m?$/

export const LIMIT_VALUE_HINT: Record<LimitResource, string> = {
  memory: 'e.g. 512Mi, 1Gi, 1.5G (Ki, Mi, Gi, Ti, Pi, Ei, k, M, G, T, P, E)',
  cpu: 'e.g. 500m, 0.5, 2 (millicores with m, or cores)',
}

/** Parses the headroom text; null unless it's a number in [0, 100). */
export function parseHeadroomPercent(raw: string): number | null {
  if (raw.trim() === '') return null
  const n = Number(raw)
  if (!Number.isFinite(n) || n < 0 || n >= 100) return null
  return n
}

/**
 * Parses an absolute limit into bytes (memory) or millicores (CPU); null
 * unless it's a positive Kubernetes quantity valid for that resource, with
 * no finer precision than Kubernetes keeps (whole bytes, whole millicores).
 */
export function parseLimitValue(raw: string, resource: LimitResource): number | null {
  const trimmed = raw.trim()
  if (resource === 'memory') {
    if (!MEMORY_QUANTITY.test(trimmed)) return null
    const bytes = parseK8sQuantityBytes(trimmed)
    if (bytes === null || bytes <= 0 || !Number.isInteger(bytes)) return null
    return bytes
  }
  if (!CPU_QUANTITY.test(trimmed)) return null
  const milli = parseCPUMilli(trimmed)
  if (milli === null || milli <= 0 || !Number.isInteger(Math.round(milli * 1e6) / 1e6)) return null
  return milli
}

export function parseLimitInput(input: LimitInput, resource: LimitResource): ParsedLimit | null {
  if (input.mode === 'headroom') {
    const percent = parseHeadroomPercent(input.raw)
    return percent === null ? null : { mode: 'headroom', percent, spec: { headroomPercent: percent } }
  }
  const value = parseLimitValue(input.raw, resource)
  return value === null ? null : { mode: 'absolute', value, spec: { value: input.raw.trim() } }
}

export function limitFromRequest(request: number, headroomPercent: number): number {
  return Math.ceil((request * 100) / (100 - headroomPercent))
}

export interface LimitChange {
  /** Formatted live limit, or null when the workload declares none. */
  current: string | null
  /** Formatted new limit, or null when nothing will be written. */
  next: string | null
  currentValue: number | null
  nextValue: number | null
  decreases: boolean
  /** An absolute limit below the new request -- Kubernetes would reject it. */
  belowRequest: boolean
  /** Why no limit will be written, when next is null. */
  reason?: string
}

const FORMAT: Record<LimitResource, (v: number) => string> = {
  cpu: formatCPUMilli,
  memory: formatMemoryBytes,
}

const PARSE: Record<LimitResource, (raw: string) => number | null> = {
  cpu: (raw) => parseCPUMilli(raw),
  memory: parseK8sQuantityBytes,
}

export function limitChange(item: RecommendationDTO, resource: LimitResource, limit: ParsedLimit): LimitChange {
  const configured = resource === 'cpu' ? item.cpuLimitConfigured : item.memoryLimitConfigured
  const currentRaw = resource === 'cpu' ? item.currentCpuLimit : item.currentMemoryLimit
  const recommendedRaw = resource === 'cpu' ? item.recommendedCpu : item.recommendedMemory
  const parse = PARSE[resource]
  const format = FORMAT[resource]

  const currentValue = currentRaw ? parse(currentRaw) : null
  const current = currentValue === null ? null : format(currentValue)
  const none = { current, currentValue, next: null, nextValue: null, decreases: false, belowRequest: false }
  if (!configured) return { ...none, reason: 'limit not managed' }
  if (currentValue === null) return { ...none, reason: 'no limit (not created)' }
  const request = recommendedRaw ? parse(recommendedRaw) : null
  if (request === null) return none

  const nextValue = limit.mode === 'headroom' ? limitFromRequest(request, limit.percent) : limit.value
  return {
    current,
    currentValue,
    next: limit.mode === 'absolute' ? String(limit.spec.value) : format(nextValue),
    nextValue,
    decreases: nextValue < currentValue,
    belowRequest: nextValue < request,
  }
}
