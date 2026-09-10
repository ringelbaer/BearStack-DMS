(function () {
  "use strict";
  var root = document.querySelector("[data-person-detail]");
  if (!root) return;
  var grid = root.querySelector("[data-photo-gallery]");
  var status = root.querySelector("[data-detail-status]");
  var retry = root.querySelector("[data-detail-retry]");
  var groupButton = document.querySelector("[data-detail-edit-group]");
  var selection = root.querySelector("[data-detail-selection]");
  var selectAll = root.querySelector("[data-detail-select-all]");
  var display = root.querySelector("[data-detail-display]");
  var size = display.querySelector("[data-detail-size]");
  var paths = display.querySelector("[data-detail-paths]");
  var storageKey = "bearstack.people.detailDisplay:" + root.dataset.personUser;
  var busy = false, uncertain = false, mode = "group", editingFaces = [], retryTarget = null;
  function cards() { return Array.from(grid.querySelectorAll("[data-detail-face]")); }
  function selected() { return cards().filter(function (card) { return card.querySelector('[name="face_id"]').checked; }); }
  function updateControls() {
    var blocked = busy || uncertain;
    root.setAttribute("aria-busy", String(busy));
    root.querySelectorAll("button, input, select").forEach(function (control) {
      if (!control.closest("dialog")) control.disabled = blocked;
    });
    retry.disabled = busy;
    if (groupButton) groupButton.disabled = blocked || !cards().length;
    if (selection) {
      var count = selected().length;
      selection.hidden = count === 0;
      document.body.classList.toggle("has-person-selection", count > 0);
      selection.querySelector("[data-detail-selection-count]").textContent = count + " ausgewählt";
      selectAll.textContent = count === cards().length && count ? "Auswahl aufheben" : "Alle auswählen";
    }
  }
  function setBusy(value) { busy = value; updateControls(); }
  function configureDisplay() {
    root.dataset.size = size.value; root.dataset.showPaths = String(paths.checked);
  }
  try {
    var saved = JSON.parse(localStorage.getItem(storageKey));
    if (saved) { if (["s","m","l"].includes(saved.size)) size.value = saved.size; paths.checked = saved.paths === true; }
  } catch (_) {}
  configureDisplay(); display.hidden = false;
  display.addEventListener("change", function () {
    configureDisplay();
    try { localStorage.setItem(storageKey, JSON.stringify({ size: size.value, paths: paths.checked })); } catch (_) {}
  });
  display.addEventListener("keydown", function (event) { if (event.key === "Escape") { display.open = false; display.querySelector("summary").focus(); } });

  async function refresh(target) {
    retryTarget = target || null;
    var address = new URL(location.href);
    if (target) { address.pathname = "/photos/people/" + encodeURIComponent(target); address.search = ""; }
    var response = await fetch(address, { credentials: "same-origin", redirect: "error", cache: "no-store", headers: { Accept: "text/html" } });
    if (response.status === 404 && !target) {
      // A completely moved/ignored group no longer has an active detail page.
      grid.replaceChildren(); groupButton.disabled = true;
      document.querySelector("[data-detail-count]").textContent = "Keine aktiven Gesichter mehr";
      document.querySelector(".people-pagination").hidden = true;
      grid.dispatchEvent(new CustomEvent("photo-gallery-updated"));
      uncertain = false; retry.hidden = true; return;
    }
    if (!response.ok) throw new Error("Ansicht konnte nicht aktualisiert werden.");
    var view = new DOMParser().parseFromString(await response.text(), "text/html");
    var updated = view.querySelector("[data-person-detail]");
    if (!updated) throw new Error("Ansicht konnte nicht aktualisiert werden. Bitte Anmeldung prüfen.");
    var old = new Map(cards().map(function (card) { return [card.dataset.detailFace, card]; }));
    var next = Array.from(updated.querySelectorAll("[data-detail-face]")).map(function (card) {
      var existing = old.get(card.dataset.detailFace);
      if (!existing) return document.importNode(card, true);
      // Retain loaded photo/thumbnail nodes, updating only metadata and actions.
      Array.from(card.attributes).forEach(function (attribute) { existing.setAttribute(attribute.name, attribute.value); });
      existing.querySelector(".person-face-actions").replaceWith(document.importNode(card.querySelector(".person-face-actions"), true));
      return existing;
    });
    grid.replaceChildren.apply(grid, next);
    root.dataset.personId = updated.dataset.personId; root.dataset.personName = updated.dataset.personName;
    document.querySelector("[data-detail-title]").textContent = updated.dataset.personName || "Unbenannt";
    document.querySelector("[data-detail-count]").textContent = view.querySelector("[data-detail-count]").textContent;
    document.querySelector(".people-pagination").replaceWith(document.importNode(view.querySelector(".people-pagination"), true));
    root.querySelectorAll("[data-detail-edit-face], [data-detail-ignore-face]").forEach(function (button) { button.hidden = false; });
    grid.dispatchEvent(new CustomEvent("photo-gallery-updated"));
    if (target) history.replaceState(null, "", address.pathname);
    uncertain = false; retry.hidden = true; retryTarget = null;
  }
  function failed(error) { uncertain = true; retry.hidden = false; status.textContent = error.message + " Bitte die Ansicht erneut laden."; }
  retry.addEventListener("click", async function () {
    if (busy) return;
    setBusy(true);
    try { await refresh(retryTarget); status.textContent = "Ansicht aktualisiert. Bitte die Auswahl erneut prüfen."; }
    catch (error) { failed(error); }
    finally { setBusy(false); }
  });
  if (!groupButton) return;
  document.addEventListener("photo-people-updated", async function (event) {
    if (!event.detail.ids.includes(root.dataset.personId) || busy) return;
    setBusy(true);
    try { await refresh(event.detail.target); }
    catch (error) { failed(error); }
    finally { setBusy(false); }
  });
  groupButton.hidden = false; selectAll.hidden = false;
  root.querySelectorAll("[data-detail-edit-face], [data-detail-ignore-face]").forEach(function (button) { button.hidden = false; });
  var editor = window.BearStackPersonDialog.bind({
    surface: grid, status: status,
    getPreviewCard: function () { return mode === "faces" ? editingFaces[0] : null; },
    onSaveError: function (error) { failed(error); return true; },
    isBusy: function () { return busy || uncertain; }, onBusy: setBusy,
    configureDialog: function (context) {
      if (mode === "group") {
        context.dialog.querySelector("#person-dialog-title").textContent = "Person benennen oder zuordnen";
      } else {
        context.form.dataset.personFaceSelection = "";
        context.dialog.querySelector("#person-dialog-title").textContent = editingFaces.length === 1 ? "Gesicht benennen oder zuordnen" : editingFaces.length + " Gesichter benennen oder zuordnen";
        context.dialog.querySelector("#overview-person-hint").textContent = "Nur die ausgewählten Gesichter werden verschoben. Einen neuen Namen eingeben, eine vorhandene Person wählen oder ohne Namen als neue Gruppe abtrennen.";
        context.form.dispatchEvent(new CustomEvent("person-picker-reset", { detail: { name: "" } }));
      }
    },
    getSaveRequest: function (request) {
      if (mode === "group") return request;
      var body = new URLSearchParams({ action: "move", target: request.body.get("target") || "0", name: request.body.get("target") && request.body.get("target") !== "0" ? "" : request.body.get("name") });
      editingFaces.forEach(function (card) { body.append("face_id", card.dataset.detailFace); });
      return { action: "/photos/faces/edit", body: body };
    },
    getIgnoreRequest: function () {
      if (mode === "group") return null;
      var body = new URLSearchParams({ action: "ignore" });
      editingFaces.forEach(function (card) { body.append("face_id", card.dataset.detailFace); });
      return { action: "/photos/faces/edit", body: body };
    },
    savedMessage: function (ignoring) { return ignoring ? "Ausgewählte Gesichter ignoriert." : mode === "group" ? "Person gespeichert." : "Gesichtszuordnung gespeichert."; },
    onSave: async function (_, request) {
      try { await refresh(mode === "group" && request.action.endsWith("/merge") ? request.body.get("target") : null); }
      catch (error) { failed(error); throw error; }
    }
  });
  function openSelection(choices, opener) {
    if (busy || uncertain || !choices.length) return;
    mode = "faces"; editingFaces = choices;
    editor.open([root.dataset.personId], opener);
  }
  groupButton.addEventListener("click", function () { mode = "group"; editingFaces = []; editor.open([root.dataset.personId], groupButton); });
  root.addEventListener("change", function (event) { if (event.target.matches('[name="face_id"]')) updateControls(); });
  selectAll.addEventListener("click", function () {
    var check = selected().length !== cards().length;
    cards().forEach(function (card) { card.querySelector('[name="face_id"]').checked = check; }); updateControls();
  });
  selection.querySelector("[data-detail-clear]").addEventListener("click", function () { cards().forEach(function (card) { card.querySelector('[name="face_id"]').checked = false; }); updateControls(); });
  selection.querySelector("[data-detail-edit-selection]").addEventListener("click", function (event) { openSelection(selected(), event.currentTarget); });
  async function ignore(choices) {
    if (busy || uncertain || !choices.length) return;
    var body = new URLSearchParams({ action: "ignore" });
    choices.forEach(function (card) { body.append("face_id", card.dataset.detailFace); });
    setBusy(true);
    try {
      var response = await fetch("/photos/faces/edit", { method: "POST", credentials: "same-origin", redirect: "error", headers: { Accept: "application/json" }, body: body });
      var result = await response.json();
      if (!response.ok || result.ok !== true) throw new Error("Ignorieren konnte nicht bestätigt werden.");
      await refresh(); status.textContent = choices.length === 1 ? "Gesicht ignoriert." : "Ausgewählte Gesichter ignoriert.";
    } catch (error) { failed(error); }
    finally { setBusy(false); }
  }
  selection.querySelector("[data-detail-ignore-selection]").addEventListener("click", function () { ignore(selected()); });
  grid.addEventListener("click", async function (event) {
    var card = event.target.closest("[data-detail-face]"); if (!card) return;
    if (event.target.closest("[data-detail-edit-face]")) openSelection([card], event.target.closest("button"));
    if (event.target.closest("[data-detail-ignore-face]")) ignore([card]);
    var favorite = event.target.closest("[data-detail-favorite]");
    if (!favorite) return;
    event.preventDefault(); if (busy || uncertain) return;
    setBusy(true); status.textContent = "";
    try {
      var response = await fetch("/api/photos/labeling/v1/faces/" + encodeURIComponent(card.dataset.detailFace) + "/favorite", {
        method: "PUT", credentials: "same-origin", redirect: "error", headers: { "Content-Type": "application/json", Accept: "application/json" },
        body: JSON.stringify({ person_id: Number(root.dataset.personId), favorite: favorite.getAttribute("aria-pressed") !== "true" })
      });
      var result = await response.json();
      if (!response.ok || typeof result.favorite !== "boolean") throw new Error(result.error || "Favorisierung konnte nicht bestätigt werden.");
      favorite.setAttribute("aria-pressed", String(result.favorite)); favorite.value = result.favorite ? "0" : "1";
      favorite.title = result.favorite ? "Favorisierung aufheben" : "Als Vergleichsgesicht favorisieren";
      favorite.setAttribute("aria-label", favorite.title);
      status.textContent = result.favorite ? "Gesicht als Vergleichsbild favorisiert." : "Favorisierung aufgehoben.";
    } catch (error) { status.textContent = error.message; }
    finally { setBusy(false); }
  });
  updateControls();
}());
