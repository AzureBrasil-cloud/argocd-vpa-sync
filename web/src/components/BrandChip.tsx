import chipLive from '../assets/brand-chip.gif'
import chipStill from '../assets/brand-chip.png'

/**
 * The default theme's sidebar mark: a chip drawn in circuit traces. At
 * rest it shows the assembled chip (the GIF's first frame); while the
 * sidebar is expanded the looping GIF takes over (index.css), so the loop
 * only runs while someone is using the sidebar, and never under reduced
 * motion.
 */
export function BrandChip() {
  return (
    <span className="brand-chip" aria-hidden="true">
      <img className="brand-chip-still" src={chipStill} alt="" width={28} height={28} />
      <img className="brand-chip-live" src={chipLive} alt="" width={28} height={28} />
    </span>
  )
}
