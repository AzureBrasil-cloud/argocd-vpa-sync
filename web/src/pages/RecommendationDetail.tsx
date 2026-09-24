import { useCallback, useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { getRecommendation, selectRecommendation } from '../api/client'
import type { RecommendationDTO } from '../api/types'
import { EligibilityBadge } from '../components/EligibilityBadge'
import { LimitSettingsInput, type LimitResourceCounts } from '../components/LimitSettingsInput'
import { UNMANAGED_EXCEEDED_TOOLTIP } from '../components/SelectionSummary'
import { ResourceCheckbox } from '../components/ResourceCheckbox'
import { StatusBadge } from '../components/StatusBadge'
import { formatCPU, formatMemory } from '../lib/format'
import {
  decideLimit,
  LIMIT_KEY_PATH,
  limitChange,
  limitExceededUnmanaged,
  type LimitChange,
  type LimitResource,
} from '../lib/limits'
import { useLimitSettings } from '../lib/useLimitSettings'

function newLimitCell(change: LimitChange | null) {
  if (!change) return '—'
  if (change.unmanagedExceeded) {
    return (
      <span className="limit-decreases" title={UNMANAGED_EXCEEDED_TOOLTIP}>
        {change.reason} ⚠
      </span>
    )
  }
  if (change.next === null) return <span className="muted">{change.reason ?? '—'}</span>
  if (change.belowRequest) {
    return (
      <span className="limit-below-request" title="Kubernetes rejects a request greater than its limit">
        {change.next} &lt; request ✕
      </span>
    )
  }
  return (
    <span className={change.decreases ? 'limit-decreases' : undefined} title={change.decreases ? 'The limit will be lowered' : undefined}>
      {change.next}
      {change.decreases && ' ⚠'}
    </span>
  )
}

export function RecommendationDetail() {
  const { namespace = '', vpaName = '', containerName = '' } = useParams()
  const [item, setItem] = useState<RecommendationDTO | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [selectCPU, setSelectCPU] = useState(false)
  const [selectMemory, setSelectMemory] = useState(false)
  const [selecting, setSelecting] = useState(false)
  const [selectError, setSelectError] = useState<string | null>(null)
  const limits = useLimitSettings()

  const load = useCallback(() => {
    return getRecommendation(namespace, vpaName, containerName).then(setItem)
  }, [namespace, vpaName, containerName])

  useEffect(() => {
    let cancelled = false
    load().catch((err) => {
      if (!cancelled) setError(String(err))
    })
    return () => {
      cancelled = true
    }
  }, [load])

  // See RecommendationList's identical polling: the write-back worker picks
  // this up asynchronously, so keep refreshing while its status is still
  // in-progress.
  const hasInFlightWriteBack = item?.status === 'selected' || item?.status === 'applying'
  useEffect(() => {
    if (!hasInFlightWriteBack) return
    const id = setInterval(() => {
      load().catch(() => {})
    }, 4000)
    return () => clearInterval(id)
  }, [hasInFlightWriteBack, load])

  async function handleAccept() {
    if ((!selectCPU && !selectMemory) || limitBlocked) return
    setSelecting(true)
    setSelectError(null)
    try {
      await selectRecommendation(namespace, vpaName, containerName, {
        applyCPU: selectCPU,
        applyMemory: selectMemory,
        cpuLimit: selectCPU ? cpuDecision?.parsed?.spec : undefined,
        memoryLimit: selectMemory ? memoryDecision?.parsed?.spec : undefined,
      })
      setSelectCPU(false)
      setSelectMemory(false)
      await load()
    } catch (err) {
      setSelectError(String(err))
    } finally {
      setSelecting(false)
    }
  }

  const cpuDecision = item ? decideLimit(item, 'cpu', limits) : null
  const memoryDecision = item ? decideLimit(item, 'memory', limits) : null
  const cpuChange = item && cpuDecision && !cpuDecision.invalid ? limitChange(item, 'cpu', cpuDecision.parsed) : null
  const memoryChange =
    item && memoryDecision && !memoryDecision.invalid ? limitChange(item, 'memory', memoryDecision.parsed) : null
  // Settings are shown for both resources so the preview columns are always
  // filled in, but only a resource actually ticked must have a valid one.
  const limitResources: LimitResourceCounts[] = [cpuDecision, memoryDecision].flatMap((d, i) =>
    d
      ? [
          {
            resource: (['cpu', 'memory'] as LimitResource[])[i],
            required: d.requirement === 'required' ? 1 : 0,
            optional: d.requirement === 'optional' ? 1 : 0,
          },
        ]
      : [],
  )
  const unmanagedExceeded = item
    ? (['cpu', 'memory'] as LimitResource[]).filter((r) => limitExceededUnmanaged(item, r))
    : []
  const limitBlocked =
    (selectCPU && (cpuDecision?.invalid === true || cpuChange?.belowRequest === true)) ||
    (selectMemory && (memoryDecision?.invalid === true || memoryChange?.belowRequest === true))

  return (
    <div>
      <p>
        <Link to="/">&larr; Back to list</Link>
      </p>

      {error && <div className="panel panel-error">Failed to load: {error}</div>}
      {!error && !item && <div className="panel panel-loading">Loading…</div>}

      {item && (
        <div className="panel detail">
          <div className="detail-header">
            <div>
              <h2>
                {item.vpaName} <span className="muted">/ {item.containerName}</span>
              </h2>
              <p className="muted">
                {item.namespace} &middot; {item.workload.kind}/{item.workload.name} &middot; VPA mode {item.updateMode}
              </p>
            </div>
            <StatusBadge status={item.status} operation={item.operation} pill />
          </div>

          {item.status === 'failed' && item.operation?.errorMessage && (
            <div className="panel panel-error">
              <strong>Write-back failed:</strong> {item.operation.errorMessage}
            </div>
          )}

          {item.status === 'applied' && item.operation?.commitSha && (
            <p className="muted">
              Applied to <code>{item.operation.branch}</code> as <code>{item.operation.commitSha.slice(0, 7)}</code>
            </p>
          )}

          {item.validationErrors && item.validationErrors.length > 0 && (
            <div className="panel panel-error">
              <strong>Invalid configuration:</strong>
              <ul>
                {item.validationErrors.map((e) => (
                  <li key={e}>{e}</li>
                ))}
              </ul>
            </div>
          )}

          {item.warnings && item.warnings.length > 0 && (
            <div className="panel panel-warning">
              <strong>Warnings:</strong>
              <ul>
                {item.warnings.map((w) => (
                  <li key={w}>{w}</li>
                ))}
              </ul>
            </div>
          )}

          {unmanagedExceeded.length > 0 && (
            <div className="panel panel-warning">
              <strong>Limit below the new request:</strong>
              <ul>
                {unmanagedExceeded.map((r) => (
                  <li key={r}>
                    {r === 'cpu'
                      ? `recommended cpu ${formatCPU(item.recommendedCpu)} exceeds the current limit ${formatCPU(item.currentCpuLimit)}`
                      : `recommended memory ${formatMemory(item.recommendedMemory)} exceeds the current limit ${formatMemory(item.currentMemoryLimit)}`}
                    , but the VpaGitOpsBinding sets no <code>{LIMIT_KEY_PATH[r]}</code> -- write-back can't raise
                    the limit, and Kubernetes will reject the new request. Set <code>{LIMIT_KEY_PATH[r]}</code> on the
                    binding, or raise the limit in Git by hand.
                  </li>
                ))}
              </ul>
            </div>
          )}

          {item.currentValueError && (
            <div className="panel panel-error">
              <strong>Could not read the current value from the live workload:</strong> {item.currentValueError}
            </div>
          )}

          <table className="compare-table">
            <thead>
              <tr>
                <th>Select</th>
                <th></th>
                <th>Current (live)</th>
                <th>Recommended (VPA)</th>
                <th>Lower bound</th>
                <th>Upper bound</th>
                <th>Limit (live)</th>
                <th>New limit</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>
                  <ResourceCheckbox
                    label="Select CPU"
                    configured={item.cpuConfigured}
                    eligible={item.cpuEligible}
                    reasons={item.cpuEligibilityReasons}
                    checked={selectCPU}
                    onChange={setSelectCPU}
                  />
                </td>
                <td>CPU</td>
                <td>{formatCPU(item.currentCpu) || '—'}</td>
                <td>{formatCPU(item.recommendedCpu) || '—'}</td>
                <td>{formatCPU(item.lowerBoundCpu) || '—'}</td>
                <td>{formatCPU(item.upperBoundCpu) || '—'}</td>
                <td>{formatCPU(item.currentCpuLimit) || '—'}</td>
                <td>{newLimitCell(cpuChange)}</td>
              </tr>
              <tr>
                <td>
                  <ResourceCheckbox
                    label="Select memory"
                    configured={item.memoryConfigured}
                    eligible={item.memoryEligible}
                    reasons={item.memoryEligibilityReasons}
                    checked={selectMemory}
                    onChange={setSelectMemory}
                  />
                </td>
                <td>Memory</td>
                <td>{formatMemory(item.currentMemory) || '—'}</td>
                <td>{formatMemory(item.recommendedMemory) || '—'}</td>
                <td>{formatMemory(item.lowerBoundMemory) || '—'}</td>
                <td>{formatMemory(item.upperBoundMemory) || '—'}</td>
                <td>{formatMemory(item.currentMemoryLimit) || '—'}</td>
                <td>{newLimitCell(memoryChange)}</td>
              </tr>
            </tbody>
          </table>

          <div className="detail-actions">
            <EligibilityBadge eligible={item.eligible} reasons={item.eligibilityReasons} />

            {item.status === 'selected' && (
              <span className="badge badge-selected">✓ Queued for write-back</span>
            )}

            <LimitSettingsInput settings={limits} resources={limitResources} disabled={selecting} />

            <button
              className="btn btn-primary"
              onClick={handleAccept}
              disabled={selecting || (!selectCPU && !selectMemory) || limitBlocked}
            >
              {selecting ? 'Queuing…' : 'Accept selected'}
            </button>
          </div>

          {selectError && <div className="panel panel-error">Failed to queue selection: {selectError}</div>}

          <p className="muted note">
            Tick CPU and/or memory above, then Accept to queue them for this project to write back
            to Git later -- nothing is modified in the VPA, the Deployment, or the cluster directly.
            When the new request exceeds the current limit, a new limit is required and written with
            it; otherwise the limit is left untouched unless you tick "update". Either way it's set by
            headroom (the new request becomes (100 − headroom)% of the limit) or to an absolute value;
            a limit the manifest doesn't declare is never added.
          </p>
        </div>
      )}
    </div>
  )
}
