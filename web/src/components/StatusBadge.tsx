import { useEffect, useState } from 'react'
import type { OperationDTO } from '../api/types'
import { formatAge } from '../lib/format'

/** The current time, refreshed every minute so relative ages stay current. */
function useNow(): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 60_000)
    return () => clearInterval(id)
  }, [])
  return now
}

interface StatusBadgeProps {
  status: string
  operation?: OperationDTO
  /** Use the larger pill styling (RecommendationDetail's header). */
  pill?: boolean
}

/** "24/09 15:46" in the viewer's locale and time zone. */
function formatWhen(iso: string): string {
  return new Date(iso).toLocaleString(undefined, { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
}

/**
 * The recommendation's write-back status, plus -- once there's something to
 * show -- a short commit SHA (status "applied") or the failure reason as a
 * tooltip (status "failed", which covers both a real write-back failure and
 * a conflict, see internal/api's statusFor), and when that happened: the
 * VPA's target keeps moving, so how recently a container was changed is
 * what tells whether another change is worth it yet. Reused by the table,
 * the card view, and the detail page so all three stay in sync.
 */
export function StatusBadge({ status, operation, pill }: StatusBadgeProps) {
  const now = useNow()
  const title = status === 'failed' ? operation?.errorMessage : undefined
  const at = (status === 'applied' || status === 'failed') && operation?.updatedAt ? operation.updatedAt : null
  const ageSeconds = at ? Math.max(0, Math.floor((now - new Date(at).getTime()) / 1000)) : 0

  return (
    <span className="status-cell">
      <span className={`status${pill ? ' status-pill' : ''} status-${status}`} title={title}>
        {status}
      </span>
      {status === 'applied' && operation?.commitSha && (
        <span
          className="status-commit"
          title={operation.branch ? `Branch: ${operation.branch}` : undefined}
        >
          {operation.commitSha.slice(0, 7)}
        </span>
      )}
      {at && (
        <span className="status-when" title={new Date(at).toLocaleString()}>
          {formatWhen(at)} · {formatAge(ageSeconds)} ago
        </span>
      )}
    </span>
  )
}
