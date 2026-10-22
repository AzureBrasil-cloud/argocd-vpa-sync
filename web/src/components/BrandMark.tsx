import markLive from '../assets/brand-mark.gif'
import markStill from '../assets/brand-mark.png'

/**
 * The default theme's mark: a hand-drawn flow of one source branching into
 * three manifests. At rest it shows the GIF's first frame; while the sidebar
 * is expanded the looping GIF takes over (index.css), so the loop only runs
 * while someone is using the sidebar, and never under reduced motion. The
 * loading screen draws the same mark, live.
 */
export function BrandMark() {
  return (
    <span className="brand-mark" aria-hidden="true">
      <img className="brand-mark-still" src={markStill} alt="" width={28} height={28} />
      <img className="brand-mark-live" src={markLive} alt="" width={28} height={28} />
    </span>
  )
}
