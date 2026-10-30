# Visual style

How the dashboard (`/`), `/costs`, `/context`, `/mcp` and `/setup` look, so a new block fits in. Not loaded
by default: read it before changing a page. The dashboard is the reference; the other pages share
`cmd/gascoigne/pages.css` and `pages.js` (the `G` helpers), which copy its vocabulary.

## Look
- Burgundy night header with the moon, moonlight page background, white rounded panels with a soft
  shadow. Colors, spacing and fonts are the `:root` tokens (`--accent`, `--surface`, `--muted`, `--faint`,
  `--ok`, `--warn`, `--err`, each with a `-soft`, and `--track`). Never hard-code a color; dark mode is
  the same tokens redefined, so a hard-coded one breaks it.
- Light, quiet type: big thin numbers (32px, weight 300) in KPI tiles, 13px body, 12px labels. The accent
  color is for the one number that matters on a page (`kpi accent`), bars and the active control.
- Icons are the 24px line icons in `ICON_SHAPES` (dashboard) and `G.icon` (pages). A button with an action
  has an icon (`data-icon`); add a shape there, never an icon font or image.

## Page anatomy, top to bottom
1. Header (`G.header`), then a toolbar: the controls (Period) on one line, **the description on its own
   full line below them** (`.toolbar .hint` is `flex: 1 1 100%`). A description never shares a line with
   controls.
2. KPI row (`.kpis`): 3-6 tiles, name, big value, a faint plain-words line. At most one accent tile.
3. Charts and tables in panels (`.panel`), two to a row in a `.grid` (equal height, notes pinned to the
   bottom with `margin-top: auto`), a wide table alone.
4. Explanations last ("How this is computed"), in two columns (`ul.plain.cols`).
5. A footer line saying what the numbers are not.

## Blocks
- A panel is `h2` + a one-line `.hint` description + content. The hint is its own full line; in a head
  with controls (`.feed-head`) the controls wrap below it.
- Choosing between views: a segmented control (`.seg`) with an icon per segment, never underlined tabs.
- Charts: daily bars (`G.bars`: amount above the bar, label below, labels thinned past 10, amounts hidden
  past 16 bars, quiet days kept as a thin track-colored stub), proportional bars (`G.hbars`, `.stack`),
  a line over a filled area with labelled guides (`G.area`). Bars use `--accent`; a second series or an
  assumed value uses `--warn`. No chart libraries.
- Tables: sortable headers, right-aligned tabular numbers (`td.num`), a share-of-total bar where a column
  is a share (`.share` + `.bar`), a filter box on long ones. A long text cell wraps (`td.cause`); a
  table inside a half-width panel must fit without a horizontal scroll, so drop or wrap before cutting
  a column.
- Status: `.badge` (ok/warn/err/neutral) for an outcome, a chip with a dot for an on/off state.
- Say what a figure is. Money is "API-equivalent", an inferred cause is "likely", an assumed value is
  marked "assumed". A page that only works for one agent says so under its title.
- Empty states are a faint sentence (`.empty`), never a blank box and never the text `null`:
  `replaceChildren` writes a null as text, so filter optional blocks (`.filter(Boolean)`).

## Pitfalls we hit
- `.wrap` is the page container (max width, 40px bottom padding): never use it as a cell or helper class.
  Check a new class name against `pages.css` and the page's `<style>` first.
- A box's height comes from its neighbour in a `.grid`; empty space inside a short box is fine, a stretched
  table row is not.
- The pages are embedded in the binary: after editing one, `make build` and restart the server
  (`server stop`, then `server run`), or you keep looking at the old page.

## Check it by eye
Layout bugs do not show in tests. Run the page and look at it:

```sh
make build && export GASCOIGNE_SERVER_ADDR=127.0.0.1:7992
bin/gascoigne server stop; bin/gascoigne server run -idle 10m &
.harness/scripts/shot.sh /context /tmp/context.png 1280 2400     # then view the png
```

`shot.sh` needs `firefox` and `python3` (it holds the load event so the page's data has arrived). Look
at a wide window and at 640px; check dark mode by setting the theme button. Pin structure that must not
regress in a Go test over the embedded HTML (`TestDashboardLayout`, `TestCostsPageLayout`).
