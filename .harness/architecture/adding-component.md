# Adding a component

See skill `add-component`. Package shape:

```
<kind>/<name><kind>/
  factory.go   NewFactory() via component.NewFactory, const typ
  config.go    Config struct with yaml tags + Validate() if constraints
  <name>.go    implementation (package doc here)
  <name>_test.go
```

Then one line in `internal/registry/registry.go`.

## Shared page assets (`pages.css`, `pages.js`)

The context and MCP pages are served with the dashboard's look from `/assets/pages.css` (the same tokens, night
header band, panels, KPI strip, tables, badges and dark mode) and `/assets/pages.js` (`G.header` builds the band with
the nav and the theme switch, which shares the dashboard's saved choice; `G.el`, `human`, `usd`, `num`; `G.button(icon, text)` and `G.setButton` put an outline icon (icon-only when the text is empty: the Copy buttons) (copy, check, load, undo, sun, moon, auto, built as SVG
elements) in front of a button's text). The dashboard
keeps its own inline copy; a change to the look is made in both. Pages build their DOM with `textContent`, never `innerHTML`
(the one exception is the fixed moon literal in `pages.js`).
