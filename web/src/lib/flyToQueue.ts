// A resource picked in the table travels to the write-back queue: a small
// accent chip lifts off the checkbox and arcs into the queue entry it just
// created, so the reviewer sees where the change went. Horizontal and
// vertical motion run on separate elements with separate easings, which is
// what bends the path into an arc.

const FLIGHT_MS = 520

function prefersReducedMotion(): boolean {
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

/**
 * Flies `text` from `from` to the queue entry for `key`. Call it right after
 * the selection state changes: it waits two frames for React to commit the
 * queue entry, then measures it.
 */
export function flyToQueue(from: Element, key: string, text: string): void {
  if (prefersReducedMotion() || !document.body.animate) return
  const start = from.getBoundingClientRect()

  requestAnimationFrame(() =>
    requestAnimationFrame(() => {
      const target =
        document.querySelector(`.queue-item[data-key="${CSS.escape(key)}"]`) ??
        document.querySelector('.queue-head') ??
        document.querySelector('.queue')
      if (!target) return
      const end = target.getBoundingClientRect()

      const carrier = document.createElement('div')
      carrier.className = 'queue-flyer-x'
      carrier.setAttribute('aria-hidden', 'true')
      const chip = document.createElement('div')
      chip.className = 'queue-flyer'
      chip.textContent = text
      carrier.append(chip)
      document.body.append(carrier)

      const x0 = start.left + start.width / 2 - chip.offsetWidth / 2
      const y0 = start.top + start.height / 2 - chip.offsetHeight / 2
      const x1 = end.left + 8
      const y1 = end.top + Math.min(end.height, 40) / 2 - chip.offsetHeight / 2
      // The arc's peak sits above the higher of the two ends.
      const peak = Math.min(y0, y1) - Math.min(72, Math.abs(x1 - x0) * 0.18)

      carrier.animate([{ transform: `translateX(${x0}px)` }, { transform: `translateX(${x1}px)` }], {
        duration: FLIGHT_MS,
        easing: 'cubic-bezier(0.45, 0, 0.25, 1)',
      })
      const flight = chip.animate(
        [
          { transform: `translateY(${y0}px) scale(0.6)`, opacity: 0, easing: 'cubic-bezier(0.2, 0.6, 0.4, 1)' },
          { transform: `translateY(${peak}px) scale(1)`, opacity: 1, offset: 0.42, easing: 'cubic-bezier(0.6, 0, 0.8, 0.4)' },
          { transform: `translateY(${y1}px) scale(0.7)`, opacity: 0.2 },
        ],
        { duration: FLIGHT_MS },
      )
      flight.onfinish = flight.oncancel = () => carrier.remove()
    }),
  )
}
