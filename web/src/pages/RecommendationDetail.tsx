import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { getRecommendation, listRecommendations, selectRecommendation } from '../api/client'
import type { RecommendationDTO } from '../api/types'
import { EligibilityBadge } from '../components/EligibilityBadge'
import { LoadingScreen } from '../components/LoadingScreen'
import { Hint } from '../components/Hint'
import { Icon } from '../components/Icon'
import { ResourceReading } from '../components/ResourceReading'
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
import { bindingPath, recommendationPath, rowKey } from '../lib/useRecommendations'

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

/**
 * A VPA bound, plus -- when a recorded request headroom scales it -- the
 * bound the live request is actually judged against.
 */
function boundCell(item: RecommendationDTO, resource: LimitResource, which: 'lower' | 'upper') {
  const format = resource === 'cpu' ? formatCPU : formatMemory
  const raw =
    resource === 'cpu'
      ? which === 'lower' ? item.lowerBoundCpu : item.upperBoundCpu
      : which === 'lower' ? item.lowerBoundMemory : item.upperBoundMemory
  const band =
    resource === 'cpu'
      ? which === 'lower' ? item.bandLowerCpu : item.bandUpperCpu
      : which === 'lower' ? item.bandLowerMemory : item.bandUpperMemory
  const saved = resource === 'cpu' ? item.cpuRequestHeadroomPercent : item.memoryRequestHeadroomPercent
  if (!saved || !band) return format(raw) || '—'
  return (
    <>
      {format(raw) || '—'}
      <div className="muted">
        {format(band)} (+{saved}%)
      </div>
    </>
  )
}

const WITHIN_BAND_HINT =
  "The VPA recommends a range (lower..upper bound), not just a target, and its target keeps moving. While the " +
  "current request sits inside that range (scaled by the headroom last applied), the workload is sized well " +
  "enough: changing it again would only chase noise. It is the same rule the VPA's own updater uses before " +
  'restarting a pod. It becomes eligible again once the request falls outside the range.'

function withinBandNote(reasons?: string[]) {
  if (!reasons?.includes('within-vpa-bounds')) return null
  return (
    <Hint
      className="within-band"
      trigger={
        <span className="badge badge-ok">
          <Icon name="check" /> within VPA range
        </span>
      }
    >
      {WITHIN_BAND_HINT}
    </Hint>
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
        <Icon name="warning" /> {change.reason}
      </span>
    )
  }
  if (change.next === null) return <span className="muted">{change.reason ?? '—'}</span>
  if (change.belowRequest) {
    return (
      <span className="limit-below-request" title="Kubernetes rejects a request greater than its limit">
        <Icon name="error" /> {change.next} &lt; request
      </span>
    )
  }
  return (
    <span className={change.decreases ? 'limit-decreases' : undefined} title={change.decreases ? 'The limit will be lowered' : undefined}>
      {change.decreases && <Icon name="warning" />} {change.next}
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

  // j / k step to the next / previous eligible container (in the list's
  // namespace/VPA order), so a review can walk the eligible set without
  // going back to the table each time.
  const navigate = useNavigate()
  const [eligibleKeys, setEligibleKeys] = useState<RecommendationDTO[]>([])
  useEffect(() => {
    listRecommendations()
      .then((res) => setEligibleKeys(res.items.filter((i) => i.eligible).sort((a, b) => rowKey(a).localeCompare(rowKey(b)))))
      .catch(() => setEligibleKeys([]))
  }, [])
  const currentKey = `${namespace}/${vpaName}/${containerName}`
  const neighbours = (() => {
    if (eligibleKeys.length === 0) return { prev: undefined, next: undefined }
    const after = eligibleKeys.find((i) => rowKey(i) > currentKey)
    const before = [...eligibleKeys].reverse().find((i) => rowKey(i) < currentKey)
    return { prev: before, next: after }
  })()
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.metaKey || e.ctrlKey || e.altKey) return
      const target = e.target as HTMLElement
      if (target.closest('input, select, textarea, [contenteditable]')) return
      const to = e.key === 'j' ? neighbours.next : e.key === 'k' ? neighbours.prev : undefined
      if (to) navigate(recommendationPath(to))
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [neighbours.next, neighbours.prev, navigate])

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
    <div className="page detail">
      <nav className="detail-nav" aria-label="Recommendation navigation">
        <Link to="/" className="back-link">
          <Icon name="arrow-left" /> Recommendations
        </Link>
        {(neighbours.prev || neighbours.next) && (
          <span className="stepper">
            {neighbours.prev ? (
              <Link to={recommendationPath(neighbours.prev)} className="btn btn-small">
                <kbd>k</kbd> Previous eligible
              </Link>
            ) : null}
            {neighbours.next ? (
              <Link to={recommendationPath(neighbours.next)} className="btn btn-small">
                Next eligible <kbd>j</kbd>
              </Link>
            ) : null}
          </span>
        )}
      </nav>

      {error && (
        <div className="callout callout-error">
          <Icon name="error" />
          <span>Failed to load: {error}</span>
        </div>
      )}
      {!error && !item && <LoadingScreen title="Loading recommendation" />}

      {item && (
        <>
          <header className="detail-head">
            <div>
              <h1 className="page-title">
                <span className="vpa-ns">{item.namespace} /</span> {item.vpaName}
                <span className="detail-container"> · {item.containerName}</span>
              </h1>
              <p className="page-sub">
                {item.workload.kind}/{item.workload.name} · VPA mode {item.updateMode}
              </p>
            </div>
            <StatusBadge status={item.status} operation={item.operation} pill />
          </header>

          <div className="detail-readings">
            {RESOURCES.map((r) => (
              <section key={r} className="detail-reading" aria-label={r === 'cpu' ? 'CPU' : 'Memory'}>
                <h2 className="detail-reading-title">{r === 'cpu' ? 'CPU request' : 'Memory request'}</h2>
                <ResourceReading item={item} resource={r} size="detail" />
              </section>
            ))}
          </div>

          {item.status === 'failed' && item.operation?.errorMessage && (
            <div className="callout callout-error">
              <Icon name="error" />
              <span>
                <strong>Write-back failed:</strong> {item.operation.errorMessage}
              </span>
              <Link className="btn btn-small callout-action" to={bindingPath(item)}>
                View binding
              </Link>
            </div>
          )}

          {item.status === 'applied' && item.operation?.commitSha && (
            <p className="page-sub">
              Applied to <code>{item.operation.branch}</code> as <code className="sha">{item.operation.commitSha.slice(0, 7)}</code>
            </p>
          )}

          {item.validationErrors && item.validationErrors.length > 0 && (
            <div className="callout callout-error">
              <Icon name="error" />
              <div>
              <strong>Invalid configuration:</strong>
              <ul>
                {item.validationErrors.map((e) => (
                  <li key={e}>{e}</li>
                ))}
              </ul>
              </div>
              <Link className="btn btn-small callout-action" to={bindingPath(item)}>
                View binding
              </Link>
            </div>
          )}

          {item.warnings && item.warnings.length > 0 && (
            <div className="callout callout-warning">
              <Icon name="warning" />
              <div>
              <strong>Warnings:</strong>
              <ul>
                {item.warnings.map((w) => (
                  <li key={w}>{w}</li>
                ))}
              </ul>
              </div>
            </div>
          )}

          {unmanagedExceeded.length > 0 && (
            <div className="callout callout-warning">
              <Icon name="warning" />
              <div>
              <strong>Limit below the new request:</strong>
              <ul>
                {unmanagedExceeded.map((r) => (
                  <li key={r}>
                    {`new ${r} request ${FORMAT[r](plans![r].request!)} exceeds the current limit ${
                      r === 'cpu' ? formatCPU(item.currentCpuLimit) : formatMemory(item.currentMemoryLimit)
                    }`}
                    , but the VpaGitOpsBinding sets no <code>{LIMIT_KEY_PATH[r]}</code>, so write-back can't raise
                    the limit, and Kubernetes will reject the new request. Set <code>{LIMIT_KEY_PATH[r]}</code> on the
                    binding, or raise the limit in Git by hand.
                  </li>
                ))}
              </ul>
              </div>
            </div>
          )}

          {item.currentValueError && (
            <div className="callout callout-error">
              <Icon name="error" />
              <span>
                <strong>Could not read the current value from the live workload:</strong> {item.currentValueError}
              </span>
            </div>
          )}

          <div className="table-wrap">
          <table className="data-table compare-table">
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
                <td>
                  {formatCPU(item.currentCpu) || '—'} {withinBandNote(item.cpuEligibilityReasons)}
                </td>
                <td>{recommendedCell(item, 'cpu')}</td>
                <td>{newRequestCell(plans!.cpu, 'cpu')}</td>
                <td>{boundCell(item, 'cpu', 'lower')}</td>
                <td>{boundCell(item, 'cpu', 'upper')}</td>
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
                <td>
                  {formatMemory(item.currentMemory) || '—'} {withinBandNote(item.memoryEligibilityReasons)}
                </td>
                <td>{recommendedCell(item, 'memory')}</td>
                <td>{newRequestCell(plans!.memory, 'memory')}</td>
                <td>{boundCell(item, 'memory', 'lower')}</td>
                <td>{boundCell(item, 'memory', 'upper')}</td>
                <td>{formatMemory(item.currentMemoryLimit) || '—'}</td>
                <td>{newLimitCell(plans!.memory.limitChange)}</td>
              </tr>
            </tbody>
          </table>
          </div>

          {item.binding && (
            <section className="detail-binding" aria-labelledby="detail-binding-title">
              <h2 id="detail-binding-title">Where write-back commits</h2>
              <dl className="facts">
                <div>
                  <dt>Repository</dt>
                  <dd className="mono-path">{item.binding.repoURL ?? '—'}</dd>
                </div>
                <div>
                  <dt>Branch</dt>
                  <dd>{item.binding.branch ?? '—'}</dd>
                </div>
                <div>
                  <dt>Manifest</dt>
                  <dd className="mono-path">
                    {item.binding.manifestPath ?? '—'}
                    {item.binding.manifestType && <span className="cell-sub"> ({item.binding.manifestType})</span>}
                  </dd>
                </div>
                <div>
                  <dt>Request keys</dt>
                  <dd className="mono-path">
                    {item.binding.cpuKeyPath ?? 'cpu not set'} · {item.binding.memoryKeyPath ?? 'memory not set'}
                  </dd>
                </div>
                <div>
                  <dt>Limit keys</dt>
                  <dd className="mono-path">
                    {item.binding.cpuLimitKeyPath ?? 'cpu not set'} · {item.binding.memoryLimitKeyPath ?? 'memory not set'}
                  </dd>
                </div>
                <div>
                  <dt>Min. change</dt>
                  <dd>{item.binding.minChangePercent}%</dd>
                </div>
              </dl>
            </section>
          )}

          <div className="detail-actions">
            <EligibilityBadge eligible={item.eligible} reasons={item.eligibilityReasons} />

            {item.status === 'selected' && (
              <span className="badge badge-selected">
                <Icon name="check" /> Queued for write-back
              </span>
            )}

            <RequestHeadroomInput settings={headroom} resources={RESOURCES} disabled={selecting} />
            <LimitSettingsInput settings={limits} resources={limitResources} disabled={selecting} />

            <button
              className="btn btn-primary"
              onClick={handleAccept}
              disabled={selecting || (!selectCPU && !selectMemory) || limitBlocked}
            >
              {selecting ? 'Queuing…' : 'Queue selected for write-back'}
            </button>
          </div>

          {selectError && (
            <div className="callout callout-error">
              <Icon name="error" />
              <span>Failed to queue selection: {selectError}</span>
            </div>
          )}

          <p className="note">
            Tick CPU and/or memory above, then queue them for this project to write back
            to Git later. Nothing is modified in the VPA, the Deployment, or the cluster directly.
            The request is written with the request headroom above the recommendation (the
            recommendation becomes (100 − headroom)% of it); once applied, that headroom is remembered
            for this container and later recommendations are compared against recommendation + headroom.
            When the new request exceeds the current limit, a new limit is required and written with
            it; otherwise the limit is left untouched unless you tick "update". Either way it's set by
            headroom (the new request becomes (100 − headroom)% of the limit) or to an absolute value;
            a limit the manifest doesn't declare is never added.
          </p>
        </>
      )}
    </div>
  )
}
