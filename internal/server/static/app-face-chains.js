(function () {
  "use strict";
  var surface = document.querySelector("[data-face-chains]");
  if (!surface) return;
  var find = function (name) { return surface.querySelector("[data-chain-" + name + "]"); };
  var options = find("options"), hopsInput = options.elements.hops, grid = find("grid"), content = find("content");
  var status = find("status"), retry = find("retry"), recover = find("recover"), assign = find("assign"), stop = find("stop");
  var chain = null, page = null, omitted = new Set(), skipped = new Set(), after = 0, hops = 2;
  var loading = false, saving = false, pending = null, controller = null, retryAction = null;
  var storageKey = "bearstack.face-chain.pending:" + surface.dataset.chainUser;
  function persist() { try { if (pending) sessionStorage.setItem(storageKey, JSON.stringify(pending)); else sessionStorage.removeItem(storageKey); } catch (_) {} }
  function selectedCount() { return page ? page.total - omitted.size : 0; }
  function sync() {
    var locked = loading || saving || !!pending;
    surface.setAttribute("aria-busy", String(loading || saving));
    surface.querySelectorAll("button,input").forEach(function (control) { control.disabled = locked; });
    stop.disabled = !loading; recover.disabled = loading || saving; retry.disabled = loading || saving || !!pending;
    assign.disabled = locked || !page || selectedCount() <= 0;
    find("previous").disabled = locked || !page || page.page <= 1;
    find("next-page").disabled = locked || !page || page.page >= page.pages;
    find("selected").textContent = selectedCount() + " Gesichter ausgewählt (über alle Seiten)";
    grid.querySelectorAll("input[type=checkbox]").forEach(function (box) { box.checked = !omitted.has(Number(box.dataset.faceId)); });
  }
  function fail(error, action) {
    status.textContent = error.name === "AbortError" ? "Suche angehalten oder Zeitlimit erreicht. Du kannst es erneut versuchen." : error.message;
    retryAction = action; retry.hidden = false;
  }
  async function request(url, body, method) {
    controller = new AbortController();
    var current = controller, timer = setTimeout(function () { current.abort(); }, 35000);
    try {
      var response = await fetch(url, { method: method || "POST", credentials: "same-origin", redirect: "error", cache: "no-store", signal: current.signal,
        headers: { Accept: "application/json", "Content-Type": "application/json" }, body: body === undefined ? undefined : JSON.stringify(body) });
      var result = await response.json();
      if (!response.ok) { var error = new Error(result.error || "Anfrage fehlgeschlagen (HTTP " + response.status + ")."); error.status = response.status; throw error; }
      return result;
    } finally { clearTimeout(timer); if (controller === current) controller = null; }
  }
  function selection() { return { dataset: chain.dataset, groups: chain.groups.map(function (g) { return { id: g.id, revision: g.revision }; }) }; }
  function render(result) {
    page = result; grid.replaceChildren();
    result.faces.forEach(function (face) {
      var card = document.createElement("article"); card.className = "person-card person-face-card";
      card.dataset.personId = String(face.person_id); card.dataset.personName = ""; card.dataset.faceId = String(face.id); card.dataset.displayPath = face.display_path;
      ["x", "y", "width", "height"].forEach(function (key) { card.dataset[key] = String(face[key]); });
      var label = document.createElement("label"), box = document.createElement("input"); box.type = "checkbox"; box.dataset.faceId = String(face.id);
      box.checked = !omitted.has(face.id); box.setAttribute("aria-label", "Gesicht auswählen: " + face.display_path); label.append(box);
      var image = document.createElement("img"); image.loading = "lazy"; image.width = 160; image.height = 160; image.alt = "Gesicht aus " + face.folder_name;
      image.src = "/photos/faces/" + face.id + "/thumbnail";
      var folder = document.createElement("strong"); folder.textContent = face.folder_name; folder.title = face.display_path;
      var group = chain.groups.find(function (g) { return g.id === face.person_id; });
      var depth = document.createElement("span"); depth.className = "muted";
      depth.textContent = group.depth === 0 ? "Ausgangsgruppe" : group.depth + (group.depth === 1 ? " Sprung" : " Sprünge");
      card.append(label, image, folder, depth); grid.append(card);
    });
    find("summary").textContent = chain.groups.length + " verbundene Gruppen · " + result.total + " Gesichter · Mindestähnlichkeit " + chain.similarity.toLocaleString("de-DE", { maximumFractionDigits: 2 });
    find("page").textContent = "Seite " + result.page + " von " + result.pages;
    find("pagination").hidden = result.pages <= 1;
    content.hidden = false;
  }
  async function fetchPage(number) { render(await request("/photos/people/chains/faces", Object.assign(selection(), { page: number }))); }
  async function loadPage(number) {
    if (loading || saving || pending) return;
    loading = true; retry.hidden = true; sync();
    try { await fetchPage(number); status.textContent = ""; }
    catch (error) {
      if (error.status === 409 || error.status === 404) { content.hidden = true; page = null; chain = null; fail(new Error("Die Gruppen wurden geändert. Bitte die Kette erneut laden und prüfen."), searchNext); }
      else fail(error, function () { loadPage(number); });
    } finally { loading = false; sync(); }
  }
  async function searchNext() {
    if (loading || saving || pending) return;
    loading = true; chain = null; page = null; omitted.clear(); content.hidden = true; retry.hidden = true; stop.hidden = false; sync();
    try {
      while (true) {
        if (skipped.size > 10000) throw new Error("Dieser Durchlauf hat 10.000 Gruppen erreicht. Bitte einen neuen Durchlauf starten.");
        status.textContent = "Passende Gesichtskette wird gesucht …";
        var result = await request("/photos/people/chains/search", { hops: hops, after: after, excluded_groups: Array.from(skipped) });
        if (result.groups.length) { chain = result; await fetchPage(1); status.textContent = "Prüfe die Auswahl und entferne unpassende Gesichter."; break; }
        after = result.after;
        if (!result.has_more) { status.textContent = "Keine weiteren Gesichtsketten. Ein neuer Durchlauf berücksichtigt übersprungene Gruppen wieder."; break; }
      }
    } catch (error) { fail(error, searchNext); }
    finally { loading = false; stop.hidden = true; sync(); }
  }
  function advance() {
    chain.groups.forEach(function (g) { skipped.add(g.id); }); after = chain.after;
    chain = null; page = null; omitted.clear(); content.hidden = true;
  }
  function verifyReceipt(receipt) {
    if (!receipt || receipt.operation_id !== pending.payload.operation_id || receipt.action !== "assign_chain" || receipt.source_id !== pending.payload.groups[0].id || !(receipt.faces > 0)) throw new Error("Ungültige Speicherbestätigung. Bitte Speicherung prüfen.");
  }
  function completeSave(receipt) {
    verifyReceipt(receipt);
    chain = pending.chain; after = pending.after; hops = pending.hops; skipped = new Set(pending.skipped);
    advance(); pending = null; persist(); recover.hidden = true;
  }
  var editor = window.BearStackPersonDialog.bind({
    surface: surface, status: status, requestTimeoutMS: 30000,
    isBusy: function () { return loading || saving || !!pending; },
    onBusy: function (busy) { saving = busy; sync(); if (!busy && !pending && !chain) searchNext(); },
    getPreviewCard: function () { var box = grid.querySelector("input:checked"); return box ? box.closest("article") : grid.querySelector("article"); },
    configureDialog: function (state) {
      state.dialog.querySelector("#person-dialog-title").textContent = selectedCount() + " Gesichter zuordnen";
      state.dialog.querySelector("#overview-person-hint").textContent = "Nur die ausgewählten Gesichter dieser Kette werden einer vorhandenen Person oder einem neuen Namen zugeordnet.";
      state.dialog.querySelector("[data-person-dialog-ignore]").hidden = true;
      state.form.dataset.personFaceSelection = "1";
      state.form.querySelector("[data-person-search]").required = true;
    },
    getSaveRequest: function (state) {
      if (!chain || selectedCount() <= 0 || pending) return null;
      var target = Number(state.body.get("target") || 0), name = (state.body.get("name") || "").trim();
      if (!name) return null;
      var targetInput = document.querySelector("[data-person-dialog] [data-person-target]");
      var id = Array.from(crypto.getRandomValues(new Uint8Array(16)), function (b) { return b.toString(16).padStart(2, "0"); }).join("");
      var payload = Object.assign(selection(), { operation_id: id, excluded_faces: Array.from(omitted), target_id: target, target_revision: target ? Number(targetInput.dataset.revision || 0) : 0, target_name: target ? targetInput.dataset.name : "", name: target ? "" : name });
      pending = { payload: payload, chain: chain, after: after, hops: hops, skipped: Array.from(skipped) }; persist();
      return { action: "/photos/people/chains/assign", body: new URLSearchParams({ payload: JSON.stringify(payload) }) };
    },
    onSave: async function (_, state) { completeSave(state.result.receipt); },
    savedMessage: function () { return "Auswahl gespeichert. Nächste Kette wird gesucht …"; },
    onSaveError: function (error) {
      if ([400, 403, 404, 409].includes(error.status)) {
        pending = null; persist(); content.hidden = true; page = null;
        fail(new Error("Die Zuordnung wurde nicht gespeichert. Bitte den Dialog schließen und die Kette erneut prüfen."), searchNext);
      } else { recover.hidden = false; status.textContent = "Speicherung nicht bestätigt. Bitte den Dialog schließen und zuerst die Speicherung prüfen."; }
      return true;
    }
  });
  assign.addEventListener("click", async function () {
    if (loading || saving || pending || !page || selectedCount() <= 0) return;
    var checked = grid.querySelector("input:checked"), currentPage = page.page, pages = page.pages;
    // The usual naming dialog must preview a selected face, including when the
    // entire current page was deselected but later pages are still selected.
    for (var number = 1; !checked && number <= pages; number++) {
      if (number === currentPage) continue;
      await loadPage(number);
      if (!retry.hidden || !page || !chain) return;
      checked = grid.querySelector("input:checked");
    }
    if (checked) { var card = checked.closest("article"); editor.open([card.dataset.personId], assign); }
  });
  grid.addEventListener("change", function (event) {
    var id = Number(event.target.dataset.faceId); if (!id || loading || saving || pending) return;
    if (event.target.checked) omitted.delete(id);
    else if (omitted.size < 10000) omitted.add(id);
    else { event.target.checked = true; status.textContent = "Höchstens 10.000 Gesichter können pro Kette abgewählt werden. Bitte weniger Sprünge wählen."; }
    sync();
  });
  find("select-page").addEventListener("click", function () { page.faces.forEach(function (f) { omitted.delete(f.id); }); sync(); });
  find("deselect-page").addEventListener("click", function () {
    if (omitted.size + page.faces.filter(function (f) { return !omitted.has(f.id); }).length > 10000) { status.textContent = "Höchstens 10.000 Gesichter können abgewählt werden."; return; }
    page.faces.forEach(function (f) { omitted.add(f.id); }); sync();
  });
  find("previous").addEventListener("click", function () { loadPage(page.page - 1); });
  find("next-page").addEventListener("click", function () { loadPage(page.page + 1); });
  find("skip").addEventListener("click", function () { advance(); searchNext(); });
  options.addEventListener("submit", function (event) { event.preventDefault(); if (loading || saving || pending || !options.reportValidity()) return; hops = Number(hopsInput.value); after = 0; skipped.clear(); searchNext(); });
  stop.addEventListener("click", function () { if (controller) controller.abort(); });
  retry.addEventListener("click", function () { if (retryAction) retryAction(); });
  recover.addEventListener("click", async function () {
    if (!pending || loading || saving) return; loading = true; sync();
    try {
      var receipt;
      try { receipt = await request("/api/photos/labeling/v1/actions/" + encodeURIComponent(pending.payload.operation_id) + "?dataset=" + encodeURIComponent(pending.payload.dataset), undefined, "GET"); }
      catch (error) { if (error.status !== 404) throw error; receipt = (await request("/photos/people/chains/assign", pending.payload)).receipt; }
      completeSave(receipt); status.textContent = "Speicherung bestätigt.";
    } catch (error) {
      if ([400, 403, 404, 409].includes(error.status)) { pending = null; persist(); recover.hidden = true; fail(new Error("Keine Zuordnung gespeichert. Bitte die Kette erneut prüfen."), searchNext); }
      else status.textContent = "Speicherung weiterhin unbestätigt. Bitte erneut prüfen.";
    } finally { loading = false; sync(); }
    if (!pending && !chain && retry.hidden) searchNext();
  });
  window.addEventListener("pagehide", function () { if (controller) controller.abort(); });
  try { var stored = JSON.parse(sessionStorage.getItem(storageKey)); if (stored && stored.payload && stored.chain && Array.isArray(stored.skipped)) pending = stored; } catch (_) {}
  if (pending) { recover.hidden = false; status.textContent = "Eine frühere Speicherung ist noch nicht bestätigt. Bitte zuerst die Speicherung prüfen."; sync(); }
  else searchNext();
}());
