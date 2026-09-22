// Mirrors internal/api/dto.go -- keep in sync with the Go structs.

export interface WorkloadDTO {
  kind: string
  name: string
  namespace: string
}

export type RecommendationStatus =
  | 'new'
  | 'selected'
  | 'applying'
  | 'applied'
  | 'skipped'
  | 'failed'

export interface RecommendationDTO {
  cluster: string
  namespace: string
  vpaName: string
  workload: WorkloadDTO
  containerName: string
  updateMode: string

  recommendedCpu?: string
  recommendedMemory?: string
  lowerBoundCpu?: string
  lowerBoundMemory?: string
  upperBoundCpu?: string
  upperBoundMemory?: string

  currentCpu?: string
  currentMemory?: string

  deltaCpuAbsoluteMilli?: number
  deltaCpuPercent?: number
  deltaMemoryAbsoluteMilli?: number
  deltaMemoryPercent?: number

  recommendationAgeSeconds: number

  eligible: boolean
  eligibilityReasons?: string[]

  // cpuConfigured/memoryConfigured: does the VpaGitOpsBinding even declare a
  // key path for this resource on this container (drives whether a
  // selection checkbox appears at all). cpuEligible/memoryEligible: does
  // this resource individually clear the eligibility threshold (drives
  // whether the checkbox, once shown, is enabled).
  cpuConfigured: boolean
  cpuEligible: boolean
  cpuEligibilityReasons?: string[]
  memoryConfigured: boolean
  memoryEligible: boolean
  memoryEligibilityReasons?: string[]

  status: RecommendationStatus

  currentValueError?: string
  warnings?: string[]
  validationErrors?: string[]
}

export interface ListRecommendationsResponse {
  items: RecommendationDTO[]
}

// Mirrors domain.PendingSelection -- the response body of POST .../select.
export interface PendingSelectionDTO {
  idempotencyKey: string
  vpaNamespace: string
  vpaName: string
  containerName: string
  selectedAt: string
  selectedBy?: string
}

// Mirrors api.SelectRequest -- the body of POST .../select and
// POST .../bulk-select.
export interface SelectRequest {
  applyCPU: boolean
  applyMemory: boolean
}

