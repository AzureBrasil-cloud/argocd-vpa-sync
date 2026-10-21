import { createContext, useContext } from 'react'

// The dashboard's install-time themes, chosen with the Helm value "theme"
// (DASHBOARD_THEME, see internal/config). Every theme renders the same pages
// and behavior; they differ in type, palette, shell layout and how a
// resource's recommendation is drawn:
//   default       the standard admin console: left sidebar, delta badges
//   fitting-room  charcoal navbar, tape-measure gauge per resource
//   calibration   ink header band, tolerance bar with an IN/OUT verdict
// The tokens and layout for each live in index.css under [data-theme].
export const THEMES = ['default', 'fitting-room', 'calibration'] as const

export type Theme = (typeof THEMES)[number]

export const THEME_LABELS: Record<Theme, string> = {
  default: 'Default',
  'fitting-room': 'The Fitting Room',
  calibration: 'Calibration Certificate',
}

export function isTheme(value: unknown): value is Theme {
  return typeof value === 'string' && (THEMES as readonly string[]).includes(value)
}

/**
 * The theme to render: the server's, except that a development build
 * (npm run dev) also takes ?theme=<name> so every theme can be previewed
 * without redeploying.
 */
export function resolveTheme(serverTheme: unknown): Theme {
  if (import.meta.env.DEV) {
    const override = new URLSearchParams(window.location.search).get('theme')
    if (isTheme(override)) return override
  }
  return isTheme(serverTheme) ? serverTheme : 'default'
}

export function applyTheme(theme: Theme): void {
  document.documentElement.dataset.theme = theme
}

export const ThemeContext = createContext<Theme>('default')

export function useTheme(): Theme {
  return useContext(ThemeContext)
}
