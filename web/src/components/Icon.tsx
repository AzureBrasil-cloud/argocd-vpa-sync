// A small authored icon set: one 16px grid, 1.5px stroke, currentColor, so
// every theme tints them from its own palette.
const PATHS = {
  warning: (
    <>
      <path d="M8 2.2 14.3 13H1.7L8 2.2Z" />
      <path d="M8 6.5v3" />
      <path d="M8 11.4v.1" />
    </>
  ),
  error: (
    <>
      <circle cx="8" cy="8" r="6" />
      <path d="m5.8 5.8 4.4 4.4M10.2 5.8l-4.4 4.4" />
    </>
  ),
  info: (
    <>
      <circle cx="8" cy="8" r="6" />
      <path d="M8 7.2V11" />
      <path d="M8 5v.1" />
    </>
  ),
  check: <path d="m3.2 8.4 3 3 6.6-6.8" />,
  close: <path d="m4.5 4.5 7 7M11.5 4.5l-7 7" />,
  'arrow-left': <path d="M13 8H3.5M7.5 4 3.5 8l4 4" />,
  'sort-asc': <path d="M8 12.5V3.5M4.5 7 8 3.5 11.5 7" />,
  'sort-desc': <path d="M8 3.5v9M4.5 9 8 12.5 11.5 9" />,
  'external': (
    <>
      <path d="M9.5 3H13v3.5M13 3 7.5 8.5" />
      <path d="M11.5 9.5V13H3V4.5h3.5" />
    </>
  ),
  'sign-out': (
    <>
      <path d="M6.5 13H3V3h3.5" />
      <path d="M10 5l3 3-3 3M13 8H6.5" />
    </>
  ),
} as const

export type IconName = keyof typeof PATHS

interface IconProps {
  name: IconName
  /** Accessible name; omit for a decorative icon next to its own text. */
  label?: string
  className?: string
}

export function Icon({ name, label, className }: IconProps) {
  return (
    <svg
      className={`icon icon-${name}${className ? ` ${className}` : ''}`}
      viewBox="0 0 16 16"
      width="16"
      height="16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      role={label ? 'img' : undefined}
      aria-label={label}
      aria-hidden={label ? undefined : true}
      focusable="false"
    >
      {PATHS[name]}
    </svg>
  )
}

/**
 * The product mark: a request tick inside a recommended range -- the one
 * idea every theme draws, kept as the logo in all of them.
 */
export function Logo() {
  return (
    <svg className="logo" viewBox="0 0 24 24" width="24" height="24" aria-hidden="true" focusable="false">
      <rect className="logo-band" x="3" y="9" width="18" height="6" />
      <path className="logo-scale" d="M3 19h18M3 17v2M7.5 18v1M12 17v2M16.5 18v1M21 17v2" />
      <path className="logo-tick" d="M10 5v14" />
    </svg>
  )
}
