import { Outlet } from 'react-router-dom'

export function App() {
  return (
    <div className="app">
      <header className="app-header">
        <div className="app-header-title">
          <span className="app-logo" aria-hidden="true">◔</span>
          <div>
            <h1>argocd-vpa-updater</h1>
            <p className="muted">Vertical Pod Autoscaler recommendations vs. what's currently running</p>
          </div>
        </div>
      </header>
      <main>
        <Outlet />
      </main>
    </div>
  )
}
