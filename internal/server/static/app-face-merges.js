(function () {
  "use strict";
  var list = document.querySelector("[data-merge-suggestions]");
  if (!list) return;
  var status = document.querySelector("[data-merge-status]");
  var refreshButton = document.querySelector("[data-merge-refresh]");
  var hint = document.querySelector("[data-merge-hint]");
  var pending = new Set(), uncertain = new Set();
  var refreshing = false, refreshRequested = false, manualRefresh = false, revision = 0;

  function notify(message) {
    status.textContent = message;
    status.hidden = false;
  }

  function syncBusy() {
    list.setAttribute("aria-busy", String(refreshing));
    list.querySelectorAll("[data-merge-id]").forEach(function (card) {
      var id = card.dataset.mergeId;
      if (pending.has(id)) card.setAttribute("aria-busy", "true");
      else card.removeAttribute("aria-busy");
      card.querySelectorAll("button").forEach(function (button) { button.disabled = pending.has(id) || uncertain.has(id); });
    });
    refreshButton.disabled = refreshing || refreshRequested;
  }

  async function loadSuggestions() {
    var response = await fetch("/photos/people/merge-suggestions", {
      credentials: "same-origin", redirect: "error", cache: "no-store", headers: { Accept: "text/html" }
    });
    if (!response.ok) throw new Error("Vorschläge konnten nicht aktualisiert werden.");
    var documentView = new DOMParser().parseFromString(await response.text(), "text/html");
    var updated = documentView.querySelector("[data-merge-suggestions]");
    if (!updated) throw new Error("Vorschläge konnten nicht aktualisiert werden. Bitte prüfe deine Anmeldung.");
    return updated;
  }

  function applySuggestions(updated) {
    var existing = new Map();
    list.querySelectorAll("[data-merge-id]").forEach(function (card) {
      existing.set(card.dataset.mergeId, card);
    });
    var children = Array.from(updated.children).map(function (card) {
      var old = existing.get(card.dataset.mergeId);
      if (old) {
        // Compare a copy; never briefly unlock the live card during reconciliation.
        var normalized = old.cloneNode(true);
        normalized.removeAttribute("aria-busy");
        normalized.querySelectorAll("button").forEach(function (button) { button.disabled = false; });
        if (normalized.isEqualNode(card)) return old;
      }
      return document.importNode(card, true);
    });
    Array.from(list.children).forEach(function (card) {
      if (!children.includes(card)) card.remove();
    });
    children.forEach(function (card, index) {
      if (list.children[index] !== card) list.insertBefore(card, list.children[index] || null);
    });
    hint.hidden = !list.querySelector("[data-merge-id]");
    refreshButton.hidden = true;
  }

  async function refresh() {
    refreshRequested = true;
    // Coalesce refreshes until all current writes finish. Other cards stay usable.
    if (pending.size || refreshing) { syncBusy(); return; }
    refreshRequested = false;
    refreshing = true;
    var version = revision;
    syncBusy();
    try {
      var updated = await loadSuggestions();
      if (version !== revision) { refreshRequested = true; return; }
      applySuggestions(updated);
      uncertain.clear();
      if (manualRefresh) notify("Vorschläge aktualisiert. Bitte die Gruppen erneut prüfen.");
      manualRefresh = false;
    } catch (error) {
      if (version !== revision) { refreshRequested = true; return; }
      notify((status.textContent ? status.textContent + " " : "") + error.message);
      refreshButton.hidden = false;
    } finally {
      refreshing = false;
      syncBusy();
      if (refreshRequested && !pending.size) refresh();
    }
  }

  refreshButton.addEventListener("click", function () {
    manualRefresh = true;
    refresh();
  });

  list.addEventListener("submit", async function (event) {
    var form = event.target;
    if (!form.matches("form")) return;
    event.preventDefault();
    var card = form.closest("[data-merge-id]");
    if (!card) return;
    var id = card.dataset.mergeId;
    if (pending.has(id) || uncertain.has(id)) return;
    var submitter = event.submitter;
    var action = submitter && submitter.getAttribute("formaction") || form.action;
    var body = new URLSearchParams(new FormData(form));
    var message = "";
    pending.add(id);
    revision++;
    syncBusy();
    try {
      var response = await fetch(action, {
        method: "POST", credentials: "same-origin", redirect: "error",
        headers: { Accept: "application/json" }, body: body
      });
      if (response.status === 409 || response.status === 404) {
        message = "Die Personengruppen haben sich geändert. Bitte die aktualisierten Vorschläge erneut prüfen.";
        uncertain.add(id);
      } else {
        var result = await response.json();
        if (!response.ok || result.ok !== true) throw new Error(result.error || "Die Entscheidung konnte nicht gespeichert werden.");
        message = action.endsWith("/reject") ? "Die Gruppen bleiben getrennt." : "Personengruppen zusammengeführt.";
        card.remove();
      }
      notify(message);
      refreshRequested = true;
    } catch (_) {
      uncertain.add(id);
      notify("Die Entscheidung konnte nicht bestätigt werden. Bitte die Vorschläge aktualisieren, bevor du es erneut versuchst.");
      refreshButton.hidden = false;
    } finally {
      pending.delete(id);
      revision++;
      syncBusy();
      if (refreshRequested) refresh();
      if (submitter && !submitter.isConnected && document.activeElement === document.body) {
        var next = list.querySelector("button:not([disabled])");
        if (next) next.focus({ preventScroll: true });
        else { status.tabIndex = -1; status.focus({ preventScroll: true }); }
      }
    }
  });

  // Reuse the person picker, while preserving the reviewed pair and its revisions.
  var dialog = document.querySelector("[data-person-dialog]");
  if (!dialog) return;
  var namingForm = dialog.querySelector("form");
  var namingStatus = dialog.querySelector("[data-person-dialog-status]");
  var targetInput = namingForm.querySelector("[data-person-target]");
  var nameInput = namingForm.querySelector("[data-person-search]");
  var cancelNaming = dialog.querySelector("[data-person-dialog-cancel]");
  var namingCard, namingOpener, namingSession, namingOperation, namingBusy = false;
  namingForm.dataset.personSuggestionsUrl = "/api/photos/labeling/v1/suggestions?q=";
  dialog.querySelector("[data-person-dialog-ignore]").hidden = true;
  dialog.querySelector("[data-person-face-match]").hidden = true;

  function namingControls(disabled) {
    namingForm.querySelectorAll("input, button").forEach(function (control) { control.disabled = disabled; });
    dialog.setAttribute("aria-busy", String(disabled));
  }
  function closeNaming() { if (!namingBusy) dialog.close(); }
  cancelNaming.addEventListener("click", closeNaming);
  dialog.addEventListener("cancel", function (event) { if (namingBusy) event.preventDefault(); });
  dialog.addEventListener("close", function () {
    namingForm.dispatchEvent(new CustomEvent("person-picker-close"));
    if (namingOpener && namingOpener.isConnected && !namingOpener.disabled) namingOpener.focus({ preventScroll: true });
    else {
      var next = list.querySelector("button:not([disabled])");
      if (next) next.focus({ preventScroll: true });
      else { status.tabIndex = -1; status.focus({ preventScroll: true }); }
    }
  });
  list.addEventListener("click", async function (event) {
    var button = event.target.closest("[data-merge-name]");
    if (!button || button.disabled || dialog.open) return;
    namingCard = button.closest("[data-merge-id]");
    namingOpener = button;
    namingSession = null;
    // An operation ID is retained until the outcome is known; uncertain writes
    // close the dialog and lock this card until suggestions have been reloaded.
    namingOperation = Array.from(crypto.getRandomValues(new Uint8Array(16)), function (b) { return b.toString(16).padStart(2, "0"); }).join("");
    var operation = namingOperation;
    dialog.querySelector("#person-dialog-title").textContent = "Zusammenführen und benennen/zuordnen";
    dialog.querySelector("#overview-person-hint").textContent = "Beide Gruppen werden unter dem neuen Namen oder mit der ausgewählten Person zusammengeführt. Abbrechen ändert nichts.";
    dialog.querySelector(".person-dialog-photo").replaceChildren(namingCard.querySelector(".face-merge-pair").cloneNode(true));
    dialog.querySelectorAll(".person-dialog-photo a").forEach(function (link) { link.removeAttribute("href"); });
    namingForm.dataset.personCount = "2";
    namingForm.dataset.renameAction = "/unused/rename";
    namingForm.dispatchEvent(new CustomEvent("person-picker-reset", { detail: { name: "" } }));
    namingStatus.textContent = "Namenssuche wird vorbereitet …";
    namingControls(true);
    cancelNaming.disabled = false;
    dialog.showModal();
    try {
      var response = await fetch("/api/photos/labeling/v1/session", { credentials: "same-origin", redirect: "error", cache: "no-store" });
      if (!response.ok) throw new Error("Namenssuche konnte nicht geladen werden. Bitte den Dialog erneut öffnen.");
      var session = await response.json();
      if (operation !== namingOperation || !dialog.open) return;
      namingSession = session;
      namingStatus.textContent = "";
      namingControls(false);
      nameInput.focus();
    } catch (error) {
      if (operation === namingOperation && dialog.open) namingStatus.textContent = error.message;
    }
  });
  namingForm.addEventListener("submit", async function (event) {
    event.preventDefault();
    if (namingBusy || !namingSession) return;
    var card = namingCard, id = card.dataset.mergeId;
    if (!card.isConnected || pending.has(id) || uncertain.has(id)) {
      closeNaming();
      notify("Die Personengruppen haben sich geändert. Bitte die aktualisierten Vorschläge erneut prüfen.");
      return;
    }
    var target = Number(targetInput.value) || 0;
    var name = target ? "" : nameInput.value.trim();
    if (!target && !name) return;
    var body = {
      action: "name_merge", operation_id: namingOperation, dataset: namingSession.dataset,
      suggestion_id: Number(id), revision: Number(card.querySelector('[name="source_revision"]').value),
      target_id: Number(card.dataset.targetId), target_revision: Number(card.querySelector('[name="target_revision"]').value),
      name: name, allow_duplicate: targetInput.value === "0",
      assign_id: target, assign_revision: target ? Number(targetInput.dataset.revision) : 0
    };
    namingBusy = true;
    pending.add(id); revision++; syncBusy();
    namingForm.dispatchEvent(new CustomEvent("person-picker-close"));
    namingControls(true);
    namingStatus.textContent = "Personengruppen werden gespeichert …";
    try {
      var response = await fetch("/api/photos/labeling/v1/people/" + encodeURIComponent(card.dataset.sourceId) + "/actions", {
        method: "POST", credentials: "same-origin", redirect: "error",
        headers: { Accept: "application/json", "Content-Type": "application/json" }, body: JSON.stringify(body)
      });
      var result = await response.json();
      if (response.status === 409 && result.code === "name_exists") {
        namingStatus.textContent = "Dieser Name existiert bereits. Bitte eine vorhandene Person oder ausdrücklich einen neuen Eintrag aus der Namenssuche auswählen.";
        return;
      }
      if (!response.ok) {
        if (response.status === 409 || response.status === 404) {
          uncertain.add(id); refreshRequested = true;
          notify("Die Personengruppen haben sich geändert. Bitte die aktualisierten Vorschläge erneut prüfen.");
          dialog.close();
          return;
        }
        throw new Error("Speichern nicht bestätigt");
      }
      if (result.operation_id !== namingOperation || result.action !== "name_merge") throw new Error("Ungültige Quittung");
      card.remove(); refreshRequested = true;
      notify("Personengruppen zusammengeführt und benannt/zugeordnet.");
      dialog.close();
    } catch (_) {
      uncertain.add(id);
      notify("Die Entscheidung konnte nicht bestätigt werden. Bitte die Vorschläge aktualisieren, bevor du es erneut versuchst.");
      refreshButton.hidden = false;
      dialog.close();
    } finally {
      namingBusy = false; namingControls(false);
      pending.delete(id); revision++; syncBusy();
      if (refreshRequested) refresh();
    }
  });
})();
