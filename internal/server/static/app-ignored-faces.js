(function () {
  "use strict";
  var grid = document.querySelector("[data-ignored-faces]");
  if (!grid || !window.BearStackPersonDialog) return;
  var status = document.querySelector("[data-ignored-status]");
  var retry = document.querySelector("[data-ignored-retry]");
  var busy = false, uncertain = false, activeCard, editingCards = [], selectionControls;
  var selection = document.querySelector("[data-ignored-selection]");
  function selected() { return Array.from(grid.querySelectorAll("[data-ignored-select]:checked")).map(function (input) { return input.closest("[data-ignored-face]"); }); }
  function updateSelection() {
    var count = selected().length;
    selection.hidden = !count;
    selection.querySelector("[data-ignored-selection-count]").textContent = count + (count === 1 ? " Gesicht" : " Gesichter") + " auf dieser Seite";
    selection.querySelectorAll("button").forEach(function (button) { button.disabled = busy || uncertain; });
    if (selectionControls) selectionControls.update();
  }
  function setBusy(value) {
    busy = value;
    grid.setAttribute("aria-busy", String(value));
    grid.querySelectorAll("button").forEach(function (button) { button.disabled = busy || uncertain; });
    grid.querySelectorAll("[data-ignored-edit]").forEach(function (button) { button.hidden = false; });
    retry.disabled = busy;
    updateSelection();
  }
  function failed(error) {
    uncertain = true; retry.hidden = false;
    status.textContent = error.message + " Bitte die Ansicht erneut laden und den aktuellen Zustand prüfen.";
  }
  async function refresh(signal) {
    var controller = signal ? null : new AbortController();
    var timer = controller ? setTimeout(function () { controller.abort(); }, 20000) : null;
    signal = signal || controller.signal;
    try {
      await refreshView(signal);
    } finally { if (timer !== null) clearTimeout(timer); }
  }
  async function refreshView(signal) {
    var response = await fetch(location.href, { credentials: "same-origin", redirect: "error", cache: "no-store", signal: signal, headers: { Accept: "text/html" } });
    if (!response.ok) throw new Error("Ignorierte Gesichter konnten nicht geladen werden.");
    var view = new DOMParser().parseFromString(await response.text(), "text/html");
    var updated = view.querySelector("[data-ignored-faces]");
    var pagination = view.querySelector(".people-pagination");
    if (!updated || !pagination) throw new Error("Ansicht konnte nicht aktualisiert werden. Bitte Anmeldung prüfen.");
    var existing = new Map(Array.from(grid.querySelectorAll("[data-ignored-face]")).map(function (card) { return [card.dataset.ignoredFace, card]; }));
    // At most one page (60 faces); keep already loaded thumbnails for survivors.
    grid.replaceChildren.apply(grid, Array.from(updated.children).map(function (card) {
      var old = existing.get(card.dataset.ignoredFace);
      if (old && old.dataset.personId === card.dataset.personId && old.dataset.personName === card.dataset.personName) return old;
      return document.importNode(card, true);
    }));
    document.querySelector(".people-pagination").replaceWith(document.importNode(pagination, true));
    var address = new URL(location.href);
    address.searchParams.set("page", pagination.dataset.peoplePage);
    history.replaceState(history.state, "", address);
    document.dispatchEvent(new CustomEvent("people-page-updated", { detail: Number(pagination.dataset.peoplePage) }));
    grid.querySelectorAll("[data-ignored-select]").forEach(function (input) { input.checked = false; });
    uncertain = false; retry.hidden = true;
  }
  var editor = window.BearStackPersonDialog.bind({
    surface: grid, status: status, requestTimeoutMS: 20000,
    isBusy: function () { return busy || uncertain; }, onBusy: setBusy,
    getPreviewCard: function () { return activeCard; },
    configureDialog: function (context) {
      context.form.dataset.personRestore = "";
      // An ignored portrait cannot be matched until restored; name lookup and
      // the original preview remain usable without changing the face first.
      context.form.dataset.personFaceId = "";
      context.form.dataset.personExclude = "";
      context.form.querySelector("[data-person-face-match]").hidden = true;
      context.form.querySelector("[data-person-dialog-ignore]").hidden = true;
      context.dialog.querySelector("#person-dialog-title").textContent = editingCards.length + (editingCards.length === 1 ? " Gesicht" : " Gesichter") + " zuordnen und wiederherstellen";
      context.dialog.querySelector("#overview-person-hint").textContent = (editingCards.length === 1 ? "Nur dieses Gesicht wird wiederhergestellt." : "Nur diese " + editingCards.length + " Gesichter werden gemeinsam wiederhergestellt.") + " Einen neuen Namen eingeben oder eine vorhandene Person auswählen.";
      context.form.dispatchEvent(new CustomEvent("person-picker-reset", { detail: { name: activeCard.dataset.personName || "" } }));
    },
    getSaveRequest: function (request) {
      var target = request.body.get("target") || "0";
      var body = new URLSearchParams({ action: "restore", ignored: "1", target: target, name: target === "0" ? request.body.get("name") : "" });
      editingCards.forEach(function (card) { body.append("face_id", card.dataset.ignoredFace); });
      return { action: "/photos/faces/edit", body: body };
    },
    onSave: function () { return refresh(); },
    savedMessage: function () { return editingCards.length + (editingCards.length === 1 ? " Gesicht" : " Gesichter") + " benannt oder zugeordnet und wiederhergestellt."; },
    onSaveError: function (error) { failed(error); return true; }
  });
  grid.addEventListener("click", function (event) {
    var button = event.target.closest("[data-ignored-edit]");
    if (!button || busy || uncertain) return;
    activeCard = button.closest("[data-ignored-face]");
    editingCards = [activeCard];
    editor.open([activeCard.dataset.personId], button);
  });
  async function restore(cards) {
    if (busy || uncertain || !cards.length) return;
    var body = new URLSearchParams({ action: "restore", restore_individually: "1", ignored: "1", target: "0", name: "" });
    cards.forEach(function (card) { body.append("face_id", card.dataset.ignoredFace); });
    var controller = new AbortController(), timer = setTimeout(function () { controller.abort(); }, 20000);
    var restored = false;
    setBusy(true); status.textContent = cards.length + (cards.length === 1 ? " Gesicht wird" : " Gesichter werden") + " als unbenannt wiederhergestellt …";
    try {
      var response = await fetch("/photos/faces/edit", { method: "POST", credentials: "same-origin", redirect: "error", headers: { Accept: "application/json" }, body: body, signal: controller.signal });
      if (!response.ok || (await response.json()).ok !== true) throw new Error("Wiederherstellen konnte nicht bestätigt werden.");
      await refresh(controller.signal); status.textContent = cards.length + (cards.length === 1 ? " Gesicht" : " Gesichter") + " als unbenannt wiederhergestellt.";
      restored = true;
    } catch (error) { failed(error); }
    finally {
      clearTimeout(timer); setBusy(false);
      if (restored) document.querySelector('[data-people-selection-mode]').focus({ preventScroll: true });
    }
  }
  grid.addEventListener("submit", function (event) {
    var form = event.target.closest(".ignored-face-form");
    if (form) { event.preventDefault(); restore([form.closest("[data-ignored-face]")]); }
  });
  selectionControls = window.BearStackPeopleControls.bindSelection({
    grid: grid, button: document.querySelector("[data-people-selection-mode]"), all: document.querySelector("[data-people-select-all]"), clear: selection.querySelector("[data-ignored-clear]"),
    input: "[data-ignored-select]", card: "[data-ignored-face]", open: "[data-ignored-card-open]",
    blocked: function () { return busy || uncertain; }, changed: updateSelection
  });
  selection.querySelector("[data-ignored-restore]").addEventListener("click", function () { restore(selected()); });
  selection.querySelector("[data-ignored-assign]").addEventListener("click", function (event) {
    if (busy || uncertain) return;
    editingCards = selected(); if (!editingCards.length) return;
    activeCard = editingCards[0]; editor.open([activeCard.dataset.personId], event.currentTarget);
  });
  retry.addEventListener("click", async function () {
    if (busy) return;
    setBusy(true);
    try { await refresh(); status.textContent = "Ansicht aktualisiert. Bitte das Ergebnis prüfen."; }
    catch (error) { failed(error); }
    finally { setBusy(false); }
  });
  setBusy(false);
}());
