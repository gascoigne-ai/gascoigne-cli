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

  // Icons: 24-pixel outline shapes, drawn with the text's colour. Built as SVG elements, never parsed from strings.
  const shapes = {
    copy: [["rect", { x: 9, y: 9, width: 13, height: 13, rx: 2 }], ["path", { d: "M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" }]],
    check: [["path", { d: "M20 6 9 17l-5-5" }]],
    load: [["path", { d: "M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" }], ["path", { d: "m7 10 5 5 5-5" }], ["path", { d: "M12 15V3" }]],
    undo: [["path", { d: "M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8" }], ["path", { d: "M3 3v5h5" }]],
    sun: [["circle", { cx: 12, cy: 12, r: 4 }], ["path", { d: "M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41" }]],
    moon: [["path", { d: "M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z" }]],
    auto: [["rect", { x: 2, y: 3, width: 20, height: 14, rx: 2 }], ["path", { d: "M8 21h8M12 17v4" }]],
  };
  const NS = "http://www.w3.org/2000/svg";
  const icon = (name) => {
    const svg = document.createElementNS(NS, "svg");
    for (const [k, v] of Object.entries({ class: "icon", viewBox: "0 0 24 24", fill: "none", stroke: "currentColor", "stroke-width": 2, "stroke-linecap": "round", "stroke-linejoin": "round", "aria-hidden": "true" })) svg.setAttribute(k, v);
    for (const [tag, attrs] of shapes[name] || []) {
      const e = document.createElementNS(NS, tag);
      for (const [k, v] of Object.entries(attrs)) e.setAttribute(k, v);
      svg.append(e);
    }
    return svg;
  };
  // button is a text button with an icon in front; setButton changes both (a copy button says "Copied").
  // Without text it is an icon-only button, named for screen readers by its title.
  const setButton = (b, name, text) => {
    b.classList.toggle("icon-only", !text);
    if (text) b.replaceChildren(icon(name), el("span", { text })); else b.replaceChildren(icon(name));
    return b;
  };
  const button = (name, text, attrs) => {
    const b = setButton(el("button", { type: "button", ...(attrs || {}) }), name, text);
    if (!text && attrs && attrs.title) b.setAttribute("aria-label", attrs.title);
    return b;
  };

  const moon = `<svg viewBox="0 0 48 48" aria-hidden="true"><defs>
    <radialGradient id="pg-lit" cx="38%" cy="32%" r="72%"><stop offset="0" stop-color="#fffdfb"/><stop offset=".6" stop-color="#ece4e2"/><stop offset="1" stop-color="#c9bcbf"/></radialGradient>
    <radialGradient id="pg-crater" cx="38%" cy="38%" r="62%"><stop offset="0" stop-color="#9c8b8f"/><stop offset=".75" stop-color="#c4b6b8"/><stop offset="1" stop-color="#fffaf8"/></radialGradient>
    <radialGradient id="pg-halo" r="50%"><stop offset=".78" stop-color="#f6ecf0" stop-opacity=".18"/><stop offset="1" stop-color="#f6ecf0" stop-opacity="0"/></radialGradient></defs>
    <circle cx="24" cy="24" r="24" fill="url(#pg-halo)"/><circle cx="24" cy="24" r="19" fill="url(#pg-lit)"/>
    <g fill="#b3a2a7" opacity=".5"><path d="M10 18c3-5 10-6 14-3 3 3 0 7-4 8-4 1-7 4-10 1-2-2-2-4 0-6z"/><path d="M25 26c4-3 10-2 12 2 2 3-1 7-5 7-5 1-9-1-9-5 0-2 1-3 2-4z"/></g>
    <g fill="url(#pg-crater)"><circle cx="31" cy="17" r="3"/><circle cx="35" cy="28" r="2"/><circle cx="21" cy="29" r="2.4"/><circle cx="15" cy="15" r="1.6"/></g></svg>`;

  // header builds the dashboard's night band, identical on every page: the same brand and links, the page's own link
  // marked. The page's title and subtitle go in a line below it.
  const header = (target, title, subtitle, current) => {
    const links = [["/", "Dashboard", "dashboard"], ["/costs", "Costs", "costs"], ["/context", "Context", "context"], ["/mcp", "MCP servers", "mcp"]];
    const theme = el("div", { class: "theme", role: "group", "aria-label": "Theme" });
    const saved = (() => { try { return localStorage.getItem("gascoigne-theme") || "auto"; } catch { return "auto"; } })();
    for (const name of ["auto", "light", "dark"]) {
      const b = button({ auto: "auto", light: "sun", dark: "moon" }[name], name[0].toUpperCase() + name.slice(1), { "aria-pressed": String(saved === name) });
      b.addEventListener("click", () => {
        try { name === "auto" ? localStorage.removeItem("gascoigne-theme") : localStorage.setItem("gascoigne-theme", name); } catch {}
        if (name === "auto") delete document.documentElement.dataset.theme; else document.documentElement.dataset.theme = name;
        theme.querySelectorAll("button").forEach((x) => x.setAttribute("aria-pressed", String(x === b)));
      });
      theme.append(b);
    }
    const brand = el("div", { class: "brand" }, el("div", { class: "moon" }), el("div", {}, el("h1", { text: "Gascoigne" }), el("div", { class: "subtitle", text: "Token savings for coding agents" })));
    brand.querySelector(".moon").innerHTML = moon; // a fixed literal, no data
    const ver = el("span", { class: "ver", title: "gascoigne version" });
    // The live dot follows the server: a health check now and every few seconds, as the dashboard's event stream does.
    const live = el("div", { class: "live" }, el("i"), el("span", { text: "Connecting…" }));
    const ping = () => fetch("/api/health").then((r) => r.json()).then((h) => {
      ver.textContent = h.version ? "v" + h.version : "";
      live.classList.add("on"); live.querySelector("span").textContent = "Live";
    }).catch(() => { live.classList.remove("on"); live.querySelector("span").textContent = "Reconnecting…"; });
    ping(); setInterval(ping, 5000);
    target.replaceWith(el("header", {}, brand, el("div", { class: "head-right" },
      ...links.map(([href, text, id]) => el("a", { class: "navlink", href, text, ...(id === current ? { "aria-current": "page" } : {}) })), ver, live, theme)),
      el("div", { class: "pagehead" }, el("h2", { text: title }), el("span", { class: "hint", text: subtitle })));
  };

  window.G = { el, human, usd, num, header, icon, button, setButton };
})();
