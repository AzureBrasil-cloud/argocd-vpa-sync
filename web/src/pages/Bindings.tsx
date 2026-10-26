import { useEffect, useMemo } from 'react'
import { Link, useLocation } from 'react-router-dom'
import type { RecommendationDTO } from '../api/types'
import { Icon } from '../components/Icon'
import { LoadingScreen } from '../components/LoadingScreen'
import { PageTitle } from '../components/PageTitle'
import { bindingAnchor, recommendationPath, rowKey, useRecommendations } from '../lib/useRecommendations'

interface BindingGroup {
  namespace: string
  vpaName: string
  first: RecommendationDTO
  containers: RecommendationDTO[]
}

function KeyPath({ path }: { path?: string }) {
  return path ? <code className="mono-path">{path}</code> : <span className="cell-sub">not set</span>
}

/**
 * Every VpaGitOpsBinding in the cluster, read from the containers it opts
 * in: which Argo CD Application and repo it writes to, and for each
 * container the manifest and key paths write-back patches. Read-only --
 * bindings are managed as CRDs in Git like everything else.
 */
export function Bindings() {
  const { items, error } = useRecommendations()

  const groups = useMemo(() => {
    const byVPA = new Map<string, BindingGroup>()
    for (const item of items ?? []) {
      const key = `${item.namespace}/${item.vpaName}`
      const group = byVPA.get(key) ?? { namespace: item.namespace, vpaName: item.vpaName, first: item, containers: [] }
      group.containers.push(item)
      byVPA.set(key, group)
    }
    return [...byVPA.values()].sort((a, b) => `${a.namespace}/${a.vpaName}`.localeCompare(`${b.namespace}/${b.vpaName}`))
  }, [items])

  // Linked from a recommendation (bindingPath): the binding to scroll to,
  // once the list has rendered it.
  const { hash } = useLocation()
  const target = hash ? decodeURIComponent(hash.slice(1)) : ''
  useEffect(() => {
    if (target && groups.length > 0) document.getElementById(target)?.scrollIntoView({ block: 'start' })
  }, [target, groups.length])

  return (
    <div className="page">
      <header className="page-head">
        <PageTitle section="bindings" />
        <p className="page-sub">
          Each <code>VpaGitOpsBinding</code> opts a VPA in to write-back and says where in Git its values live. Edit them as CRDs
          in Git; this page is read-only.
        </p>
      </header>

      {error && (
        <div className="callout callout-error">
          <Icon name="error" />
          <span>Failed to load bindings: {error}</span>
        </div>
      )}
      {!error && items === null && <LoadingScreen title="Loading bindings" />}
      {items && groups.length === 0 && (
        <div className="empty">
          <h2>No bindings yet</h2>
          <p>
            Create a <code>VpaGitOpsBinding</code> in the same namespace as a VerticalPodAutoscaler to opt it in.
          </p>
        </div>
      )}

      {groups.map(({ namespace, vpaName, first, containers }) => {
        const b = first.binding
        const invalid = first.validationErrors && first.validationErrors.length > 0
        const anchor = bindingAnchor(first)
        return (
          <section
            className={anchor === target ? 'binding is-target' : 'binding'}
            key={`${namespace}/${vpaName}`}
            id={anchor}
            aria-labelledby={`${anchor}-title`}
          >
            <header className="binding-head">
              <h2 id={`${anchor}-title`}>
                <span className="vpa-ns">{namespace} /</span> {vpaName}
              </h2>
              <dl className="facts">
                <div>
                  <dt>Workload</dt>
                  <dd>
                    {first.workload.kind}/{first.workload.name}
                  </dd>
                </div>
                <div>
                  <dt>VPA mode</dt>
                  <dd>{first.updateMode || '—'}</dd>
                </div>
                {b?.argocdApplication && (
                  <div>
                    <dt>Argo CD app</dt>
                    <dd>
                      {b.argocdApplicationNamespace ? `${b.argocdApplicationNamespace}/` : ''}
                      {b.argocdApplication}
                    </dd>
                  </div>
                )}
                <div>
                  <dt>Repository</dt>
                  <dd className="mono-path">{b?.repoURL ?? '—'}</dd>
                </div>
                <div>
                  <dt>Branch</dt>
                  <dd>{b?.branch ?? '—'}</dd>
                </div>
                {b?.writeBackPolicy && (
                  <div>
                    <dt>Write-back</dt>
                    <dd>{b.writeBackPolicy}</dd>
                  </div>
                )}
              </dl>
            </header>

            {invalid && (
              <div className="callout callout-error">
                <Icon name="error" />
                <span>
                  Invalid binding: {first.validationErrors!.join('; ')}
                </span>
              </div>
            )}

            <div className="table-wrap">
              <table className="data-table">
                <thead>
                  <tr>
                    <th>Container</th>
                    <th>Manifest</th>
                    <th>CPU request key</th>
                    <th>Memory request key</th>
                    <th>CPU limit key</th>
                    <th>Memory limit key</th>
                    <th className="num">Min. change</th>
                  </tr>
                </thead>
                <tbody>
                  {containers.map((c) => (
                    <tr key={rowKey(c)}>
                      <td>
                        <Link className="vpa-link" to={recommendationPath(c)}>
                          {c.containerName || '—'}
                        </Link>
                      </td>
                      <td>
                        <KeyPath path={c.binding?.manifestPath} />
                        {c.binding?.manifestType && <span className="cell-sub">{c.binding.manifestType}</span>}
                      </td>
                      <td>
                        <KeyPath path={c.binding?.cpuKeyPath} />
                      </td>
                      <td>
                        <KeyPath path={c.binding?.memoryKeyPath} />
                      </td>
                      <td>
                        <KeyPath path={c.binding?.cpuLimitKeyPath} />
                      </td>
                      <td>
                        <KeyPath path={c.binding?.memoryLimitKeyPath} />
                      </td>
                      <td className="num">{c.binding ? `${c.binding.minChangePercent}%` : '—'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>
        )
      })}
    </div>
  )
}
