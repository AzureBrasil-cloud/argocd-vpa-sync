// What selecting one recommendation will write, per resource: the request
// (VPA recommendation + request headroom) and the limit decision that
// request implies. Shared by the list, the selection summary and the detail
// page so the preview and the request body never disagree.
import type { RecommendationDTO, SelectRequest } from '../api/types'
import { decideLimit, limitChange, proposedRequest, type LimitChange, type LimitDecision, type LimitResource } from './limits'
import type { LimitSettings } from './useLimitSettings'
import type { RequestHeadroomSettings } from './useRequestHeadroom'

export interface ResourcePlan {
  /** Request headroom percent (null while its input is invalid). */
  headroom: number | null
  /** Proposed request: millicores (CPU) or bytes (memory). */
  request: number | null
  limit: LimitDecision
  /** Limit preview, or null while the limit setting is invalid. */
  limitChange: LimitChange | null
}

export interface SelectionPlan {
  cpu: ResourcePlan | null
  memory: ResourcePlan | null
  /** A headroom or limit setting that has to be sent doesn't parse. */
  invalid: boolean
  /** Everything of the select request body but applyCPU/applyMemory. */
  body: Omit<SelectRequest, 'applyCPU' | 'applyMemory'>
}

export function planResource(
  item: RecommendationDTO,
  resource: LimitResource,
  limits: LimitSettings,
  headroom: RequestHeadroomSettings,
): ResourcePlan {
  const pct = headroom.parsed[resource]
  const request = pct === null ? null : proposedRequest(item, resource, pct)
  const limit = decideLimit(item, resource, limits, request)
  return {
    headroom: pct,
    request,
    limit,
    limitChange: limit.invalid ? null : limitChange(item, resource, limit.parsed, request),
  }
}

export function planSelection(
  item: RecommendationDTO,
  sel: { cpu: boolean; memory: boolean },
  limits: LimitSettings,
  headroom: RequestHeadroomSettings,
): SelectionPlan {
  const cpu = sel.cpu ? planResource(item, 'cpu', limits, headroom) : null
  const memory = sel.memory ? planResource(item, 'memory', limits, headroom) : null
  const invalid = [cpu, memory].some((p) => p !== null && (p.headroom === null || p.limit.invalid))
  return {
    cpu,
    memory,
    invalid,
    body: {
      cpuLimit: cpu?.limit.parsed?.spec,
      memoryLimit: memory?.limit.parsed?.spec,
      cpuRequestHeadroomPercent: cpu?.headroom ?? undefined,
      memoryRequestHeadroomPercent: memory?.headroom ?? undefined,
    },
  }
}
