import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { getRecommendation, selectRecommendation } from '../api/client'
import type { RecommendationDTO } from '../api/types'
import { EligibilityBadge } from '../components/EligibilityBadge'
import { LimitSettingsInput, type LimitResourceCounts } from '../components/LimitSettingsInput'
import { RequestHeadroomInput } from '../components/RequestHeadroomInput'
import { UNMANAGED_EXCEEDED_TOOLTIP } from '../components/SelectionSummary'
import { ResourceCheckbox } from '../components/ResourceCheckbox'
import { StatusBadge } from '../components/StatusBadge'
import { formatCPU, formatMemory } from '../lib/format'
import { FORMAT, LIMIT_KEY_PATH, type LimitChange, type LimitResource } from '../lib/limits'
import { planResource, planSelection, type ResourcePlan } from '../lib/selectionPlan'
import { useLimitSettings } from '../lib/useLimitSettings'
import { useRequestHeadroom } from '../lib/useRequestHeadroom'

const RESOURCES: LimitResource[] = ['cpu', 'memory']

/** The recommendation, plus the target it implies when a request headroom was recorded. */
function recommendedCell(item: RecommendationDTO, resource: LimitResource) {
  const format = resource === 'cpu' ? formatCPU : formatMemory
  const recommended = format(resource === 'cpu' ? item.recommendedCpu : item.recommendedMemory) || '—'
  const saved = resource === 'cpu' ? item.cpuRequestHeadroomPercent : item.memoryRequestHeadroomPercent
  const target = resource === 'cpu' ? item.targetCpu : item.targetMemory
  if (!saved || !target) return recommended
  return (
    <>
      {recommended}
      <div className="muted" title="The live request is compared against the recommendation plus the headroom last applied">
        target {format(target)} (+{saved}% saved)
      </div>
    </>
  )
}

function newRequestCell(plan: ResourcePlan, resource: LimitResource) {
  if (plan.request === null) return '—'
  return (
    <>
      {FORMAT[resource](plan.request)}
      {plan.headroom ? <span className="muted"> (+{plan.headroom}%)</span> : null}
    </>
  )
}

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
  const headroom = useRequestHeadroom()

  // Start the request headroom from the one this container was last applied
  // with, once per container (not on every poll, which would overwrite what
  // the user is typing).
  const seededFor = useRef<string | null>(null)
  const { seed } = headroom
  useEffect(() => {
    if (!item) return
    const key = `${item.namespace}/${item.vpaName}/${item.containerName}`
    if (seededFor.current === key) return
    seededFor.current = key
    seed({ cpu: item.cpuRequestHeadroomPercent, memory: item.memoryRequestHeadroomPercent })
  }, [item, seed])

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
        ...selection!.body,
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

  // Both resources are planned so the preview columns are always filled in,
  // but only a resource actually ticked must have valid settings.
  const plans = item ? { cpu: planResource(item, 'cpu', limits, headroom), memory: planResource(item, 'memory', limits, headroom) } : null
  const selection = item ? planSelection(item, { cpu: selectCPU, memory: selectMemory }, limits, headroom) : null
  const limitResources: LimitResourceCounts[] = plans
    ? RESOURCES.map((resource) => ({
        resource,
        required: plans[resource].limit.requirement === 'required' ? 1 : 0,
        optional: plans[resource].limit.requirement === 'optional' ? 1 : 0,
      }))
    : []
  const unmanagedExceeded = plans ? RESOURCES.filter((r) => plans[r].limitChange?.unmanagedExceeded) : []
  const limitBlocked =
    selection === null ||
    selection.invalid ||
    RESOURCES.some((r) => selection[r]?.limitChange?.belowRequest === true)

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
                    {`new ${r} request ${FORMAT[r](plans![r].request!)} exceeds the current limit ${
                      r === 'cpu' ? formatCPU(item.currentCpuLimit) : formatMemory(item.currentMemoryLimit)
                    }`}
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
                <th>New request</th>
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
                <td>{recommendedCell(item, 'cpu')}</td>
                <td>{newRequestCell(plans!.cpu, 'cpu')}</td>
                <td>{formatCPU(item.lowerBoundCpu) || '—'}</td>
                <td>{formatCPU(item.upperBoundCpu) || '—'}</td>
                <td>{formatCPU(item.currentCpuLimit) || '—'}</td>
                <td>{newLimitCell(plans!.cpu.limitChange)}</td>
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
                <td>{recommendedCell(item, 'memory')}</td>
                <td>{newRequestCell(plans!.memory, 'memory')}</td>
                <td>{formatMemory(item.lowerBoundMemory) || '—'}</td>
                <td>{formatMemory(item.upperBoundMemory) || '—'}</td>
                <td>{formatMemory(item.currentMemoryLimit) || '—'}</td>
                <td>{newLimitCell(plans!.memory.limitChange)}</td>
              </tr>
            </tbody>
          </table>

          <div className="detail-actions">
            <EligibilityBadge eligible={item.eligible} reasons={item.eligibilityReasons} />

            {item.status === 'selected' && (
              <span className="badge badge-selected">✓ Queued for write-back</span>
            )}

            <RequestHeadroomInput settings={headroom} resources={RESOURCES} disabled={selecting} />
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
            The request is written with the request headroom above the recommendation (the
            recommendation becomes (100 − headroom)% of it); once applied, that headroom is remembered
            for this container and later recommendations are compared against recommendation + headroom.
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
