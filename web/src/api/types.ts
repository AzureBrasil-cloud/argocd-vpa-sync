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

  // The request headroom last applied to this container (absent = none),
  // and the recommendation plus that headroom -- what the live request and
  // the delta fields are compared against.
  cpuRequestHeadroomPercent?: number
  memoryRequestHeadroomPercent?: number
  targetCpu?: string
  targetMemory?: string

  // Live workload limits (absent when it declares none), and whether
  // write-back has a limit key path to keep in step with the request.
  currentCpuLimit?: string
  currentMemoryLimit?: string
  cpuLimitConfigured: boolean
  memoryLimitConfigured: boolean
  // The recommendation exceeds the live limit, so selecting that resource
  // must also set a new limit; otherwise setting one is optional.
  cpuLimitRequired: boolean
  memoryLimitRequired: boolean
  // The recommendation exceeds the live limit, managed or not. Exceeded but
  // not configured is a warning: write-back can't raise that limit.
  cpuLimitExceeded: boolean
  memoryLimitExceeded: boolean

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

  // Operation carries branch/commit/error detail for the most recent
  // write-back attempt, once one exists (status has moved past new/selected).
  operation?: OperationDTO
}

// OperationDTO mirrors internal/api/dto.go's OperationDTO.
export interface OperationDTO {
  branch?: string
  commitSha?: string
  prUrl?: string
  errorMessage?: string
  updatedAt: string
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
  // When set, that resource's limit is rewritten too (only if the resource
  // itself is applied). Omitted leaves it untouched.
  cpuLimit?: LimitSpecRequest
  memoryLimit?: LimitSpecRequest
  // Writes the request as recommendation / (1 - p/100), p in [0, 100), so
  // the recommendation is (100 - p)% of it. Omitted or 0: the bare
  // recommendation.
  cpuRequestHeadroomPercent?: number
  memoryRequestHeadroomPercent?: number
}

// Mirrors api.LimitSpecDTO: exactly one of headroomPercent (the new request
// becomes (100 - p)% of the limit, p in [0, 100)) or value (an absolute
// Kubernetes quantity, e.g. "512Mi" or "500m").
export interface LimitSpecRequest {
  headroomPercent?: number
  value?: string
}

