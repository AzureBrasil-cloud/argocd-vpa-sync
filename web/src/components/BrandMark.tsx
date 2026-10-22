import markLive from '../assets/brand-mark.gif'
import markStill from '../assets/brand-mark.png'

/**
 * The mark of the default and fitting-room themes: a hand-drawn flow of one
 * source branching into three manifests. At rest it shows the GIF's first
 * frame; the looping GIF takes over only while someone is at it (default:
 * the sidebar is expanded; fitting-room: the brand is hovered or focused;
 * see index.css), and never under reduced motion. The
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
