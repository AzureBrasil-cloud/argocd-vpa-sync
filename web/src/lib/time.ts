import { useEffect, useState } from 'react'
import { formatAge } from './format'

/** The current time, refreshed every minute so relative ages stay current. */
export function useNow(): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 60_000)
    return () => clearInterval(id)
  }, [])
  return now
}

/** "24/09 15:46" in the viewer's locale and time zone. */
export function formatWhen(iso: string): string {
  return new Date(iso).toLocaleString(undefined, { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
}

export function ageSince(iso: string, now: number): string {
  return `${formatAge(Math.max(0, Math.floor((now - new Date(iso).getTime()) / 1000)))} ago`
}
