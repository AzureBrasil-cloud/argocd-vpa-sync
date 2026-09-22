// Mirrors the reason codes internal/eligibility returns (see
// internal/eligibility/eligibility.go's Reason* constants).
export const REASON_LABELS: Record<string, string> = {
  'no-current-value': 'no current value from the live workload',
  'no-recommendation': 'no recommendation yet',
  'below-minimum-change-threshold': 'below minimum change threshold',
  'container-excluded': 'container excluded',
  'recommendation-below-min-allowed': "below the VPA's minAllowed",
  'recommendation-above-max-allowed': "above the VPA's maxAllowed",
  'no-resource-selected': 'no resource selected',
}

export function formatReasons(reasons?: string[]): string | undefined {
  if (!reasons || reasons.length === 0) return undefined
  const labels = reasons.map((r) => REASON_LABELS[r] ?? r)
  // CPU and memory can independently fail for the same reason (e.g. both
  // below the minimum change threshold) -- collapse duplicates so the
  // combined message doesn't repeat itself.
  return [...new Set(labels)].join(', ')
}
