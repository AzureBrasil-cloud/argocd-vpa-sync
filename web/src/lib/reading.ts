import type { RecommendationDTO } from '../api/types'
import type { DeltaBadgeProps } from '../components/DeltaBadge'
import type { LimitResource } from './limits'

/** Everything one resource's reading needs, pulled out of the DTO once. */
export function readingProps(item: RecommendationDTO, resource: LimitResource): DeltaBadgeProps & {
  bandLower?: string
  bandUpper?: string
} {
  if (resource === 'cpu') {
    return {
      kind: 'cpu',
      current: item.currentCpu,
      recommended: item.targetCpu ?? item.recommendedCpu,
      vpa: item.recommendedCpu,
      headroomPercent: item.cpuRequestHeadroomPercent,
      percent: item.deltaCpuPercent,
      eligible: item.cpuConfigured && item.cpuEligible,
      reasons: item.cpuEligibilityReasons,
      bandLower: item.bandLowerCpu ?? item.lowerBoundCpu,
      bandUpper: item.bandUpperCpu ?? item.upperBoundCpu,
    }
  }
  return {
    kind: 'memory',
    current: item.currentMemory,
    recommended: item.targetMemory ?? item.recommendedMemory,
    vpa: item.recommendedMemory,
    headroomPercent: item.memoryRequestHeadroomPercent,
    percent: item.deltaMemoryPercent,
    eligible: item.memoryConfigured && item.memoryEligible,
    reasons: item.memoryEligibilityReasons,
    bandLower: item.bandLowerMemory ?? item.lowerBoundMemory,
    bandUpper: item.bandUpperMemory ?? item.upperBoundMemory,
  }
}
