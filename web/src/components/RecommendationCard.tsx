import { Link } from 'react-router-dom'
import type { RecommendationDTO } from '../api/types'
import { formatAge } from '../lib/format'
import { DeltaBadge } from './DeltaBadge'
import { EligibilityBadge } from './EligibilityBadge'
import { ResourceCheckbox } from './ResourceCheckbox'
import { StatusBadge } from './StatusBadge'

interface RecommendationCardProps {
  item: RecommendationDTO
  cpuChecked: boolean
  memoryChecked: boolean
  busy: boolean
  error?: string
  onToggle: (resource: 'cpu' | 'memory', checked: boolean) => void
  onAccept: () => void
}

/**
 * A single recommendation shown as a self-contained card instead of a table
 * row -- same data and actions as the table, laid out for scanning one
 * VPA/container at a time rather than comparing columns across many rows.
 */
export function RecommendationCard({
  item,
  cpuChecked,
  memoryChecked,
  busy,
  error,
  onToggle,
  onAccept,
}: RecommendationCardProps) {
  const canAccept = (cpuChecked || memoryChecked) && !busy

  return (
    <div className="recommendation-card">
      <div className="recommendation-card-header">
        <div>
          <Link to={`/recommendations/${item.namespace}/${item.vpaName}/${item.containerName}`}>{item.vpaName}</Link>
          <span className="muted"> / {item.containerName}</span>
          <p className="muted recommendation-card-subtitle">
            {item.namespace} &middot; {item.workload.kind}/{item.workload.name} &middot; {item.updateMode}
          </p>
        </div>
        <StatusBadge status={item.status} operation={item.operation} />
      </div>

      <div className="recommendation-card-row">
        <ResourceCheckbox
          label={`Select CPU for ${item.containerName}`}
          configured={item.cpuConfigured}
          eligible={item.cpuEligible}
          reasons={item.cpuEligibilityReasons}
          checked={cpuChecked}
          onChange={(checked) => onToggle('cpu', checked)}
        />
        <span className="recommendation-card-row-label">CPU</span>
        <DeltaBadge kind="cpu" current={item.currentCpu} recommended={item.recommendedCpu} percent={item.deltaCpuPercent} />
      </div>

      <div className="recommendation-card-row">
        <ResourceCheckbox
          label={`Select memory for ${item.containerName}`}
          configured={item.memoryConfigured}
          eligible={item.memoryEligible}
          reasons={item.memoryEligibilityReasons}
          checked={memoryChecked}
          onChange={(checked) => onToggle('memory', checked)}
        />
        <span className="recommendation-card-row-label">Memory</span>
        <DeltaBadge
          kind="memory"
          current={item.currentMemory}
          recommended={item.recommendedMemory}
          percent={item.deltaMemoryPercent}
        />
      </div>

      <div className="recommendation-card-footer">
        {item.currentValueError ? (
          <span className="badge badge-ineligible" title={item.currentValueError}>
            Error
          </span>
        ) : (
          <EligibilityBadge eligible={item.eligible} reasons={item.eligibilityReasons} />
        )}
        <span className="muted recommendation-card-age">{formatAge(item.recommendationAgeSeconds)}</span>
      </div>

      <div className="recommendation-card-actions">
        <button className="btn btn-primary btn-small" disabled={!canAccept} onClick={onAccept}>
          {busy ? '…' : 'Accept'}
        </button>
        {error && (
          <div className="cell-error" title={error}>
            Failed
          </div>
        )}
      </div>
    </div>
  )
}
