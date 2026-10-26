import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { Icon } from '../components/Icon'
import { LoadingScreen } from '../components/LoadingScreen'
import { PageTitle } from '../components/PageTitle'
import { Segmented } from '../components/Segmented'
import { StatusBadge } from '../components/StatusBadge'
import { ageSince, formatWhen, useNow } from '../lib/time'
import { shortRepo } from '../lib/format'
import { recommendationPath, rowKey, useRecommendations } from '../lib/useRecommendations'

type Filter = 'all' | 'applied' | 'failed' | 'in-flight'

/** A PR link or a commit page on the common Git hosts, when the repo URL allows one. */
function commitURL(repoURL: string | undefined, sha: string): string | null {
  if (!repoURL) return null
  const https = repoURL
    .replace(/^git@([^:]+):/, 'https://$1/')
    .replace(/^ssh:\/\/git@([^/]+)\//, 'https://$1/')
    .replace(/\.git$/, '')
  if (!https.startsWith('https://')) return null
  if (/github\.com|gitlab\.com/.test(https)) return `${https}/commit/${sha}`
  return null
}

/**
 * The latest write-back per container: what was committed, where, and
 * when -- or why it failed. The state store keeps only the most recent
 * attempt for each container, so this is not a full audit log; Git history
 * is.
 */
export function History() {
  const { items, error } = useRecommendations()
  const now = useNow()
  const [filter, setFilter] = useState<Filter>('all')

  const entries = useMemo(
    () =>
      (items ?? [])
        .filter((item) => item.operation || item.status === 'selected' || item.status === 'applying')
        .sort((a, b) => (b.operation?.updatedAt ?? '').localeCompare(a.operation?.updatedAt ?? '')),
    [items],
  )

  const counts = {
    all: entries.length,
    applied: entries.filter((e) => e.status === 'applied').length,
    failed: entries.filter((e) => e.status === 'failed').length,
    'in-flight': entries.filter((e) => e.status === 'selected' || e.status === 'applying').length,
  }
  const shown = entries.filter((e) =>
    filter === 'all' ? true : filter === 'in-flight' ? e.status === 'selected' || e.status === 'applying' : e.status === filter,
  )

  return (
    <div className="page">
      <header className="page-head">
        <PageTitle section="history" />
        <p className="page-sub">
          The latest write-back for each container, newest first. Git keeps the full history of every commit.
        </p>
      </header>

      {error && (
        <div className="callout callout-error">
          <Icon name="error" />
          <span>Failed to load write-backs: {error}</span>
        </div>
      )}
      {!error && items === null && <LoadingScreen title="Loading write-backs" />}

      {items && (
        <>
          <div className="filters">
            <Segmented
              label="Write-back status"
              value={filter}
              onChange={setFilter}
              options={[
                { value: 'all', label: 'All', count: counts.all },
                { value: 'applied', label: 'Applied', count: counts.applied },
                { value: 'failed', label: 'Failed', count: counts.failed },
                { value: 'in-flight', label: 'In flight', count: counts['in-flight'] },
              ]}
            />
          </div>

          {shown.length === 0 ? (
            <div className="empty">
              <h2>{entries.length === 0 ? 'No write-backs yet' : 'Nothing in this filter'}</h2>
              <p>
                {entries.length === 0
                  ? 'Select changes on Recommendations and apply them; each one shows up here once the worker commits it.'
                  : 'Pick another status above.'}
              </p>
            </div>
          ) : (
            <div className="table-wrap">
              <table className="data-table">
                <thead>
                  <tr>
                    <th>When</th>
                    <th>Status</th>
                    <th>VPA · namespace</th>
                    <th>Repository · branch</th>
                    <th>Commit</th>
                  </tr>
                </thead>
                <tbody>
                  {shown.map((item) => {
                    const op = item.operation
                    const url = op?.commitSha ? commitURL(item.binding?.repoURL, op.commitSha) : null
                    return (
                      <tr key={rowKey(item)}>
                        <td className="nowrap">
                          {op?.updatedAt ? (
                            <>
                              <span className="num">{formatWhen(op.updatedAt)}</span>
                              <span className="cell-sub">{ageSince(op.updatedAt, now)}</span>
                            </>
                          ) : (
                            <span className="cell-sub">waiting for the worker</span>
                          )}
                        </td>
                        <td>
                          <StatusBadge status={item.status} operation={op} />
                        </td>
                        <td>
                          <Link className="vpa-link" to={recommendationPath(item)} title={`${item.namespace}/${item.vpaName}`}>
                            {item.vpaName}
                          </Link>
                          <span className="vpa-meta">
                            {item.namespace} · {item.workload.kind} · {item.containerName}
                          </span>
                        </td>
                        <td>
                          <span className="repo" title={item.binding?.repoURL}>
                            {shortRepo(item.binding?.repoURL)}
                          </span>
                          <span className="cell-sub">{op?.branch ?? item.binding?.branch ?? ''}</span>
                        </td>
                        <td>
                          {op?.commitSha ? (
                            url ? (
                              <a href={url} target="_blank" rel="noreferrer" className="ext-link">
                                <code className="sha">{op.commitSha.slice(0, 7)}</code> <Icon name="external" />
                              </a>
                            ) : (
                              <code className="sha">{op.commitSha.slice(0, 7)}</code>
                            )
                          ) : op?.errorMessage ? (
                            <span className="text-bad error-text">{op.errorMessage}</span>
                          ) : (
                            '—'
                          )}
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}
    </div>
  )
}
