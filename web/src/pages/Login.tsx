import { useEffect, useState, type FormEvent } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { getUserInfo, listRecommendations, login } from '../api/client'
import { Icon } from '../components/Icon'
import { EntryLoader } from '../components/LoadingScreen'
import markLive from '../assets/brand-mark.gif'
import markStill from '../assets/brand-mark.png'
import { applyTheme, resolveTheme } from '../theme'

/**
 * Where to go after logging in: the ?next= path, but only a same-origin
 * absolute path -- never "//host" or a full URL, which would make this an
 * open redirect.
 */
function safeNext(next: string | null): string {
  if (!next || !next.startsWith('/') || next.startsWith('//') || next.startsWith('/\\')) {
    return '/'
  }
  return next
}

export function Login() {
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  // After a good sign-in the card becomes the entrance loader while the
  // dashboard's first data loads; `ready` once that request has settled.
  const [entering, setEntering] = useState(false)
  const [ready, setReady] = useState(false)

  // The theme comes from the same unauthenticated endpoint the shell uses.
  useEffect(() => {
    getUserInfo()
      .then((info) => applyTheme(resolveTheme(info.theme)))
      .catch(() => applyTheme(resolveTheme(undefined)))
  }, [])

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setSubmitting(true)
    setError(null)
    try {
      await login(username, password)
      setEntering(true)
      // Warms the first screen; a failure here is the dashboard's to report.
      listRecommendations()
        .catch(() => undefined)
        .finally(() => setReady(true))
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      setError(
        message.startsWith('401')
          ? 'Invalid username or password.'
          : message.startsWith('429')
            ? 'Too many failed attempts. Wait a moment and try again.'
            : message,
      )
      setSubmitting(false)
    }
  }

  if (entering) {
    return (
      <main className="login">
        <div className="login-form login-entering">
          <EntryLoader
            title="Opening argocd-vpa-sync"
            minDurationMs={2000}
            finished={ready}
            onComplete={() => navigate(safeNext(params.get('next')), { replace: true })}
          />
        </div>
      </main>
    )
  }

  return (
    <main className="login">
      <form className="login-form" onSubmit={handleSubmit}>
        <div className="login-brand">
          {/* The product mark, live here (its still frame under reduced
              motion): the sign-in screen is where it introduces the product. */}
          <picture className="login-mark" aria-hidden="true">
            <source srcSet={markStill} media="(prefers-reduced-motion: reduce)" />
            <img src={markLive} alt="" width={56} height={56} />
          </picture>
          <h1>argocd-vpa-sync</h1>
        </div>
        <p className="login-sub">Sign in to review VPA recommendations and queue write-backs to Git.</p>
        <label className="field">
          <span>Username</span>
          <input name="username" autoComplete="username" value={username} onChange={(e) => setUsername(e.target.value)} required />
        </label>
        <label className="field">
          <span>Password</span>
          <input
            name="password"
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoFocus
            required
          />
        </label>
        {error && (
          <div className="callout callout-error" role="alert">
            <Icon name="error" />
            <span>{error}</span>
          </div>
        )}
        <button className="btn btn-primary btn-block" type="submit" disabled={submitting}>
          {submitting ? 'Signing in…' : 'Sign in'}
        </button>
      </form>
    </main>
  )
}
