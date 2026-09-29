import { useState, type FormEvent } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { login } from '../api/client'

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
    <div className="login">
      <form className="panel login-form" onSubmit={handleSubmit}>
        <div className="app-header-title">
          <span className="app-logo" aria-hidden="true">◔</span>
          <h1>argocd-vpa-updater</h1>
        </div>
        <label className="login-field">
          <span>Username</span>
          <input
            name="username"
            autoComplete="username"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            required
          />
        </label>
        <label className="login-field">
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
        {error && <div className="panel panel-error">{error}</div>}
        <button className="btn btn-primary" type="submit" disabled={submitting}>
          {submitting ? 'Signing in…' : 'Sign in'}
        </button>
      </form>
    </div>
  )
}
