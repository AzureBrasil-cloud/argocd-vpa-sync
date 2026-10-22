import { useEffect, useState } from 'react'
import { NavLink, Outlet } from 'react-router-dom'
import { getUserInfo, logout, redirectToLogin } from './api/client'
import type { UserInfo } from './api/types'
import { AccountButton } from './components/AccountButton'
import { BrandChip } from './components/BrandChip'
import { Icon, Logo } from './components/Icon'
import { NAV } from './nav'
import { applyTheme, resolveTheme, ThemeContext, type Theme } from './theme'

/**
 * The shell every theme shares: brand, the four sections and the session.
 * Where it sits is the theme's call (index.css): a left sidebar in default,
 * a top navbar in fitting-room and calibration.
 */
export function App() {
  const [user, setUser] = useState<UserInfo | null>(null)
  const [theme, setTheme] = useState<Theme>(() => resolveTheme(undefined))

  useEffect(() => {
    getUserInfo()
      .then((info) => {
        const resolved = resolveTheme(info.theme)
        applyTheme(resolved)
        setTheme(resolved)
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
    <ThemeContext.Provider value={theme}>
      <div className="shell">
        <a className="skip-link" href="#main">
          Skip to content
        </a>
        <header className="shell-bar">
          <NavLink to="/" className="brand" end>
            {theme === 'default' ? <BrandChip /> : <Logo />}
            <span className="brand-name">argocd-vpa-sync</span>
          </NavLink>
          <nav className="nav" aria-label="Sections">
            {NAV.map((item) => (
              <NavLink key={item.to} to={item.to} end={item.end} className="nav-link">
                {theme === 'default' && (
                  <item.icon
                    className={`nav-icon nav-icon-${item.motion}`}
                    size={16}
                    strokeWidth={1.5}
                    aria-hidden="true"
                  />
                )}
                <span className="nav-label">{item.label}</span>
              </NavLink>
            ))}
          </nav>
          {user.authEnabled && theme === 'default' && (
            <div className="session">
              <AccountButton username={user.username} onSignOut={handleLogout} />
            </div>
          )}
          {user.authEnabled && theme !== 'default' && (
            <div className="session">
              <span className="session-user">{user.username}</span>
              <button className="btn btn-small btn-shell" onClick={handleLogout} aria-label="Sign out">
                <Icon name="sign-out" />
                <span className="btn-label">Sign out</span>
              </button>
            </div>
          )}
        </header>
        <main id="main" className="shell-main">
          <Outlet />
        </main>
      </div>
    </ThemeContext.Provider>
  )
}
