---
name: argocd-vpa-sync
description: A review desk for VPA recommendations that leave for the cluster only as Git commits, shipped in three install-time themes over one set of pages.
colors:
  default-bg: "#ffffff"
  default-surface-2: "#f1f5f9"
  default-ink: "#0f172a"
  default-ink-2: "#334155"
  default-ink-3: "#5b6474"
  default-line: "#e2e8f0"
  default-line-strong: "#cbd5e1"
  default-accent: "#2563eb"
  default-accent-hover: "#1d4ed8"
  default-accent-soft: "#eff6ff"
  default-ok: "#15803d"
  default-ok-soft: "#dcfce7"
  default-warn: "#a14b06"
  default-warn-soft: "#ffedd5"
  default-bad: "#b91c1c"
  default-bad-soft: "#fee2e2"
  default-band: "#dbe3ee"
  fitting-room-bg: "#f6f7f5"
  fitting-room-surface: "#ffffff"
  fitting-room-surface-2: "#eceee9"
  fitting-room-ink: "#1d1f23"
  fitting-room-ink-2: "#3f444b"
  fitting-room-ink-3: "#61666e"
  fitting-room-line: "#d9dcd6"
  fitting-room-charcoal: "#23262b"
  fitting-room-accent: "#f2c230"
  fitting-room-accent-hover: "#e3b11c"
  fitting-room-accent-soft: "#fbf1cc"
  fitting-room-ok: "#2e7d4f"
  fitting-room-ok-soft: "#dcefe3"
  fitting-room-warn: "#8a5d00"
  fitting-room-bad: "#c8372d"
  fitting-room-bad-soft: "#f8dfdc"
  fitting-room-rec: "#3b6fb6"
  fitting-room-band: "#dcdfd9"
  fitting-room-shell-ink-2: "#b4b9c0"
  fitting-room-shell-line: "#3a3e45"
  calibration-bg: "#ffffff"
  calibration-surface-2: "#f3f5f8"
  calibration-ink: "#1c2430"
  calibration-ink-2: "#3c4857"
  calibration-ink-3: "#5b6b7c"
  calibration-line: "#dde3ea"
  calibration-accent: "#2348a8"
  calibration-accent-hover: "#1b3a8c"
  calibration-accent-soft: "#edf1fb"
  calibration-shell-mark: "#8fa8e8"
  calibration-ok: "#1f7a4d"
  calibration-ok-soft: "#e3f2ea"
  calibration-warn: "#8a5a00"
  calibration-warn-soft: "#fbf0d9"
  calibration-bad: "#b8391f"
  calibration-bad-soft: "#fbe6e1"
  calibration-band: "#d3dbe4"
  calibration-shell-ink-2: "#aeb8c4"
  calibration-shell-line: "#313c4b"
typography:
  default-title:
    fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif"
    fontSize: "22px"
    fontWeight: 650
    lineHeight: 1.2
    letterSpacing: "-0.015em"
  default-body:
    fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif"
    fontSize: "14px"
    fontWeight: 400
    lineHeight: 1.45
    fontFeature: "tnum"
  default-label:
    fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif"
    fontSize: "12px"
    fontWeight: 600
    letterSpacing: "0"
  fitting-room-title:
    fontFamily: "'Archivo Variable', 'Arial Narrow', system-ui, sans-serif"
    fontSize: "24px"
    fontWeight: 700
    lineHeight: 1.2
    letterSpacing: "-0.015em"
    fontVariation: "'wdth' 92"
  fitting-room-body:
    fontFamily: "'Archivo Variable', 'Arial Narrow', system-ui, sans-serif"
    fontSize: "14px"
    fontWeight: 400
    lineHeight: 1.45
    fontFeature: "tnum"
  fitting-room-label:
    fontFamily: "'Archivo Variable', 'Arial Narrow', system-ui, sans-serif"
    fontSize: "11px"
    fontWeight: 650
    letterSpacing: "0.08em"
  calibration-title:
    fontFamily: "'Public Sans Variable', system-ui, sans-serif"
    fontSize: "22px"
    fontWeight: 700
    lineHeight: 1.2
    letterSpacing: "-0.015em"
  calibration-body:
    fontFamily: "'Public Sans Variable', system-ui, sans-serif"
    fontSize: "14px"
    fontWeight: 400
    lineHeight: 1.45
    fontFeature: "tnum"
  calibration-label:
    fontFamily: "'Public Sans Variable', system-ui, sans-serif"
    fontSize: "11px"
    fontWeight: 650
    letterSpacing: "0.07em"
  calibration-id-value:
    fontFamily: "'Public Sans Variable', system-ui, sans-serif"
    fontSize: "20px"
    fontWeight: 700
    lineHeight: 1.2
  queue-title:
    fontSize: "18px"
    fontWeight: 700
    letterSpacing: "-0.01em"
  reading-detail:
    fontSize: "18px"
    fontWeight: 650
    fontFeature: "tnum"
  mono:
    fontFamily: "'JetBrains Mono Variable', ui-monospace, Menlo, monospace"
    fontSize: "12px"
    fontWeight: 400
rounded:
  default-sm: "4px"
  default-md: "6px"
  default-container: "8px"
  fitting-room: "0px"
  calibration-sm: "2px"
  calibration-md: "3px"
spacing:
  xs: "4px"
  sm: "8px"
  md: "12px"
  lg: "16px"
  xl: "24px"
  2xl: "32px"
  cell: "10px 12px"
  queue: "18px"
  gutter-default: "24px"
  gutter-navbar: "32px"
  gutter-mobile: "16px"
components:
  button:
    backgroundColor: "{colors.default-bg}"
    textColor: "{colors.default-ink}"
    rounded: "{rounded.default-md}"
    padding: "0 12px"
    height: "32px"
  button-small:
    height: "28px"
    padding: "0 10px"
  button-primary-default:
    backgroundColor: "{colors.default-accent}"
    textColor: "{colors.default-bg}"
    rounded: "{rounded.default-md}"
    height: "42px"
  button-primary-default-hover:
    backgroundColor: "{colors.default-accent-hover}"
  button-primary-fitting-room:
    backgroundColor: "{colors.fitting-room-accent}"
    textColor: "{colors.fitting-room-ink}"
    rounded: "{rounded.fitting-room}"
    height: "42px"
  button-primary-fitting-room-hover:
    backgroundColor: "{colors.fitting-room-accent-hover}"
  button-primary-calibration:
    backgroundColor: "{colors.calibration-accent}"
    textColor: "{colors.calibration-bg}"
    rounded: "{rounded.calibration-md}"
    height: "42px"
  button-primary-calibration-hover:
    backgroundColor: "{colors.calibration-accent-hover}"
  input:
    backgroundColor: "{colors.default-bg}"
    textColor: "{colors.default-ink}"
    rounded: "{rounded.default-md}"
    padding: "0 10px"
    height: "32px"
  table-cell:
    padding: "{spacing.cell}"
  queue-panel:
    backgroundColor: "{colors.default-bg}"
    rounded: "{rounded.default-container}"
    padding: "{spacing.queue}"
    width: "300px"
  queue-panel-navbar-themes:
    width: "320px"
  queue-head-fitting-room:
    backgroundColor: "{colors.fitting-room-charcoal}"
    textColor: "{colors.fitting-room-bg}"
    padding: "16px 18px"
  badge-increase:
    backgroundColor: "{colors.default-warn-soft}"
    textColor: "{colors.default-warn}"
    rounded: "{rounded.default-sm}"
    padding: "2px 8px"
  badge-decrease:
    backgroundColor: "{colors.default-ok-soft}"
    textColor: "{colors.default-ok}"
    rounded: "{rounded.default-sm}"
    padding: "2px 8px"
  badge-muted:
    backgroundColor: "{colors.default-surface-2}"
    textColor: "{colors.default-ink-2}"
    rounded: "{rounded.default-sm}"
    padding: "2px 8px"
  verdict-in:
    backgroundColor: "{colors.calibration-ok-soft}"
    textColor: "{colors.calibration-ok}"
    rounded: "{rounded.calibration-sm}"
    padding: "1px 5px"
  verdict-out:
    backgroundColor: "{colors.calibration-bad}"
    textColor: "{colors.calibration-bg}"
    rounded: "{rounded.calibration-sm}"
    padding: "1px 5px"
  nav-link-default-active:
    backgroundColor: "{colors.default-accent-soft}"
    textColor: "{colors.default-accent-hover}"
    rounded: "{rounded.default-md}"
    padding: "8px 10px"
  id-strip-cell:
    backgroundColor: "{colors.calibration-ink}"
    textColor: "{colors.calibration-bg}"
    padding: "14px 32px 16px"
---

# Design System: argocd-vpa-sync

## Overview

**Creative North Star: "One Desk, Three Instruments"**

The dashboard is a review desk for a weekly batch of VPA recommendations, read on a large monitor in office daylight. Every page, component and behavior is shared; what changes per install is the instrument the reviewer reads a recommendation with. The Helm value `theme` (env `DASHBOARD_THEME`) picks one of three named variants: **Default** (`default`), the familiar admin console with a left sidebar and delta badges; **The Fitting Room** (`fitting-room`), a charcoal navbar, tape-yellow commitment and a tape-measure gauge per resource; **Calibration Certificate** (`calibration`), an ink header band with an identification strip, a tolerance bar and an IN/OUT verdict per resource, and a values diff in the queue. All three are light grounds.

The shared system is dense and tabular: one fixed-layout table of workloads, a sticky guide line naming the namespace range on screen, and a sticky write-back queue on the right that is the only way to apply from the list. Meaning never rides on colour alone: eligibility is carried by fill (solid vs hatched), direction by shape (arrowhead, tick position), write-back status by a distinct mark per state. Themes swap tokens and a small set of scoped rules under `:root[data-theme]`; they never fork a page.

Depth is flat. Surfaces sit on hairlines, not shadows; the one lifted object is the hover tooltip that carries each reading's full breakdown.

**Key Characteristics:**
- Three install-time variants over identical routes, components and behavior; tokens redefined per `[data-theme]`.
- Dense, tabular figures everywhere (`font-variant-numeric: tabular-nums` on body).
- Shape + fill marks for eligibility and status, readable without colour.
- One apply path on the list: the sticky write-back queue panel.
- A guide line above the table naming the first and last namespace in view.
- Flat surfaces on 1px hairlines; the tooltip is the only shadowed element.

## Colors

Each variant is a light, low-chroma ground with one committed accent and a fixed trio of state colours (ok, warn, bad) plus a recommendation colour (`--rec`) and a range-band grey (`--band`).

### Primary
- **Console Blue** (default-accent): default's primary button, selected-row tint (via default-accent-soft), active sidebar link (accent-soft fill, accent-text label and icon), each page title's section icon, the user badge's hover and the account dialog's avatar, checkbox fill and focus ring (via its hover shade).
- **Default dark mode** (`[data-mode='dark']`, a per-viewer choice): composed, not inverted. Canvas #0b1120, sidebar #0e1626, panels and table #121a2b, headers and hover #1a2438; ink #e2e8f0 / #b4bfd0 / #8b97ab; lines #243047 / #34425c. Accent text lifts to #9bbcff (9.1:1 on panels, 8:1 on its #16244a soft fill), which also carries the active sidebar link and the section icons; the primary button keeps #2563eb (white text 5.2:1). Status: ok #4ade80 on #0f2d1f, warn #fbbf5a on #33240c, bad #f87171 on #3a1518. The chip mark is inverted with its hue turned back and screened, so its white ground drops out and the teal stays teal.
- **Tape Yellow** (fitting-room-accent): fitting-room's committed accent. Fills the primary button (with a 1px ink border), the pressed segmented filter, the active navbar underline (3px), selection highlight, the queue count, and the selected-row tint (via fitting-room-accent-soft). Text on it is always ink, never white.
- **Seal Blue** (calibration-accent): calibration's primary button, links, pressed segment text and selected-row tint (via calibration-accent-soft).

### Secondary
- **Chalk Blue** (fitting-room-rec): the recommendation move on the tape-measure gauge and the eligible delta figure. Default's `--rec` is its accent blue; calibration's `--rec` is ink.
- **Pale Seal** (calibration-shell-mark): on calibration's ink band only: the logo band and the active nav underline.

### Tertiary (state)
- **Eligible / IN green** (default-ok, fitting-room-ok, calibration-ok): solid eligibility mark, applied status dot, decrease badge, IN verdict, the "to" side of the values diff.
- **Warning amber** (default-warn, fitting-room-warn, calibration-warn): increase badge, warning callouts, limits that go down.
- **Failure red / OUT vermilion** (default-bad, fitting-room-bad, calibration-bad): failed status triangle, error callouts, invalid inputs, OUT verdict (solid fill, white text), the "from" side of the values diff (struck through).

### Neutral
- **Grounds**: default and calibration on pure white (default-bg, calibration-bg); fitting-room on cool off-white (fitting-room-bg) with white table and queue surfaces (fitting-room-surface).
- **Ink ramp** (`--ink`, `--ink-2`, `--ink-3`): primary text, secondary text and table headers, meta lines and placeholders. Slate in default, near-black in fitting-room, ink-navy in calibration.
- **Hairlines** (`--line`, `--line-strong`, `--head-rule`): row dividers and control borders. In fitting-room and calibration `--line-strong` and `--head-rule` are the ink itself, so headings and the guide line sit on a 2px ink rule.
- **Shell grounds**: default's sidebar is white with a 1px line; fitting-room's navbar and queue head are charcoal (fitting-room-charcoal); calibration's navbar and id strip are ink (calibration-ink).
- **Range band** (`--band`): the grey VPA range on gauges and the legend.

### Named Rules
**The One Commitment Rule.** Each variant has exactly one accent, and it marks what the reviewer has committed to: the primary button, selected rows, the active section. State colours are never borrowed as accents.

**The Muted-When-Ineligible Rule.** A reading is coloured by direction only when it can be selected. A green decrease on a resource that is not eligible reads as an invitation; ineligible readings are muted (grey badge, hatched move, regular-weight delta).

## Typography

**Default font:** the platform UI stack (system-ui, with -apple-system, Segoe UI, Roboto fallbacks)
**Fitting-room font:** Archivo Variable (with Arial Narrow), titles condensed to 92% width
**Calibration font:** Public Sans Variable (with system-ui)
**Mono (fitting-room, calibration):** JetBrains Mono Variable; default uses ui-monospace. Mono is reserved for commit SHAs, manifest paths, key paths and the values diff.

All three faces are self-hosted via Fontsource and bundled into the binary.

**Character:** a working sans at 14px with tabular figures throughout; the variants differ in how labels speak. Default labels are sentence case; fitting-room and calibration labels are small uppercase with open tracking, like the printing on a measuring tool or a certificate.

### Hierarchy
- **Title** (650-700, 22-24px, 1.2, -0.015em): the page title. Calibration's list page replaces it with the identification strip. In default, the title leads with its section's sidebar icon, bare (24px, 1.75 stroke, accent text), 10px before the text, and the icon plays its section's motion once when the page opens; the subtitle sits 8px below on the page's left edge, and a hairline 18px under it closes the header band before the work area. The title renders in loading, error and empty states too.
- **Queue title** (700, 18px): "N changes selected"; fitting-room sets the count in tape yellow at 26px/800.
- **ID value** (700, 20px, calibration only): the five figures in the identification strip, with 12.5px small-text units.
- **Section heading** (700, 15-16px): binding names, detail sections, settings sections.
- **Body** (400, 14px, 1.45, tabular): table cells and prose; secondary text 13px; meta lines 12px in `--ink-3`. Page subtitles cap at 90ch, notes at 75ch.
- **Label** (`--label-size` / `--label-weight` / `--label-case` / `--label-tracking`): table headers, fact terms, detail reading titles. Default 12px/600 sentence case; fitting-room 11px/650 uppercase 0.08em; calibration 11px/650 uppercase 0.07em.
- **Reading values** (13px row, 18px detail): "current → target" with the target at 650 and the delta at 12px.

### Named Rules
**The Tabular Figures Rule.** Every number on screen uses tabular figures so values align down a column; the body sets it once.

**The Mono-For-Git Rule.** Monospace is for what lands in Git (SHAs, file paths, key paths, diffs), never for labels or figures.

## Layout

The page grid is the **work layout**: a fluid main column and a fixed queue column (300px in default, 320px in fitting-room and calibration) with a 24px gap, top-aligned. The page pads 24px top and the theme gutter at the sides (24px default, 32px navbar themes, 16px under 860px), capped at 1720px wide; narrow pages (settings, bindings) cap at 880px.

**Shell placement by variant.** Default: a sticky left sidebar (brand, vertical nav, session pinned to the bottom above a hairline) that rests on desktop as a 64px rail of icons and, while the pointer is on it or keyboard focus is in it, expands to 208px by widening its grid column, so the page narrows and widens with it and is never covered. Opening is the softer move: the column glides out over 360ms `cubic-bezier(0.32, 0.72, 0, 1)` after 60ms, then the labels follow top to bottom (from 170ms, 30ms apart, the account text last), each fading up over 220ms while sliding 6px toward its icon. Closing is quicker and in one piece: labels drop out in 90ms and the column folds back over 220ms `cubic-bezier(0.4, 0, 0.2, 1)` after a 160ms pause. Default's brand mark is a circuit-trace chip (28px, from the supplied GIF, `web/src/assets/brand-chip.*`): its first frame at rest, the looping GIF only while the sidebar is open, never under reduced motion. Under 860px it is the top bar instead. Fitting-room and calibration: a 48px sticky top navbar (`--bar-h`) with brand, horizontal nav and the session pushed right. Calibration's list page adds the identification strip, a full-bleed five-column band in the shell ink directly under the navbar (cluster, bindings, eligible, failed write-backs, last commit) that stands in for the page title.

**Rhythm.** Controls are 32px tall (28px small, 38px form fields, 42px block primary). Table cells pad 10px 12px. Gaps step 4 / 8 / 12 / 16 / 24 / 32px. The filter bar wraps with 10px 12px gaps. In default it holds search (fills up to 380px, Lucide search icon inset 10px), the eligibility filter and Status; "Select eligible" sits on the guide line, pushed right, since it acts on the set the guide line names. The navbar themes keep "Select eligible" pushed right in the filter bar, where their guide line carries the gauge legend.

**The guide line.** Sticky under the shell (`top: var(--bar-h)`), it names the namespace range in view ("atlas-stg — monitor"), the visible count, and (navbar themes) a legend for the gauge marks. It sits on a 2px head rule in calibration, on a strip of tape ruling (1px ticks every 8px, longer every 80px) in fitting-room, and with no rule in default.

**Responsive.** At 1240px the queue drops below the table and stops being sticky; the id strip goes to three columns. At 860px the gutter becomes 16px, default's sidebar becomes a top bar, the guide line unsticks and loses its legend, calibration's serial column hides, and the VPA column sticks left while the readings scroll sideways. At 600px the nav wraps to its own row and the navbar themes stop being sticky.

## Elevation & Depth

Flat by construction. Surfaces separate by hairlines and tonal grounds (off-white ground vs white table in fitting-room, surface-2 header row in default), and by the dark shell bands in the navbar themes. The only shadowed element is the hover tooltip; fitting-room's focus ring uses a box-shadow as a second ring, not as elevation.

### Shadow Vocabulary
- **Tooltip** (`box-shadow: 0 6px 20px rgb(15 23 42 / 0.18)`): the dark hint bubble that carries a reading's full breakdown and eligibility reasons.
- **Fitting-room focus ring** (`box-shadow: 0 0 0 4px` tape yellow, under a 2px ink outline): focus-visible on every control in that variant.

### Named Rules
**The Hairline Rule.** Containers are drawn with 1px lines, headings with 2px rules. No surface floats.

## Shapes

Corners are a per-variant token: default rounds controls at 6px and badges at 4px, with the table, queue and login card at 8px; calibration is nearly square (3px, 2px small); fitting-room is square (0px) everywhere, including checkboxes' surroundings, buttons, inputs and the queue.

The recurring geometry is the **mark**, a 10px glyph that encodes state by shape and fill:
- Eligibility: solid square (eligible, green), hatched square at 135deg (not eligible), solid triangle (error, red).
- Write-back status: outlined (new), accent-filled (selected), half-filled (applying), solid (applied; ink in fitting-room, green elsewhere), outlined with a diagonal stroke (skipped), red triangle (failed). In default and calibration status marks are 9px circles; in fitting-room they stay square.

Gauges are drawn from absolutely positioned bars, ticks and clip-path arrowheads, never images.

## Components

### Buttons
- **Shape:** the variant radius (6px / 0 / 3px); 32px tall, 12px side padding, 13px/550.
- **Secondary (default):** surface fill, 1px border (`--line` in default, `--line-strong` elsewhere, which is ink in the navbar themes); hover to surface-2 in 120ms ease-out.
- **Primary:** accent fill, weight 650. In fitting-room the border is ink and the label is ink on tape yellow. The queue's Apply button is the block size (full width, 42px, 14.5px).
- **Disabled:** 45% opacity, not-allowed cursor.
- **Link button:** underlined text in `--link` ("Clear selection").
- **Icon button:** 26px square, transparent until hover (line border, surface-2).

### Select eligible
- **Style:** three small buttons ("CPU + memory", "Memory", "CPU") after a muted label, acting on the rows on screen; ticking is additive.
- **Pressed:** the option the selection currently matches (all eligible changes on screen of both resources, of memory only, or of CPU only) shows pressed: accent-soft fill, accent-text border and label, and a leading check (Lucide Check, 14px) so the state isn't colour alone; tape yellow with ink in fitting-room. It is read from the selection, so unticking a row clears it; pressing the pressed option again unticks those resources.

### Segmented filter
- **Style:** one bordered group (All / Eligible / Not eligible) with counts in `--ink-3`; 30px tall.
- **Pressed:** accent-soft fill with accent text; in fitting-room, solid tape yellow with ink text and an ink border on the group. In default, one accent-soft highlight slides to the pressed option (240ms, `cubic-bezier(0.16, 1, 0.3, 1)`) instead of each button filling. One shared component (`Segmented`) on Recommendations and Write-back history.

### Inputs / Fields
- **Style:** 32px, 10px side padding, 1px border (`--line-strong`; calibration uses `--line`), variant radius. Form fields on login and settings are 38px.
- **Focus:** 2px outline in accent text colour, 2px offset; fitting-room adds the 4px yellow ring under an ink outline.
- **Error:** border and fill switch to bad / bad-soft.
- **Checkbox:** 16px native, accent-coloured (ink in fitting-room); disabled at 35% opacity.

### Navigation
- **Default:** vertical sidebar links, 14px/550, 8px 10px padding, rounded, each led by a 16px Lucide icon (1.5 stroke, 10px gap: Gauge, History, Link2, Settings); hover surface-2; active is accent-soft fill with accent text. Each icon acts out its section once on hover, keyboard focus and arrival (440–560ms, `cubic-bezier(0.16, 1, 0.3, 1)`): the gauge needle swings back and settles, the clock hands rewind a turn, the link halves part and close, the cog turns one tooth.
- **Fitting-room / calibration:** full-height navbar links, 13.5px/550 in shell ink-2, hover to shell ink; active gets a 3px bottom border (tape yellow / pale seal) and white text.
- **Default light/dark switch:** the sidebar's last item, drawn as a nav link (16px Lucide Moon in light, Sun in dark, 1.5 stroke; "Dark mode" / "Light mode" shown with the expanded sidebar), 8px above the session's hairline; in the mobile top bar it sits right, beside the user button. It starts from the system preference, follows the system until the viewer picks, and remembers the pick in this browser. Switching cross-fades the page (View Transition) and the new icon turns in (380ms, from -90° and 60%); on hover the sun's rays turn 45° and the moon rocks, once.
- **Default session:** one button under the hairline at the foot of the sidebar: a 36px round user badge (Lucide UserRound, 1px line, surface fill; hover accent-soft with accent icon) beside "Signed in as" (12px, ink-3) over the username (13.5px/700). The rail shows the badge only; the text fades in with the expanded sidebar; the mobile top bar shows the badge only. On hover and keyboard focus the figure lifts its head (2.5px) and shoulders (1px) once, 480ms `cubic-bezier(0.16, 1, 0.3, 1)`. It opens the account dialog: a native `<dialog>` (360px, 1px line, 8px radius, no shadow, slate backdrop at 32%) with an accent-soft avatar, "Signed in as" and the username, one line of copy, and Cancel (focused) / Sign out (primary, LogOut icon). Escape and a backdrop click close it; it enters with a 240ms fade and 8px rise.
- **Mobile:** brand name and username hide; the nav wraps to its own row.

### Recommendation table
- **Style:** fixed layout, full-width white surface; columns for checkbox, (calibration) serial number, VPA · namespace, CPU request → rec., Memory request → rec., Eligibility, Write-back. Headers use the label style; default puts them on a surface-2 row inside an 8px rounded, 1px bordered table, the navbar themes on open hairlines.
- **Rows:** 1px dividers; hover surface-2; selected rows tinted accent-soft.
- **Serials (calibration):** three-digit row numbers in `--ink-3` that stay with the workload when the table is re-sorted.

### Resource reading (the per-variant signature)
One component, three instruments, the same tooltip breakdown in all three (current request, VPA recommendation, headroom maths, change, eligibility reasons).
- **Default: delta badge.** A 4px-rounded badge "target (±n%)": amber for an increase, green for a decrease, grey when not eligible or unchanged; an extra green check badge when the request already matches.
- **Fitting-room: tape-measure gauge.** Ruled scale along the bottom (minor ticks every 5%, major every 25%), the VPA range as a grey band, the live request as a 2px ink tick, the move to the recommendation as a 4px chalk-blue bar ending in an arrowhead pointing the direction of change. Not eligible: the bar is hatched and slightly faded. Unchanged: an outlined diamond at the target. Values sit below ("22m → 27m  +22.7%"). Eligible bars draw in from the current tick over 420ms (`cubic-bezier(0.16, 1, 0.3, 1)`), skipped under reduced motion.
- **Calibration: tolerance bar + verdict.** Values sit above. A 2px rule with the VPA range between 2px end caps, the recommendation as a 2px ink nominal tick, and the live request as the measured point: a green ring inside tolerance, a solid vermilion dot outside it. A verdict chip follows: IN (green text on green-soft, tinted border) or OUT (white on solid vermilion), 11px/650, 0.06em tracking.

### Eligibility and write-back cells
- **Style:** mark + 13px/600 word ("Eligible", "Not eligible", "Applied", "Failed"), with a 12px `--ink-3` reason or note line indented under the word (resources, "within VPA range", commit SHA and age).

### Write-back queue
The list page's single apply path. A sticky panel (top = bar height + 16px) holding: title with count and "N containers · commits to <branch>", request totals (from → to, with limit lines), a scrolling list of selected containers with remove buttons and a "+N more below" chip over a fade, headroom and limit settings, warning/error callouts, the block Apply button and the caption "Write-back commits to Git; Argo CD syncs from there."
- **Default:** white panel, 1px line, 8px radius, 18px padding.
- **Fitting-room: alteration ticket.** Square, ink-bordered; the head is a charcoal band with the count in tape yellow, and a 2px dashed tear line separates it from the totals and the stub of changes.
- **Calibration: values diff.** Each selected container shows its manifest path in mono and a bordered mono diff: key path, old value struck through in vermilion, arrow, new value bold green.
- **Empty:** "Nothing selected" with a hint and the Git-only reassurance.
- **Motion (all themes):** the panel acknowledges each selection change. A part that appears (filled or empty state, a totals row, a resource line, a callout) settles in over 220ms (4px rise, `cubic-bezier(0.16, 1, 0.3, 1)`); a resource ticked in the table flies to the queue as a small accent chip ("cpu 250m"), arcing from the checkbox to its queue entry over 520ms (horizontal and vertical on separate easings, peak up to 72px above the higher end), and the entry holds an accent-soft tint that fades over 1.4s once the chip lands; the count rolls up when it grows and down when it shrinks (280ms); a changed total flashes in the accent text for 600ms. Removals are instant. Reduced motion keeps only the tint.

### Callouts
- **Style:** 1px border at 35% of the state colour, soft state fill, leading SVG icon in the state colour, 13px text, variant radius. Error and warning only.

### Identification strip (calibration)
Five equal cells on the shell ink with 1px shell-line dividers: an uppercase tracked label (11px/650, 0.08em) over a 20px/700 value with small 12.5px units; failed count in a light vermilion when non-zero.

### Detail page
Two reading panels side by side (1px line, variant radius, 16px padding) at the detail gauge size (28px tall; values 18px), a binding facts panel, and an actions panel (eligibility, headroom and limit settings) with the primary "Queue selected for write-back" button pushed right; a "Queued for write-back" selected badge shows once queued. Previous / Next eligible links show `j` / `k` key caps (1px border with a 2px bottom border).

## Do's and Don'ts

### Do:
- **Do** put every new colour, radius, label style and gutter behind a `--token` and redefine it under each `:root[data-theme]`; components read tokens, themes never fork a page.
- **Do** carry meaning by shape and fill first: solid vs hatched for eligibility, arrowhead or tick position for direction, a distinct mark per write-back status.
- **Do** route every write into the write-back queue (the list's queue panel, or the detail page's "Queue selected for write-back"), and keep the copy saying the change goes to Git.
- **Do** keep numbers tabular and right-aligned where they compare (totals, serials).
- **Do** keep the reading's full breakdown in the hover tooltip in every variant, not in the cell.
- **Do** keep fitting-room square (0px) and its accent text ink on yellow.
- **Do** honour `prefers-reduced-motion`; the gauge draw-in, the default theme's section-icon motions and sliding filter highlight, and the queue panel's change feedback are the only animations.

### Don't:
- **Don't** add per-row Accept buttons to the table; on the list, the queue panel is the single apply path.
- **Don't** colour a reading by direction when it isn't eligible.
- **Don't** reintroduce a table/cards toggle; the list is a table.
- **Don't** lift surfaces with shadows; the tooltip is the only shadowed object.
- **Don't** use monospace for anything that does not land in Git.
- **Don't** give a variant a dark ground; all three are read in office daylight.
