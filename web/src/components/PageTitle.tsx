import type { ReactNode } from 'react'
import { SECTIONS, type SectionKey } from '../nav'
import { useTheme } from '../theme'

interface PageTitleProps {
  section: SectionKey
  /** Defaults to the section's nav label. */
  children?: ReactNode
}

/**
 * A page's h1. In the default theme it leads with the section's sidebar
 * icon, which acts out its section once when the page opens (index.css).
 */
export function PageTitle({ section, children }: PageTitleProps) {
  const theme = useTheme()
  const { icon: SectionIcon, motion, label } = SECTIONS[section]
  return (
    <h1 className="page-title">
      {theme === 'default' && (
        <SectionIcon
          className={`page-title-icon nav-icon nav-icon-${motion}`}
          size={24}
          strokeWidth={1.75}
          aria-hidden="true"
        />
      )}
      {children ?? label}
    </h1>
  )
}
