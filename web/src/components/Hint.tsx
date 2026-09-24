import { useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'

interface HintProps {
  /** The tooltip text shown on hover, or on keyboard focus. */
  children: ReactNode
  /** What is hovered: an ⓘ icon by default. */
  trigger?: ReactNode
  className?: string
  /** Which edge of the trigger the bubble lines up with; 'right' for a trigger at the end of a row. */
  align?: 'left' | 'right'
  /** False when the trigger already holds a focusable control (e.g. a checkbox), to avoid a second tab stop. */
  focusable?: boolean
}

const GAP = 6
const MARGIN = 8

/**
 * An inline explanation that appears on hover (or keyboard focus) as a
 * styled bubble -- used instead of the native title attribute, which shows
 * late or not at all on some platforms (never on a disabled input) and
 * can't hold longer text. The bubble is portaled to <body> with fixed
 * positioning, so a scrolling or overflow-hidden ancestor (the table panel,
 * the floating selection summary) can't clip it; it flips above the
 * trigger when there's no room below and stays inside the viewport.
 */
export function Hint({ children, trigger, className, align = 'left', focusable = true }: HintProps) {
  const triggerRef = useRef<HTMLSpanElement>(null)
  const bubbleRef = useRef<HTMLSpanElement>(null)
  const [open, setOpen] = useState(false)
  const [pos, setPos] = useState<{ top: number; left: number } | null>(null)

  useLayoutEffect(() => {
    if (!open || !triggerRef.current || !bubbleRef.current) return
    const t = triggerRef.current.getBoundingClientRect()
    const b = bubbleRef.current.getBoundingClientRect()
    const below = t.bottom + GAP
    const top = below + b.height > window.innerHeight - MARGIN ? Math.max(MARGIN, t.top - GAP - b.height) : below
    const preferred = align === 'right' ? t.right - b.width : t.left
    const left = Math.min(Math.max(MARGIN, preferred), window.innerWidth - MARGIN - b.width)
    setPos({ top, left })
  }, [open, align])

  function show() {
    setPos(null)
    setOpen(true)
  }

  return (
    <span
      ref={triggerRef}
      className={`hint ${className ?? 'limit-settings-info'}`}
      tabIndex={focusable ? 0 : undefined}
      onMouseEnter={show}
      onMouseLeave={() => setOpen(false)}
      // Focus bubbles up from a control inside the trigger too; only
      // keyboard focus opens it, so a click doesn't leave it hanging open.
      onFocus={(e) => (e.target as HTMLElement).matches(':focus-visible') && show()}
      onBlur={() => setOpen(false)}
    >
      {trigger ?? <span aria-hidden="true">ⓘ</span>}
      {open &&
        createPortal(
          <span
            ref={bubbleRef}
            className="hint-bubble"
            role="tooltip"
            // Measured once invisibly, then placed (see useLayoutEffect).
            style={pos ? { top: pos.top, left: pos.left } : { top: 0, left: 0, visibility: 'hidden' }}
          >
            {children}
          </span>,
          document.body,
        )}
    </span>
  )
}
