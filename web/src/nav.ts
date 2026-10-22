import { Gauge, History, Link2, Settings, type LucideIcon } from 'lucide-react'

// The dashboard's four sections, shared by the shell's nav and each page's
// title. The icons are drawn in the default theme only (sidebar and page
// titles); `motion` names each icon's animation in index.css: the gauge
// needle takes a reading, the clock hands rewind, the link's halves close,
// the cog turns one tooth.
export interface Section {
  to: string
  label: string
  icon: LucideIcon
  motion: 'gauge' | 'history' | 'link' | 'cog'
  end?: boolean
}

export const SECTIONS = {
  recommendations: { to: '/', label: 'Recommendations', icon: Gauge, motion: 'gauge', end: true },
  history: { to: '/history', label: 'Write-back history', icon: History, motion: 'history' },
  bindings: { to: '/bindings', label: 'Bindings', icon: Link2, motion: 'link' },
  settings: { to: '/settings', label: 'Settings', icon: Settings, motion: 'cog' },
} as const satisfies Record<string, Section>

export type SectionKey = keyof typeof SECTIONS

export const NAV: readonly Section[] = Object.values(SECTIONS)
