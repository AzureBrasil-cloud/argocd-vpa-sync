import { useEffect, useState } from 'react'
import { getSettings } from '../api/client'
import type { SettingsDTO } from '../api/types'
import { Icon } from '../components/Icon'
import { RequestHeadroomInput } from '../components/RequestHeadroomInput'
import { useRequestHeadroom } from '../lib/useRequestHeadroom'
import { THEME_LABELS, THEMES, useTheme, type Theme } from '../theme'

const THEME_NOTES: Record<Theme, string> = {
  default: 'Standard admin console: left sidebar, blue accent, delta badges.',
  'fitting-room': 'Charcoal navbar, tape-yellow accent; each CPU/memory cell is a tape-measure gauge.',
  calibration: 'Ink header band and summary strip; tolerance bars with an IN/OUT verdict and a values-diff preview.',
}

function formatDuration(seconds?: number): string {
  if (!seconds) return '—'
  if (seconds % 3600 === 0) return `${seconds / 3600}h`
  if (seconds % 60 === 0) return `${seconds / 60}m`
  return `${seconds}s`
}

/**
 * The deployment's configuration, read-only (it comes from the Helm values),
 * plus the per-browser defaults the dashboard remembers.
 */
export function Settings() {
  const theme = useTheme()
  const headroom = useRequestHeadroom()
  const [settings, setSettings] = useState<SettingsDTO | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    getSettings()
      .then(setSettings)
      .catch((err) => setError(String(err)))
  }, [])

  return (
    <div className="page page-narrow">
      <header className="page-head">
        <h1 className="page-title">Settings</h1>
        <p className="page-sub">
          Deployment settings come from the Helm values and are read-only here. Change them with <code>helm upgrade</code>.
        </p>
      </header>

      {error && (
        <div className="callout callout-error">
          <Icon name="error" />
          <span>Failed to load settings: {error}</span>
        </div>
      )}

      <section className="settings-section" aria-labelledby="settings-theme">
        <h2 id="settings-theme">Theme</h2>
        <ul className="theme-list">
          {THEMES.map((t) => (
            <li key={t} className={t === theme ? 'is-current' : undefined}>
              <span className="theme-name">
                {THEME_LABELS[t]} <code>{t}</code>
                {t === theme && <span className="tag">in use</span>}
              </span>
              <span className="cell-sub">{THEME_NOTES[t]}</span>
            </li>
          ))}
        </ul>
        <pre className="snippet">
          <code>{`helm upgrade argocd-vpa-sync oci://registry-1.docker.io/azurebrasil/argocd-vpa-sync \\
  -n argocd --reuse-values --set theme=fitting-room`}</code>
        </pre>
      </section>

      {settings && (
        <>
          <section className="settings-section" aria-labelledby="settings-writeback">
            <h2 id="settings-writeback">Write-back</h2>
            <dl className="facts facts-stacked">
              <div>
                <dt>Worker poll interval</dt>
                <dd>{formatDuration(settings.writeBackPollIntervalSeconds)}</dd>
              </div>
              <div>
                <dt>Argo CD namespace</dt>
                <dd>{settings.argocdNamespace}</dd>
              </div>
              <div>
                <dt>State Secret</dt>
                <dd className="mono-path">
                  {settings.stateSecretNamespace}/{settings.stateSecretName}
                </dd>
              </div>
            </dl>
          </section>

          <section className="settings-section" aria-labelledby="settings-auth">
            <h2 id="settings-auth">Authentication</h2>
            <dl className="facts facts-stacked">
              <div>
                <dt>Login</dt>
                <dd>{settings.authEnabled ? 'Required (admin password set)' : 'Off: the dashboard is open to anyone who can reach it'}</dd>
              </div>
              {settings.authEnabled && (
                <>
                  <div>
                    <dt>Username</dt>
                    <dd>{settings.username}</dd>
                  </div>
                  <div>
                    <dt>Session length</dt>
                    <dd>{formatDuration(settings.sessionTtlSeconds)}</dd>
                  </div>
                </>
              )}
            </dl>
          </section>

          <section className="settings-section" aria-labelledby="settings-version">
            <h2 id="settings-version">Version</h2>
            <dl className="facts facts-stacked">
              <div>
                <dt>Version</dt>
                <dd>{settings.version}</dd>
              </div>
              <div>
                <dt>Commit</dt>
                <dd>
                  <code className="sha">{settings.gitCommit}</code>
                </dd>
              </div>
              <div>
                <dt>Built</dt>
                <dd>{settings.buildDate}</dd>
              </div>
            </dl>
          </section>
        </>
      )}

      <section className="settings-section" aria-labelledby="settings-browser">
        <h2 id="settings-browser">Defaults in this browser</h2>
        <p className="cell-sub">
          The request headroom new selections start with. It is remembered in this browser only; limit settings are chosen in
          the write-back queue.
        </p>
        <RequestHeadroomInput settings={headroom} resources={['cpu', 'memory']} />
      </section>
    </div>
  )
}
