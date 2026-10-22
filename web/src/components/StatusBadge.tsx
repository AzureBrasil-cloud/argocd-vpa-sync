import type { ReactNode } from 'react'
import type { OperationDTO } from '../api/types'
import { ageSince, formatWhen, useNow } from '../lib/time'

const STATUS_LABELS: Record<string, string> = {
  new: 'New',
  selected: 'Selected',
  applying: 'Applying',
  applied: 'Applied',
  skipped: 'Skipped',
  failed: 'Failed',
}

/** What's under the status label when there's no commit or failure to show. */
const STATUS_NOTES: Record<string, string> = {
  new: 'not queued',
  selected: 'queued for write-back',
  applying: 'committing…',
  skipped: 'nothing to change',
}

interface StatusBadgeProps {
  status: string
  operation?: OperationDTO
  /** Larger form for the detail page header. */
  pill?: boolean
}

/**
 * The recommendation's write-back status, plus -- once there's something to
 * show -- a short commit SHA (status "applied") or the failure reason
 * (status "failed", which covers both a real write-back failure and a
 * conflict, see internal/api's statusFor), and how long ago: the VPA's
 * target keeps moving, so how recently a container was changed is what tells
 * whether another change is worth it yet. Each status has its own mark shape
 * (see index.css .mark-status-*), so it reads without colour.
 */
export function StatusBadge({ status, operation, pill }: StatusBadgeProps) {
  const now = useNow()
  const at = (status === 'applied' || status === 'failed') && operation?.updatedAt ? operation.updatedAt : null

  let note: ReactNode = STATUS_NOTES[status]
  if (status === 'applied' && operation?.commitSha) {
    note = (
      <>
        <code className="sha" title={operation.branch ? `Branch: ${operation.branch}` : undefined}>
          {operation.commitSha.slice(0, 7)}
        </code>
        {at && <span title={`${formatWhen(at)} · ${new Date(at).toLocaleString()}`}> · {ageSince(at, now)}</span>}
      </>
    )
  } else if (status === 'failed') {
    note = (
      <span title={operation?.errorMessage}>
        {operation?.errorMessage ? truncate(operation.errorMessage, 28) : 'write-back failed'}
        {at && ` · ${ageSince(at, now)}`}
      </span>
    )
  }

  return (
    <span className={`status-cell${pill ? ' status-cell-pill' : ''}`}>
      <span className={`status status-${status}`}>
        <span className={`mark mark-status-${status}`} aria-hidden="true" />
        {STATUS_LABELS[status] ?? status}
      </span>
      {note && <span className="status-note">{note}</span>}
    </span>
  )
}

function truncate(text: string, max: number): string {
  return text.length > max ? `${text.slice(0, max - 1)}…` : text
}
