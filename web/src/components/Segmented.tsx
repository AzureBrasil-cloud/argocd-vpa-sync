import { useLayoutEffect, useRef } from 'react'

interface SegmentedOption<T extends string> {
  value: T
  label: string
  count: number
}

interface SegmentedProps<T extends string> {
  /** Accessible name for the group. */
  label: string
  options: readonly SegmentedOption<T>[]
  value: T
  onChange: (value: T) => void
}

/**
 * A row of mutually exclusive filter buttons with counts. The pressed
 * button's offset and width are published as --seg-x / --seg-w so the
 * default theme can slide one highlight between options (index.css);
 * the other themes fill the pressed button instead.
 */
export function Segmented<T extends string>({ label, options, value, onChange }: SegmentedProps<T>) {
  const ref = useRef<HTMLDivElement>(null)

  // Counts and late-loading fonts change button widths, so re-measure on
  // every render and on resize.
  useLayoutEffect(() => {
    const group = ref.current
    if (!group) return
    const measure = () => {
      const pressed = group.querySelector<HTMLElement>('[aria-pressed="true"]')
      if (!pressed) return
      group.style.setProperty('--seg-x', `${pressed.offsetLeft}px`)
      group.style.setProperty('--seg-w', `${pressed.offsetWidth}px`)
    }
    measure()
    // Enable the slide only after the first placement, so the highlight
    // doesn't grow in from the left edge on load.
    const frame = requestAnimationFrame(() => group.setAttribute('data-ready', ''))
    const observer = new ResizeObserver(measure)
    observer.observe(group)
    return () => {
      cancelAnimationFrame(frame)
      observer.disconnect()
    }
  })

  return (
    <div ref={ref} className="segmented" role="group" aria-label={label}>
      {options.map((option) => (
        <button key={option.value} aria-pressed={value === option.value} onClick={() => onChange(option.value)}>
          {option.label} <span className="count">{option.count}</span>
        </button>
      ))}
    </div>
  )
}
