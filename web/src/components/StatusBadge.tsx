import type { OperationDTO } from '../api/types'

interface StatusBadgeProps {
  status: string
  operation?: OperationDTO
  /** Use the larger pill styling (RecommendationDetail's header). */
  pill?: boolean
}

/**
 * The recommendation's write-back status, plus -- once there's something to
 * show -- a short commit SHA (status "applied") or the failure reason as a
 * tooltip (status "failed", which covers both a real write-back failure and
 * a conflict, see internal/api's statusFor). Reused by the table, the card
 * view, and the detail page so all three stay in sync.
 */
export function StatusBadge({ status, operation, pill }: StatusBadgeProps) {
  const title = status === 'failed' ? operation?.errorMessage : undefined

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
    </span>
  )
}
