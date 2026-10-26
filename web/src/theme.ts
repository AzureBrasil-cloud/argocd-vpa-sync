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

// Used when the server sends no (or an unknown) theme. Keep in sync with
// config.DefaultTheme.
export const DEFAULT_THEME: Theme = 'fitting-room'

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
  return isTheme(serverTheme) ? serverTheme : DEFAULT_THEME
}

export function applyTheme(theme: Theme): void {
  document.documentElement.dataset.theme = theme
}

export const ThemeContext = createContext<Theme>(DEFAULT_THEME)

export function useTheme(): Theme {
  return useContext(ThemeContext)
}

// The default theme's light/dark mode, a per-viewer choice kept in this
// browser (index.css remaps the default theme's tokens under
// [data-mode='dark']). Until the viewer picks one, it follows the system.
export type ColorMode = 'light' | 'dark'

const COLOR_MODE_KEY = 'argocd-vpa-updater.colorMode'

function storedColorMode(): ColorMode | null {
  try {
    const value = localStorage.getItem(COLOR_MODE_KEY)
    return value === 'light' || value === 'dark' ? value : null
  } catch {
    return null
  }
}

export function systemColorMode(): ColorMode {
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

export function resolveColorMode(): ColorMode {
  return storedColorMode() ?? systemColorMode()
}

export function hasStoredColorMode(): boolean {
  return storedColorMode() !== null
}

export function applyColorMode(mode: ColorMode): void {
  document.documentElement.dataset.mode = mode
}

export function storeColorMode(mode: ColorMode): void {
  try {
    localStorage.setItem(COLOR_MODE_KEY, mode)
  } catch {
    // Private windows and blocked storage: the choice lasts this page only.
  }
}
