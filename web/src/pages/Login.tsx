import { useEffect, useState, type FormEvent } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { getUserInfo, login } from '../api/client'
import { Icon, Logo } from '../components/Icon'
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
      navigate(safeNext(params.get('next')), { replace: true })
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

  return (
    <main className="login">
      <form className="login-form" onSubmit={handleSubmit}>
        <div className="login-brand">
          <Logo />
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
