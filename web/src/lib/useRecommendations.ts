import { useCallback, useEffect, useState } from 'react'
import { listRecommendations } from '../api/client'
import type { RecommendationDTO } from '../api/types'

export interface Recommendations {
  items: RecommendationDTO[] | null
  error: string | null
  reload: () => Promise<void>
}

/**
 * The recommendation list shared by the Recommendations, Write-back history
 * and Bindings pages. The write-back worker picks a "selected" item up
 * asynchronously (up to its poll interval later, see
 * internal/writebackworker) and moves it through applying -> applied/failed,
 * so this polls while anything is in one of those in-progress states and
 * stops as soon as nothing is pending, rather than polling forever.
 */
export function useRecommendations(): Recommendations {
  const [items, setItems] = useState<RecommendationDTO[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  const reload = useCallback(() => listRecommendations().then((res) => setItems(res.items)), [])

  useEffect(() => {
    let cancelled = false
    reload().catch((err) => {
      if (!cancelled) setError(String(err))
    })
    return () => {
      cancelled = true
    }
  }, [reload])

  const hasInFlightWriteBack = (items ?? []).some((item) => item.status === 'selected' || item.status === 'applying')
  useEffect(() => {
    if (!hasInFlightWriteBack) return
    const id = setInterval(() => {
      reload().catch(() => {
        // A transient poll failure isn't worth surfacing over the existing
        // view; the next tick (or a manual reload) will recover.
      })
    }, 4000)
    return () => clearInterval(id)
  }, [hasInFlightWriteBack, reload])

  return { items, error, reload }
}

export function rowKey(item: Pick<RecommendationDTO, 'namespace' | 'vpaName' | 'containerName'>): string {
  return `${item.namespace}/${item.vpaName}/${item.containerName}`
}

export function recommendationPath(item: Pick<RecommendationDTO, 'namespace' | 'vpaName' | 'containerName'>): string {
  return `/recommendations/${encodeURIComponent(item.namespace)}/${encodeURIComponent(item.vpaName)}/${encodeURIComponent(item.containerName)}`
}
