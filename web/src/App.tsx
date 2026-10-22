import { useEffect, useState } from 'react'
import { NavLink, Outlet } from 'react-router-dom'
import { Gauge, History, Link2, Settings } from 'lucide-react'
import { getUserInfo, logout, redirectToLogin } from './api/client'
import type { UserInfo } from './api/types'
import { Icon, Logo } from './components/Icon'
import { applyTheme, resolveTheme, ThemeContext, type Theme } from './theme'

// Icons are drawn only in the default theme's sidebar; the navbar themes
// keep text-only links. `motion` names each icon's hover/arrival animation
// in index.css: the gauge needle takes a reading, the clock hands rewind,
// the link's halves close, the cog turns one tooth.
const NAV = [
  { to: '/', label: 'Recommendations', icon: Gauge, motion: 'gauge', end: true },
  { to: '/history', label: 'Write-back history', icon: History, motion: 'history' },
  { to: '/bindings', label: 'Bindings', icon: Link2, motion: 'link' },
  { to: '/settings', label: 'Settings', icon: Settings, motion: 'cog' },
]

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
            <Logo />
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
                {item.label}
              </NavLink>
            ))}
          </nav>
          {user.authEnabled && (
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
