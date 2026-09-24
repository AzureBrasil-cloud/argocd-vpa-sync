import { useState } from 'react'
import { parseHeadroomPercent, type LimitResource } from './limits'

const STORAGE_KEY = 'argocd-vpa-updater.requestHeadroom'
const ENABLED_STORAGE_KEY = 'argocd-vpa-updater.requestHeadroomEnabled'

/**
 * Default request headroom: the VPA recommendation (the pod's actual usage)
 * ends up at 70% of the request, so a utilization-based HPA doesn't scale
 * out a pod that's merely at rest.
 */
export const DEFAULT_REQUEST_HEADROOM_PERCENT = 30

export type RequestHeadroomInputs = Record<LimitResource, string>

export interface RequestHeadroomSettings {
  inputs: RequestHeadroomInputs
  setInput: (resource: LimitResource, raw: string) => void
  /** Whether a headroom is applied at all for each resource (it's optional). */
  enabled: Record<LimitResource, boolean>
  setEnabled: (resource: LimitResource, enabled: boolean) => void
  /**
   * Replaces the inputs (and turns them on) without remembering them -- e.g.
   * to start the detail page from the headroom a container was last applied
   * with.
   */
  seed: (values: Partial<Record<LimitResource, number>>) => void
  /**
   * Each resource's headroom percent to apply: 0 when turned off, null while
   * turned on with an invalid input.
   */
  parsed: Record<LimitResource, number | null>
}

function readEnabled(): Record<LimitResource, boolean> {
  try {
    const stored = JSON.parse(localStorage.getItem(ENABLED_STORAGE_KEY) ?? 'null') as Partial<
      Record<LimitResource, boolean>
    > | null
    return { cpu: stored?.cpu === true, memory: stored?.memory === true }
  } catch {
    return { cpu: false, memory: false }
  }
}

const DEFAULT_INPUT = String(DEFAULT_REQUEST_HEADROOM_PERCENT)

function readStored(): RequestHeadroomInputs {
  const fallback = { cpu: DEFAULT_INPUT, memory: DEFAULT_INPUT }
  try {
    const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? 'null') as Partial<RequestHeadroomInputs> | null
    if (!stored) return fallback
    const pick = (r: LimitResource) => {
      const v = stored[r]
      return typeof v === 'string' && parseHeadroomPercent(v) !== null ? v : DEFAULT_INPUT
    }
    return { cpu: pick('cpu'), memory: pick('memory') }
  } catch {
    // storage unavailable (private mode, blocked site data) or malformed
    return fallback
  }
}

/**
 * Per-resource, optional request headroom: how far above the VPA
 * recommendation the request is written (the recommendation becomes
 * (100 - N)% of it). Off by default; the on/off choice and valid values are
 * remembered per browser, like the limit settings.
 */
export function useRequestHeadroom(): RequestHeadroomSettings {
  const [inputs, setInputs] = useState<RequestHeadroomInputs>(readStored)
  const [enabled, setEnabledState] = useState<Record<LimitResource, boolean>>(readEnabled)

  function setEnabled(resource: LimitResource, value: boolean) {
    setEnabledState((prev) => {
      const next = { ...prev, [resource]: value }
      try {
        localStorage.setItem(ENABLED_STORAGE_KEY, JSON.stringify(next))
      } catch {
        // not persisted; the in-memory value still applies
      }
      return next
    })
  }

  function setInput(resource: LimitResource, raw: string) {
    setInputs((prev) => {
      const next = { ...prev, [resource]: raw }
      try {
        const persisted = readStored()
        for (const r of ['cpu', 'memory'] as const) {
          if (parseHeadroomPercent(next[r]) !== null) persisted[r] = next[r]
        }
        localStorage.setItem(STORAGE_KEY, JSON.stringify(persisted))
      } catch {
        // not persisted; the in-memory value still applies
      }
      return next
    })
  }

  function seed(values: Partial<Record<LimitResource, number>>) {
    setInputs((prev) => ({
      cpu: values.cpu !== undefined ? String(values.cpu) : prev.cpu,
      memory: values.memory !== undefined ? String(values.memory) : prev.memory,
    }))
    setEnabledState((prev) => ({
      cpu: values.cpu !== undefined ? true : prev.cpu,
      memory: values.memory !== undefined ? true : prev.memory,
    }))
  }

  const effective = (r: LimitResource) => (enabled[r] ? parseHeadroomPercent(inputs[r]) : 0)

  return {
    inputs,
    setInput,
    enabled,
    setEnabled,
    seed,
    parsed: { cpu: effective('cpu'), memory: effective('memory') },
  }
}
