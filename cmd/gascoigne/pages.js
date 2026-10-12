// Shared by the context and MCP pages: theme, header and small helpers.
"use strict";
(() => {
  try { const t = localStorage.getItem("gascoigne-theme"); if (t === "light" || t === "dark") document.documentElement.dataset.theme = t; } catch {}

  const el = (tag, attrs, ...kids) => {
    const e = document.createElement(tag);
    for (const [k, v] of Object.entries(attrs || {})) { if (k === "text") e.textContent = v; else e.setAttribute(k, v); }
    for (const kid of kids) if (kid !== null && kid !== undefined && kid !== false) e.append(kid);
    return e;
  };
  const human = (n) => n >= 1e6 ? (n / 1e6).toFixed(n >= 1e7 ? 0 : 1) + "M" : n >= 1e3 ? (n / 1e3).toFixed(n >= 1e4 ? 0 : 1) + "k" : String(n);
  const usd = (n) => "$" + (n >= 100 ? n.toFixed(0) : n.toFixed(2));
  const num = (n) => n.toLocaleString("en-US");

  const moon = `<svg viewBox="0 0 48 48" aria-hidden="true"><defs>
    <radialGradient id="pg-lit" cx="38%" cy="32%" r="72%"><stop offset="0" stop-color="#fffdfb"/><stop offset=".6" stop-color="#ece4e2"/><stop offset="1" stop-color="#c9bcbf"/></radialGradient>
    <radialGradient id="pg-crater" cx="38%" cy="38%" r="62%"><stop offset="0" stop-color="#9c8b8f"/><stop offset=".75" stop-color="#c4b6b8"/><stop offset="1" stop-color="#fffaf8"/></radialGradient>
    <radialGradient id="pg-halo" r="50%"><stop offset=".78" stop-color="#f6ecf0" stop-opacity=".18"/><stop offset="1" stop-color="#f6ecf0" stop-opacity="0"/></radialGradient></defs>
    <circle cx="24" cy="24" r="24" fill="url(#pg-halo)"/><circle cx="24" cy="24" r="19" fill="url(#pg-lit)"/>
    <g fill="#b3a2a7" opacity=".5"><path d="M10 18c3-5 10-6 14-3 3 3 0 7-4 8-4 1-7 4-10 1-2-2-2-4 0-6z"/><path d="M25 26c4-3 10-2 12 2 2 3-1 7-5 7-5 1-9-1-9-5 0-2 1-3 2-4z"/></g>
    <g fill="url(#pg-crater)"><circle cx="31" cy="17" r="3"/><circle cx="35" cy="28" r="2"/><circle cx="21" cy="29" r="2.4"/><circle cx="15" cy="15" r="1.6"/></g></svg>`;

  // header builds the same night band as the dashboard; current is the page's own link.
  const header = (target, title, subtitle, current) => {
    const links = [["/", "Dashboard", "dashboard"], ["/context", "Context", "context"], ["/mcp", "MCP servers", "mcp"]];
    const theme = el("div", { class: "theme", role: "group", "aria-label": "Theme" });
    const saved = (() => { try { return localStorage.getItem("gascoigne-theme") || "auto"; } catch { return "auto"; } })();
    for (const name of ["auto", "light", "dark"]) {
      const b = el("button", { type: "button", "aria-pressed": String(saved === name), text: name[0].toUpperCase() + name.slice(1) });
      b.addEventListener("click", () => {
        try { name === "auto" ? localStorage.removeItem("gascoigne-theme") : localStorage.setItem("gascoigne-theme", name); } catch {}
        if (name === "auto") delete document.documentElement.dataset.theme; else document.documentElement.dataset.theme = name;
        theme.querySelectorAll("button").forEach((x) => x.setAttribute("aria-pressed", String(x === b)));
      });
      theme.append(b);
    }
    const brand = el("div", { class: "brand" }, el("div", { class: "moon" }), el("div", {}, el("h1", { text: title }), el("div", { class: "subtitle", text: subtitle })));
    brand.querySelector(".moon").innerHTML = moon; // a fixed literal, no data
    target.replaceWith(el("header", {}, brand, el("div", { class: "head-right" },
      ...links.map(([href, text, id]) => el("a", { class: "navlink", href, text, ...(id === current ? { "aria-current": "page" } : {}) })), theme)));
  };

  window.G = { el, human, usd, num, header };
})();
