import type { ReactNode } from 'react'

interface HintProps {
  /** The tooltip text shown on hover, or on keyboard focus. */
  children: ReactNode
  /** What is hovered: an ⓘ icon by default. */
  trigger?: ReactNode
  className?: string
  /** Which edge of the trigger the bubble lines up with; 'right' for a trigger at the end of a row. */
  align?: 'left' | 'right'
}

/**
 * An inline explanation that appears on hover (or keyboard focus) as a
 * styled bubble -- used instead of the native title attribute, which shows
 * late or not at all on some platforms and can't hold longer text.
 */
export function Hint({ children, trigger, className, align = 'left' }: HintProps) {
  return (
    <span className={`hint ${className ?? 'limit-settings-info'}`} tabIndex={0}>
      {trigger ?? <span aria-hidden="true">ⓘ</span>}
      <span className={align === 'right' ? 'hint-bubble hint-bubble-right' : 'hint-bubble'} role="tooltip">
        {children}
      </span>
    </span>
  )
}
