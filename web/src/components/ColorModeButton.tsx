import { useEffect, useState } from 'react'
import { flushSync } from 'react-dom'
import { Moon, Sun } from 'lucide-react'
import {
  applyColorMode,
  hasStoredColorMode,
  resolveColorMode,
  storeColorMode,
  systemColorMode,
  type ColorMode,
} from '../theme'

/**
 * The default theme's light/dark switch: the sidebar's last item, above
 * the hairline that closes off the session, drawn like a nav link. It shows the mode it switches to (a moon in light, a sun in
 * dark). The switch cross-fades the whole page (a View Transition) and the
 * new icon turns in; without the API or under reduced motion the page just
 * changes.
 */
export function ColorModeButton() {
  const [mode, setMode] = useState<ColorMode>(resolveColorMode)

  // Until the viewer picks a mode, follow the system's as it changes.
  useEffect(() => {
    const media = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = () => {
      if (hasStoredColorMode()) return
      const next = systemColorMode()
      applyColorMode(next)
      setMode(next)
    }
    media.addEventListener('change', onChange)
    return () => media.removeEventListener('change', onChange)
  }, [])

  function toggle() {
    const next: ColorMode = mode === 'dark' ? 'light' : 'dark'
    storeColorMode(next)
    const apply = () => {
      applyColorMode(next)
      flushSync(() => setMode(next))
    }
    if (document.startViewTransition && !window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
      document.startViewTransition(apply)
    } else {
      apply()
    }
  }

  const label = mode === 'dark' ? 'Light mode' : 'Dark mode'
  const ModeIcon = mode === 'dark' ? Sun : Moon
  return (
    <button className="nav-link mode-button" onClick={toggle} aria-label={`Switch to ${label.toLowerCase()}`}>
      <ModeIcon
        key={mode}
        className={`mode-icon mode-icon-${mode === 'dark' ? 'sun' : 'moon'}`}
        size={16}
        strokeWidth={1.5}
        aria-hidden="true"
      />
      <span className="mode-text" aria-hidden="true">
        {label}
      </span>
    </button>
  )
}
