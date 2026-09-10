(function () {
  "use strict";
  var grid = document.querySelector("[data-ignored-faces]");
  if (!grid || !window.BearStackPersonDialog) return;
  var status = document.querySelector("[data-ignored-status]");
  var retry = document.querySelector("[data-ignored-retry]");
  var busy = false, uncertain = false, activeCard;
  function setBusy(value) {
    busy = value;
    grid.setAttribute("aria-busy", String(value));
    grid.querySelectorAll("button").forEach(function (button) { button.disabled = busy || uncertain; });
    grid.querySelectorAll("[data-ignored-edit]").forEach(function (button) { button.hidden = false; });
    retry.disabled = busy;
  }
  function failed(error) {
    uncertain = true; retry.hidden = false;
    status.textContent = error.message + " Bitte die Ansicht erneut laden und den aktuellen Zustand prüfen.";
  }
  async function refresh() {
    var response = await fetch(location.href, { credentials: "same-origin", redirect: "error", cache: "no-store", headers: { Accept: "text/html" } });
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
    uncertain = false; retry.hidden = true;
  }
  var editor = window.BearStackPersonDialog.bind({
    surface: grid, status: status,
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
      context.dialog.querySelector("#person-dialog-title").textContent = "Gesicht benennen und wiederherstellen";
      context.dialog.querySelector("#overview-person-hint").textContent = "Nur dieses Gesicht wird wiederhergestellt. Einen neuen Namen eingeben oder eine vorhandene Person auswählen.";
      context.form.dispatchEvent(new CustomEvent("person-picker-reset", { detail: { name: activeCard.dataset.personName || "" } }));
    },
    getSaveRequest: function (request) {
      var target = request.body.get("target") || "0";
      return { action: "/photos/faces/edit", body: new URLSearchParams({ action: "restore", ignored: "1", face_id: activeCard.dataset.ignoredFace, target: target, name: target === "0" ? request.body.get("name") : "" }) };
    },
    onSave: refresh,
    savedMessage: function () { return "Gesicht benannt oder zugeordnet und wiederhergestellt."; },
    onSaveError: function (error) { failed(error); return true; }
  });
  grid.addEventListener("click", function (event) {
    var button = event.target.closest("[data-ignored-edit]");
    if (!button || busy || uncertain) return;
    activeCard = button.closest("[data-ignored-face]");
    editor.open([activeCard.dataset.personId], button);
  });
  grid.addEventListener("submit", async function (event) {
    var form = event.target.closest(".ignored-face-form");
    if (!form) return;
    event.preventDefault();
    if (busy || uncertain) return;
    var body = new URLSearchParams(new FormData(form));
    setBusy(true); status.textContent = "Gesicht wird wiederhergestellt …";
    try {
      var response = await fetch(form.getAttribute("action"), { method: "POST", credentials: "same-origin", redirect: "error", headers: { Accept: "application/json" }, body: body });
      if (!response.ok) throw new Error("Wiederherstellen konnte nicht bestätigt werden (HTTP " + response.status + ").");
      if ((await response.json()).ok !== true) throw new Error("Wiederherstellen konnte nicht bestätigt werden.");
      await refresh(); status.textContent = "Gesicht unbenannt wiederhergestellt.";
    } catch (error) { failed(error); }
    finally {
      setBusy(false);
      if (!form.isConnected) {
        var focus = grid.querySelector("button:not([disabled])") || document.querySelector("[data-people-filter] select");
        if (focus) focus.focus({ preventScroll: true });
      }
    }
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
