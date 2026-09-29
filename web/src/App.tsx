import { useEffect, useState } from 'react'
import { Outlet } from 'react-router-dom'
import { getUserInfo, logout, redirectToLogin } from './api/client'
import type { UserInfo } from './api/types'

export function App() {
  const [user, setUser] = useState<UserInfo | null>(null)

  useEffect(() => {
    getUserInfo()
      .then((info) => {
        if (!info.loggedIn) {
          redirectToLogin()
          return
        }
        setUser(info)
      })
      // Leave the pages to surface the error from their own requests.
      .catch(() => setUser({ loggedIn: true, authEnabled: false }))
  }, [])

  async function handleLogout() {
    await logout().catch(() => undefined)
    window.location.assign('/login')
  }

  if (!user) {
    return null
  }

  return (
    <div className="app">
      <header className="app-header">
        <div className="app-header-title">
          <span className="app-logo" aria-hidden="true">◔</span>
          <div>
            <h1>argocd-vpa-updater</h1>
            <p className="muted">Vertical Pod Autoscaler recommendations vs. what's currently running</p>
          </div>
          {user.authEnabled && (
            <div className="app-header-user">
              <span className="muted">{user.username}</span>
              <button className="btn btn-small" onClick={handleLogout}>
                Sign out
              </button>
            </div>
          )}
        </div>
      </header>
      <main>
        <Outlet />
      </main>
    </div>
  )
}
