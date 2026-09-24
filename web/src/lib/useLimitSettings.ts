import { useState } from 'react'
import {
  DEFAULT_LIMIT_HEADROOM_PERCENT,
  parseLimitInput,
  type LimitInput,
  type LimitResource,
  type ParsedLimit,
} from './limits'

const STORAGE_KEY = 'argocd-vpa-updater.limitSettings'

export type LimitInputs = Record<LimitResource, LimitInput>

export interface LimitSettings {
  inputs: LimitInputs
  setInput: (resource: LimitResource, input: LimitInput) => void
  /** Each resource's parsed setting, or null while its input is invalid. */
  parsed: Record<LimitResource, ParsedLimit | null>
  /**
   * Whether to also write the limit where it's optional (the new request
   * fits under the current one). A required limit is written regardless.
   */
  update: Record<LimitResource, boolean>
  setUpdate: (resource: LimitResource, update: boolean) => void
}

const DEFAULT_INPUT: LimitInput = { mode: 'headroom', raw: String(DEFAULT_LIMIT_HEADROOM_PERCENT) }

function readStored(): LimitInputs {
  const fallback = { cpu: DEFAULT_INPUT, memory: DEFAULT_INPUT }
  try {
    const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? 'null') as Partial<LimitInputs> | null
    if (!stored) return fallback
    const pick = (r: LimitResource) => {
      const v = stored[r]
      return v && (v.mode === 'headroom' || v.mode === 'absolute') && typeof v.raw === 'string' && parseLimitInput(v, r)
        ? v
        : DEFAULT_INPUT
    }
    return { cpu: pick('cpu'), memory: pick('memory') }
  } catch {
    // storage unavailable (private mode, blocked site data) or malformed
    return fallback
  }
}

/**
 * Per-resource limit settings (headroom % or absolute value). Valid
 * settings are remembered per browser, so the list and detail pages share
 * them. The opt-in to update optional limits is deliberately not
 * remembered: it starts off on every page, so a limit is never changed
 * unless it has to be or the user just asked for it.
 */
export function useLimitSettings(): LimitSettings {
  const [inputs, setInputs] = useState<LimitInputs>(readStored)
  const [update, setUpdateState] = useState<Record<LimitResource, boolean>>({ cpu: false, memory: false })

  function setUpdate(resource: LimitResource, value: boolean) {
    setUpdateState((prev) => ({ ...prev, [resource]: value }))
  }

  function setInput(resource: LimitResource, input: LimitInput) {
    setInputs((prev) => {
      const next = { ...prev, [resource]: input }
      try {
        const persisted = readStored()
        for (const r of ['cpu', 'memory'] as const) {
          if (parseLimitInput(next[r], r)) persisted[r] = next[r]
        }
        localStorage.setItem(STORAGE_KEY, JSON.stringify(persisted))
      } catch {
        // not persisted; the in-memory value still applies
      }
      return next
    })
  }

  return {
    inputs,
    setInput,
    parsed: { cpu: parseLimitInput(inputs.cpu, 'cpu'), memory: parseLimitInput(inputs.memory, 'memory') },
    update,
    setUpdate,
  }
}
