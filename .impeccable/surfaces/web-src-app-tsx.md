---
version: 1
slug: "web-src-app-tsx"
primary_target: "web/src/App.tsx"
related_targets: ["web/src/pages"]
---

# Dashboard (all routes)

Scope: the whole SPA in web/ — shell, Recommendations, Recommendation detail, Write-back history, Bindings, Settings, Login. Mode: Operate.

Audience and job: SRE/platform engineers in a weekly office review on a large monitor; scan the fleet, select CPU/Memory changes, apply a batch that lands as Git commits.

Constraints: the user chose THREE install-time themes instead of one world, selected via Helm `theme` (`default` | `fitting-room` | `calibration`), default `default`. Same routes, components and behavior in all three; themes differ in type, palette, density, shell placement and signature move only. Light grounds (office daylight). Copy stays in VPA/GitOps terms. No invented capabilities (no export, no rescan, no PR flow).

Removed on purpose: Table/Cards toggle (table only), per-row Accept buttons (the selection panel is the single apply path).

## Direction contract

THESIS: Recommendations are read as measurements against a range, and every change leaves through one reviewed queue to Git. Refuses the generic SaaS admin table with pills, rounded cards and a purple accent.

OWN-WORLD:
- fitting-room: charcoal #23262B navbar shell, tape-yellow #F2C230 committed accent (active tab, selection rows, primary button), cool off-white #F6F7F5 ground, chalk-blue #3B6FB6 recommendation mark, red #C8372D failure, green #2E7D4F eligible; zero radius, 1px hairlines, no shadows; Archivo-like grotesk, tabular figures.
- calibration: ink #1C2430 header band with a certificate identification strip, white ground, steel #5B6B7C secondary, seal blue #2348A8 accent, green IN #1F7A4D, vermilion OUT #B8391F; serial row numbers; mono only for SHAs and diffs.
- default: the category standard played straight — white ground, slate #0F172A, blue #2563EB, #E2E8F0 borders, left sidebar, 6px radius.

STORY: The reviewer sees how many workloads are eligible, why the others are not, picks changes, checks the queue panel (headroom, limits, warnings), applies, then confirms on Write-back history.

FIRST VIEWPORT: navbar with Recommendations / Write-back history / Bindings / Settings; page title with live counts (workloads, eligible, failed); filter bar (search, eligibility, status); guide line naming the namespace range in view; dense table (VPA, CPU, Memory, Eligibility, Write-back) whose resource cells carry the theme's gauge; sticky queue panel on the right with the Apply button.

FORM: Signature move per theme — fitting-room: tape-measure gauge (ruled scale, grey VPA band, black current tick, chalk-blue recommendation arrow; hatched when not eligible); calibration: tolerance bar plus IN/OUT verdict chip and a values-diff preview in the queue panel; default: plain delta badges. Raises carried: direction by mark shape and eligibility by fill, guide line, zero radius hairlines (fitting-room), j/k to step through eligible workloads on the detail page. Assigned direction (fitting-room) is position 7 of the ordered list; calibration is the pick; default is the canon exit. seed:53503e7d3d25bec1

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance
