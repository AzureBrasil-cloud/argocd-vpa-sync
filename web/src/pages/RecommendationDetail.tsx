import { useCallback, useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { getRecommendation, selectRecommendation } from '../api/client'
import type { RecommendationDTO } from '../api/types'
import { EligibilityBadge } from '../components/EligibilityBadge'
import { ResourceCheckbox } from '../components/ResourceCheckbox'
import { StatusBadge } from '../components/StatusBadge'
import { formatCPU, formatMemory } from '../lib/format'

export function RecommendationDetail() {
  const { namespace = '', vpaName = '', containerName = '' } = useParams()
  const [item, setItem] = useState<RecommendationDTO | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [selectCPU, setSelectCPU] = useState(false)
  const [selectMemory, setSelectMemory] = useState(false)
  const [selecting, setSelecting] = useState(false)
  const [selectError, setSelectError] = useState<string | null>(null)

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
    if (!selectCPU && !selectMemory) return
    setSelecting(true)
    setSelectError(null)
    try {
      await selectRecommendation(namespace, vpaName, containerName, { applyCPU: selectCPU, applyMemory: selectMemory })
      setSelectCPU(false)
      setSelectMemory(false)
      await load()
    } catch (err) {
      setSelectError(String(err))
    } finally {
      setSelecting(false)
    }
  }

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
              </tr>
            </tbody>
          </table>

          <div className="detail-actions">
            <EligibilityBadge eligible={item.eligible} reasons={item.eligibilityReasons} />

            {item.status === 'selected' && (
              <span className="badge badge-selected">✓ Queued for write-back</span>
            )}

            <button
              className="btn btn-primary"
              onClick={handleAccept}
              disabled={selecting || (!selectCPU && !selectMemory)}
            >
              {selecting ? 'Queuing…' : 'Accept selected'}
            </button>
          </div>

          {selectError && <div className="panel panel-error">Failed to queue selection: {selectError}</div>}

          <p className="muted note">
            Tick CPU and/or memory above, then Accept to queue them for this project to write back
            to Git later -- nothing is modified in the VPA, the Deployment, or the cluster directly.
          </p>
        </div>
      )}
    </div>
  )
}
