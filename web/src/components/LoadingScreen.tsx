import { useEffect, useState } from 'react'
import markLive from '../assets/brand-mark.gif'
import markStill from '../assets/brand-mark.png'
import { currentLoadStage, subscribeLoad, type LoadStage } from '../lib/loadProgress'

// Each stage owns a band of the bar. The bar starts a stage at `from` and
// creeps toward `cap` while it lasts, so it keeps moving on a slow API but
// never claims a stage that hasn't happened; the next real event jumps it on.
const STAGES: Record<LoadStage, { label: string; from: number; cap: number }> = {
  request: { label: 'Asking the API', from: 8, cap: 52 },
  response: { label: 'Receiving data', from: 60, cap: 88 },
  done: { label: 'Building the view', from: 92, cap: 99 },
}

// Fast loads never show the screen at all, so it doesn't flash.
const SHOW_AFTER_MS = 150

/**
 * A page's loading state: the product mark, live, over a progress bar that
 * follows the API client's real stages (lib/loadProgress) with the stage
 * named under it and the percentage beside it.
 */
export function LoadingScreen({ title }: { title: string }) {
  const [visible, setVisible] = useState(false)
  const [stage, setStage] = useState<LoadStage>(currentLoadStage)
  const [percent, setPercent] = useState(0)

  useEffect(() => {
    const timer = setTimeout(() => setVisible(true), SHOW_AFTER_MS)
    return () => clearTimeout(timer)
  }, [])

  useEffect(() => subscribeLoad(() => setStage(currentLoadStage())), [])

  useEffect(() => {
    const { from, cap } = STAGES[stage]
    let frame = 0
    const tick = () => {
      setPercent((p) => {
        const base = Math.max(p, from)
        return base >= cap ? base : base + (cap - base) * 0.035
      })
      frame = requestAnimationFrame(tick)
    }
    frame = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(frame)
  }, [stage])

  if (!visible) return <div className="load" aria-busy="true" />

  const shown = Math.floor(percent)
  const { label } = STAGES[stage]
  return (
    <div className="load load-visible" aria-busy="true">
      <picture className="load-mark">
        <source srcSet={markStill} media="(prefers-reduced-motion: reduce)" />
        <img src={markLive} alt="" width={64} height={64} />
      </picture>
      <p className="load-title">{title}</p>
      <div
        className="load-bar"
        role="progressbar"
        aria-label={title}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={shown}
        aria-valuetext={`${label}, ${shown}%`}
      >
        <span className="load-fill" style={{ transform: `scaleX(${percent / 100})` }} />
      </div>
      <p className="load-meta">
        <span>{label}</span>
        <span className="load-percent">{shown}%</span>
      </p>
    </div>
  )
}
