(function () {
  "use strict";
  const root = document.querySelector("[data-family-tree]"); if (!root) return;
  const get = name => root.querySelector("[data-tree-" + name + "]");
  const viewport = get("viewport"), world = get("world"), canvas = get("canvas"), cards = get("cards"), inspector = get("inspector");
  const ctx = canvas.getContext("2d"), status = get("status"), select = get("select"), search = get("search"), results = get("search-results");
  let page, tree, drawing, scale = 1, selected = 0, pending = 0, controller, drag, related = new Set();
  const elements = new Map();
  function element(tag, text, className) { const node = document.createElement(tag); if (text) node.textContent = text; if (className) node.className = className; return node; }
  function date(value) { if (!value) return ""; const parts = value.split("-"); return parts.reverse().join("."); }
  function dates(person) { return [person.birth_date && "* " + date(person.birth_date), person.death_date && "† " + date(person.death_date)].filter(Boolean); }
  function schedule() { if (!pending) pending = requestAnimationFrame(() => { pending = 0; render(); }); }
  function fitScale() { return Math.min(1, (viewport.clientWidth - 24) / drawing.width, (viewport.clientHeight - 24) / drawing.height); }
  function setScale(value, anchorX = viewport.clientWidth / 2, anchorY = viewport.clientHeight / 2) {
    if (!drawing) return;
    const x = (viewport.scrollLeft + anchorX) / scale, y = (viewport.scrollTop + anchorY) / scale;
    scale = Math.max(Math.min(0.12, fitScale()), Math.min(2.5, value));
    world.style.width = Math.ceil(drawing.width * scale) + "px"; world.style.height = Math.ceil(drawing.height * scale) + "px";
    viewport.scrollLeft = x * scale - anchorX; viewport.scrollTop = y * scale - anchorY;
    get("zoom").textContent = (scale < .01 ? (scale * 100).toFixed(2) : Math.round(scale * 100)) + " %"; schedule();
  }
  function fit() { setScale(fitScale()); viewport.scrollLeft = 0; viewport.scrollTop = 0; }
  function focusPerson(id) {
    const person = drawing.nodes.get(id); if (!person) return;
    choosePerson(id); setScale(1);
    viewport.scrollLeft = person.x + person.width / 2 - viewport.clientWidth / 2;
    viewport.scrollTop = person.y + person.height / 2 - viewport.clientHeight / 2;
    search.value = ""; results.hidden = true; schedule();
  }
  function relationLabel(edge, id) {
    if (edge.kind === "mother" || edge.kind === "father") return edge.to === id ? (edge.kind === "mother" ? "Mutter" : "Vater") : "Kind";
    if (edge.kind === "sibling") return "Geschwister";
    return edge.divorce_date ? "Geschiedene Ehe" : "Ehe";
  }
  function choosePerson(id) {
    selected = id; const person = drawing.nodes.get(id); if (!person) return;
    const relations = tree.relations.filter(edge => edge.from === id || edge.to === id);
    related = new Set([id]); relations.forEach(edge => { related.add(edge.from); related.add(edge.to); });
    inspector.replaceChildren();
    const portrait = element("img"); portrait.src = "/photos/faces/" + person.face_id + "/thumbnail"; portrait.alt = ""; portrait.width = 72; portrait.height = 72;
    inspector.append(portrait, element("p", tree.roots.includes(id) ? "Ausgangsperson" : "Stammdaten", "family-tree-eyebrow"), element("h2", person.name));
    const facts = element("dl", "", "family-tree-facts");
    [["Geboren", date(person.birth_date)], ["Gestorben", date(person.death_date)]].forEach(([label, value]) => { if (value) facts.append(element("dt", label), element("dd", value)); });
    if (facts.children.length) inspector.append(facts);
    if (person.tags.length) { const tags = element("div", "", "family-tree-tags"); person.tags.forEach(tag => tags.append(element("span", tag, "tag"))); inspector.append(tags); }
    const link = element("a", "Person und Fotos öffnen", "secondary-button"); link.href = "/photos/people/" + id; inspector.append(link);
    if (relations.length) inspector.append(element("h3", "Verbindungen"));
    relations.forEach(edge => {
      const other = drawing.nodes.get(edge.from === id ? edge.to : edge.from); if (!other) return;
      const button = element("button", "", "family-tree-relation"); button.type = "button";
      button.append(element("span", relationLabel(edge, id)), element("strong", other.name));
      if (edge.wedding_date) button.append(element("small", "Heirat: " + date(edge.wedding_date)));
      if (edge.divorce_date) button.append(element("small", "Scheidung: " + date(edge.divorce_date)));
      button.addEventListener("click", () => focusPerson(other.id)); inspector.append(button);
    });
    schedule();
  }
  function card(person) {
    const button = element("button", "", "family-tree-card"); button.type = "button"; button.dataset.treePerson = String(person.id);
    button.setAttribute("aria-label", person.name + (dates(person).length ? ", " + dates(person).join(", ") : ""));
    const heading = element("span", "", "family-tree-card-heading"), img = element("img"), name = element("strong", person.name);
    img.src = "/photos/faces/" + person.face_id + "/thumbnail"; img.alt = ""; img.loading = "lazy"; img.width = 48; img.height = 48;
    heading.append(img, name); button.append(heading);
    const details = element("span", "", "family-tree-card-dates"); dates(person).forEach(value => details.append(element("span", value))); button.append(details);
    if (tree.roots.includes(person.id)) { button.classList.add("is-root"); button.append(element("span", "Ausgangsperson", "family-tree-root-badge")); }
    button.addEventListener("click", () => choosePerson(person.id)); return button;
  }
  function render() {
    if (!drawing) return;
    const width = viewport.clientWidth, height = viewport.clientHeight, dpr = Math.min(2, window.devicePixelRatio || 1);
    if (canvas.width !== Math.round(width * dpr) || canvas.height !== Math.round(height * dpr)) { canvas.width = Math.round(width * dpr); canvas.height = Math.round(height * dpr); }
    canvas.style.width = width + "px"; canvas.style.height = height + "px";
    const left = viewport.scrollLeft, top = viewport.scrollTop; canvas.style.left = left + "px"; canvas.style.top = top + "px";
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0); ctx.clearRect(0, 0, width, height);
    const styles = getComputedStyle(root), colors = { mother: styles.getPropertyValue("--tree-parent").trim(), father: styles.getPropertyValue("--tree-parent").trim(), sibling: styles.getPropertyValue("--tree-sibling").trim(), marriage: styles.getPropertyValue("--tree-marriage").trim() };
    tree.relations.forEach(edge => {
      const from = drawing.nodes.get(edge.from), to = drawing.nodes.get(edge.to); if (!from || !to) return;
      const parent = edge.kind === "mother" || edge.kind === "father";
      let ax = (from.x + from.width / 2) * scale - left, ay = (from.y + (parent ? from.height : from.height / 2)) * scale - top;
      let bx = (to.x + to.width / 2) * scale - left, by = (to.y + (parent ? 0 : to.height / 2)) * scale - top;
      let lift = 0;
      if (!parent && from.y === to.y) {
        const distance = Math.abs(from.x - to.x), acrossCards = distance > from.width + 80;
        if (acrossCards) {
          // Distant siblings and former partners connect above their row instead
          // of drawing through the intervening people. Zoom scales the whole arc.
          ay = from.y * scale - top; by = to.y * scale - top;
          lift = Math.min(128, 64 + distance * .04) * scale;
        } else {
          ax = (from.x + (from.x < to.x ? from.width : 0)) * scale - left;
          bx = (to.x + (from.x < to.x ? 0 : to.width)) * scale - left;
          lift = Math.min(40, distance * .16) * scale;
        }
      }
      if (Math.max(ax, bx) < -80 || Math.min(ax, bx) > width + 80 || Math.max(ay, by) < -80 || Math.min(ay, by) - lift > height + 80) return;
      const highlighted = edge.from === selected || edge.to === selected;
      ctx.globalAlpha = selected && !highlighted ? .32 : .88; ctx.strokeStyle = colors[edge.kind]; ctx.lineWidth = highlighted ? 2.8 : 1.7;
      ctx.setLineDash(edge.divorce_date ? [7, 5] : edge.kind === "sibling" ? [3, 5] : []); ctx.beginPath(); ctx.moveTo(ax, ay);
      if (parent) { const middle = (ay + by) / 2; ctx.bezierCurveTo(ax, middle, bx, middle, bx, by); }
      else { ctx.bezierCurveTo(ax + (bx - ax) / 3, ay - lift, bx - (bx - ax) / 3, by - lift, bx, by); }
      ctx.stroke();
      if (parent && scale >= .45) { ctx.setLineDash([]); ctx.beginPath(); ctx.moveTo(bx - 4, by - 7); ctx.lineTo(bx, by - 1); ctx.lineTo(bx + 4, by - 7); ctx.stroke(); }
    });
    ctx.setLineDash([]); ctx.globalAlpha = 1;
    const visible = new Set();
    drawing.nodes.forEach(person => {
      const x = person.x * scale - left, y = person.y * scale - top, w = person.width * scale, h = person.height * scale;
      if (x + w < -50 || y + h < -50 || x > width + 50 || y > height + 50) return;
      if (scale < .45) {
        ctx.globalAlpha = selected && !related.has(person.id) ? .4 : 1;
        ctx.fillStyle = selected === person.id || tree.roots.includes(person.id) ? colors.mother : styles.getPropertyValue("--tree-mini").trim();
        ctx.fillRect(x, y, Math.max(w, 2), Math.max(h, 2));
        if (scale > .17) { ctx.fillStyle = styles.getPropertyValue("--tree-mini-text").trim(); ctx.font = "11px sans-serif"; ctx.fillText(person.name, x + 4, y + 16, w - 8); }
        return;
      }
      visible.add(person.id); let button = elements.get(person.id);
      if (!button) { button = card(person); elements.set(person.id, button); cards.append(button); }
      button.style.left = person.x * scale + "px"; button.style.top = person.y * scale + "px"; button.style.transform = "scale(" + scale + ")";
      button.classList.toggle("is-selected", person.id === selected); button.classList.toggle("is-dimmed", !!selected && !related.has(person.id)); button.setAttribute("aria-pressed", String(person.id === selected));
    });
    elements.forEach((button, id) => { if (!visible.has(id)) { button.remove(); elements.delete(id); } }); ctx.globalAlpha = 1;
  }
  function showTree(index) {
    tree = page.trees[index]; if (!tree) return;
    drawing = window.BearStackFamilyTreeLayout.layout(tree); selected = 0; related.clear(); elements.clear(); cards.replaceChildren(); search.value = ""; results.hidden = true;
    status.textContent = page.trees.length + (page.trees.length === 1 ? " Stammbaum" : " Stammbäume") + " · " + tree.people.length + (tree.people.length === 1 ? " Person · " : " Personen · ") + tree.relations.length + (tree.relations.length === 1 ? " Verbindung" : " Verbindungen");
    fit(); choosePerson(tree.roots[0]);
  }
  async function load() {
    if (controller) controller.abort(); controller = new AbortController(); const current = controller;
    const timer = setTimeout(() => current.abort(), 35000); get("retry").hidden = true; status.textContent = "Stammbäume werden geladen …";
    try {
      const response = await fetch("/photos/family-tree?format=json", { credentials: "same-origin", redirect: "error", headers: { Accept: "application/json" }, signal: current.signal });
      const data = await response.json(); if (!response.ok) throw new Error(data.error || "Stammbäume konnten nicht geladen werden.");
      if (current !== controller) return; page = data;
      if (!page.trees.length) { status.textContent = "Für die gewählten Ausgangspersonen sind derzeit keine freigegebenen Personendaten verfügbar."; return; }
      select.replaceChildren(); page.trees.forEach((item, index) => { const names = item.people.filter(person => item.roots.includes(person.id)).map(person => person.name); const option = element("option", names.slice(0, 3).join(" · ") + (names.length > 3 ? " + " + (names.length - 3) : "")); option.value = String(index); select.append(option); });
      ["controls", "layout", "legend"].forEach(name => { get(name).hidden = false; }); showTree(0);
    } catch (error) { if (current === controller) { status.textContent = error.name === "AbortError" ? "Das Laden dauert zu lange. Bitte erneut versuchen." : error.message; get("retry").hidden = false; } }
    finally { clearTimeout(timer); }
  }
  select.addEventListener("change", () => showTree(Number(select.value)));
  get("zoom-in").addEventListener("click", () => setScale(scale * 1.25)); get("zoom-out").addEventListener("click", () => setScale(scale / 1.25)); get("fit").addEventListener("click", fit); get("reset").addEventListener("click", () => setScale(1)); get("retry").addEventListener("click", load);
  viewport.addEventListener("scroll", schedule, { passive: true });
  viewport.addEventListener("wheel", event => { if (event.ctrlKey || event.metaKey) { event.preventDefault(); const rect = viewport.getBoundingClientRect(); setScale(scale * Math.exp(-event.deltaY * .006), event.clientX - rect.left, event.clientY - rect.top); } }, { passive: false });
  viewport.addEventListener("keydown", event => { if (event.target !== viewport) return; if (["+", "=", "-", "0"].includes(event.key)) { event.preventDefault(); if (event.key === "0") fit(); else setScale(scale * (event.key === "-" ? .8 : 1.25)); } });
  viewport.addEventListener("pointerdown", event => { if (event.pointerType !== "mouse" || event.button !== 0 || event.target.closest("button")) return; drag = { id: event.pointerId, x: event.clientX, y: event.clientY, left: viewport.scrollLeft, top: viewport.scrollTop, moved: false }; viewport.setPointerCapture(event.pointerId); });
  viewport.addEventListener("pointermove", event => { if (!drag || drag.id !== event.pointerId) return; const dx = event.clientX - drag.x, dy = event.clientY - drag.y; if (Math.abs(dx) + Math.abs(dy) > 4) drag.moved = true; viewport.scrollLeft = drag.left - dx; viewport.scrollTop = drag.top - dy; });
  viewport.addEventListener("pointerup", event => {
    if (drag && drag.id === event.pointerId) { const moved = drag.moved; drag = null; if (viewport.hasPointerCapture(event.pointerId)) viewport.releasePointerCapture(event.pointerId); if (moved) return; }
    if (!drawing || scale >= .45 || event.target.closest("button")) return;
    const rect = viewport.getBoundingClientRect(), x = (event.clientX - rect.left + viewport.scrollLeft) / scale, y = (event.clientY - rect.top + viewport.scrollTop) / scale;
    for (const person of drawing.nodes.values()) if (x >= person.x && x <= person.x + person.width && y >= person.y && y <= person.y + person.height) { focusPerson(person.id); break; }
  });
  viewport.addEventListener("pointercancel", () => { drag = null; });
  search.addEventListener("input", () => {
    results.replaceChildren(); const query = search.value.trim().toLocaleLowerCase("de"); results.hidden = !query; if (!query || !drawing) return;
    let count = 0; for (const person of drawing.nodes.values()) { if (!person.name.toLocaleLowerCase("de").includes(query)) continue; const li = element("li"), button = element("button", person.name); button.type = "button"; button.addEventListener("click", () => focusPerson(person.id)); li.append(button); results.append(li); if (++count === 20) break; }
    if (!count) results.append(element("li", "Keine Person gefunden."));
  });
  search.addEventListener("keydown", event => { if (event.key === "Escape") results.hidden = true; if (event.key === "ArrowDown") { const button = results.querySelector("button"); if (button) { event.preventDefault(); button.focus(); } } if (event.key === "Enter") { const button = results.querySelector("button"); if (button) { event.preventDefault(); button.click(); } } });
  const observer = new ResizeObserver(schedule); observer.observe(viewport);
  const themeObserver = new MutationObserver(schedule); themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme", "class", "style"] });
  window.addEventListener("pagehide", () => { if (controller) controller.abort(); observer.disconnect(); themeObserver.disconnect(); if (pending) cancelAnimationFrame(pending); });
  load();
}());
