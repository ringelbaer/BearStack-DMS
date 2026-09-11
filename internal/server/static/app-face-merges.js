(function () {
  "use strict";
  var list = document.querySelector("[data-merge-suggestions]");
  if (!list) return;
  var status = document.querySelector("[data-merge-status]");
  var refreshButton = document.querySelector("[data-merge-refresh]");
  var hint = document.querySelector("[data-merge-hint]");
  var pending = new Set(), uncertain = new Set();
  var sideWrites = new Map(), dismissed = new Set();
  var refreshing = false, refreshRequested = false, manualRefresh = false, revision = 0;
  // DOMParser parses noscript children as real controls. Remove the fallback in
  // both live and refreshed cards so its required checkbox never blocks AJAX.
  function removeFallbacks(root) { root.querySelectorAll("noscript").forEach(function (element) { element.remove(); }); }
  removeFallbacks(list);

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
      card.querySelectorAll("button").forEach(function (button) {
        var side = button.closest("[data-merge-side]");
        button.disabled = pending.has(id) || uncertain.has(id) || !!(side && side.dataset.handled);
      });
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
    removeFallbacks(updated);
    var existing = new Map(), retainedPairs = new Set();
    list.querySelectorAll("[data-merge-id]").forEach(function (card) {
      existing.set(card.dataset.mergeId, card);
      if (card.dataset.individual && !card.dataset.invalidSide) retainedPairs.add(pairKey(card));
    });
    var children = Array.from(updated.children).filter(function (card) {
      return !dismissed.has(pairKey(card)) && !retainedPairs.has(pairKey(card));
    }).slice(0, Math.max(0, 60 - retainedPairs.size)).map(function (card) {
      var old = existing.get(card.dataset.mergeId);
      if (old) {
        if (old.dataset.individual && !old.dataset.invalidSide) return old;
        // Compare a copy; never briefly unlock the live card during reconciliation.
        var normalized = old.cloneNode(true);
        normalized.removeAttribute("aria-busy");
        normalized.querySelectorAll("button").forEach(function (button) { button.disabled = false; });
        if (normalized.isEqualNode(card)) return old;
      }
      return document.importNode(card, true);
    });
    existing.forEach(function (card) {
      if (card.dataset.individual && !card.dataset.invalidSide && !children.includes(card)) children.unshift(card);
    });
    if (children.some(function (card) { return card.dataset.mergeId; })) children = children.filter(function (card) { return card.dataset.mergeId; });
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
      for (var write of sideWrites.values()) await saveSide(write, true);
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

  function pairKey(card) { return [card.dataset.sourceId, card.dataset.targetId].sort().join(":"); }
  function operationID() { return Array.from(crypto.getRandomValues(new Uint8Array(16)), function (b) { return b.toString(16).padStart(2, "0"); }).join(""); }
  async function getSession() {
    var response = await fetch("/api/photos/labeling/v1/session", { credentials: "same-origin", redirect: "error", cache: "no-store" });
    if (!response.ok) throw new Error("Die Sitzung konnte nicht geladen werden. Bitte erneut versuchen.");
    return response.json();
  }
  function completeSide(write, result) {
    if (result.operation_id !== write.body.operation_id || result.action !== write.body.action || Number(result.source_id) !== Number(write.side.dataset.mergeSide)) throw new Error("Ungültige Quittung");
    var card = write.card, side = write.side;
    side.dataset.handled = "true";
    side.querySelector(".face-merge-side-actions").hidden = true;
    if (write.body.action === "name") {
      side.dataset.sideName = write.body.name;
      side.querySelector("strong").textContent = write.body.name;
    }
    var note = side.querySelector("[data-merge-side-status]");
    note.textContent = write.body.action === "ignore" ? "Ignoriert" : write.body.action === "assign" ? "Zugeordnet" : "Benannt: " + write.body.name;
    note.hidden = false;
    // Explicitly assigning to the other side also changes that destination.
    var target = card.querySelector('[data-merge-side="' + Number(write.body.target_id) + '"]');
    if (target) target.dataset.handled = "true";
    card.dataset.individual = "true";
    card.querySelector("form").hidden = true;
    card.querySelector("[data-merge-dismiss]").hidden = false;
    sideWrites.delete(card.dataset.mergeId);
    uncertain.delete(card.dataset.mergeId);
    notify("Gruppe gespeichert. Du kannst die andere Seite bearbeiten oder das Paar ausblenden.");
  }
  async function saveSide(write, recover) {
    var response;
    if (recover) {
      response = await fetch("/api/photos/labeling/v1/actions/" + encodeURIComponent(write.body.operation_id) + "?dataset=" + encodeURIComponent(write.body.dataset), {
        credentials: "same-origin", redirect: "error", cache: "no-store"
      });
    }
    if (!response || response.status === 404) response = await fetch("/api/photos/labeling/v1/people/" + encodeURIComponent(write.side.dataset.mergeSide) + "/actions", {
      method: "POST", credentials: "same-origin", redirect: "error", headers: { "Content-Type": "application/json", Accept: "application/json" }, body: JSON.stringify(write.body)
    });
    var result = await response.json();
    if (!response.ok) {
      if ([400,404,409].includes(response.status)) {
        sideWrites.delete(write.card.dataset.mergeId);
        if (result.code !== "name_exists") write.card.dataset.invalidSide = "true";
      }
      var error = new Error(result.error || "Speichern nicht bestätigt"); error.code = result.code; throw error;
    }
    completeSide(write, result);
  }
  list.addEventListener("click", async function (event) {
    var button = event.target.closest("[data-merge-ignore], [data-merge-dismiss]");
    if (!button || button.disabled) return;
    var card = button.closest("[data-merge-id]"), id = card.dataset.mergeId;
    if (pending.has(id) || uncertain.has(id)) return;
    if (button.hasAttribute("data-merge-dismiss")) {
      if (!card.dataset.individual) return;
      dismissed.add(pairKey(card)); card.remove(); revision++; refresh(); return;
    }
    var side = button.closest("[data-merge-side]");
    if (side.dataset.handled || side.dataset.sideName) return;
    pending.add(id); revision++; syncBusy();
    try {
      var session = await getSession();
      var write = { card: card, side: side, body: { action: "ignore", operation_id: operationID(), dataset: session.dataset, revision: Number(side.dataset.sideRevision) } };
      sideWrites.set(id, write);
      await saveSide(write, false);
    } catch (error) {
      uncertain.add(id); refreshButton.hidden = false;
      notify("Die Aktion konnte nicht bestätigt werden. Bitte Vorschläge aktualisieren, um die Aktion zu prüfen.");
    } finally { pending.delete(id); revision++; syncBusy(); if (refreshRequested) refresh(); }
  });

  list.addEventListener("submit", async function (event) {
    var form = event.target;
    if (!form.matches("form")) return;
    event.preventDefault();
    var card = form.closest("[data-merge-id]");
    if (!card) return;
    if (card.dataset.individual) return;
    var id = card.dataset.mergeId;
    if (pending.has(id) || uncertain.has(id)) return;
    var submitter = event.submitter;
    var action = submitter && submitter.getAttribute("formaction") || form.action;
    var body = new URLSearchParams(new FormData(form));
    var message = "", cancelled = false;
    pending.add(id);
    revision++;
    syncBusy();
    try {
      // Keep this pair locked, with its displayed revisions, while confirming.
      // Rejecting a pair never needs confirmation, even if both groups are named.
      if (action.endsWith("/accept") && form.dataset.mergeWarning) {
        if (!await showAppConfirm(form.dataset.mergeWarning, "Benannte Gruppen zusammenführen?")) {
          cancelled = true;
          return;
        }
      }
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
      if (cancelled && submitter && submitter.isConnected && !submitter.disabled) {
        submitter.focus({ preventScroll: true });
      }
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
  var namingCard, namingSide, namingOpener, namingSession, namingOperation, namingBusy = false;
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
    var button = event.target.closest("[data-merge-name], [data-merge-side-name]");
    if (!button || button.disabled || dialog.open) return;
    namingCard = button.closest("[data-merge-id]");
    namingSide = button.closest("[data-merge-side]");
    namingOpener = button;
    namingSession = null;
    // An operation ID is retained until the outcome is known; uncertain writes
    // close the dialog and lock this card until suggestions have been reloaded.
    namingOperation = operationID();
    var operation = namingOperation;
    dialog.querySelector("#person-dialog-title").textContent = namingSide ? "Gruppe benennen/zuordnen" : "Zusammenführen und benennen/zuordnen";
    dialog.querySelector("#overview-person-hint").textContent = namingSide ? "Nur diese Gruppe wird benannt oder einer vorhandenen Person zugeordnet." : "Beide Gruppen werden unter dem neuen Namen oder mit der ausgewählten Person zusammengeführt. Abbrechen ändert nichts.";
    dialog.querySelector(".person-dialog-photo").replaceChildren((namingSide ? namingSide.querySelector(".person-card") : namingCard.querySelector(".face-merge-pair")).cloneNode(true));
    dialog.querySelectorAll(".person-dialog-photo button, .person-dialog-photo [data-merge-side-status]").forEach(function (element) { element.remove(); });
    dialog.querySelectorAll(".person-dialog-photo a").forEach(function (link) { link.removeAttribute("href"); });
    namingForm.dataset.personCount = namingSide ? "1" : "2";
    namingForm.dataset.personExclude = namingSide ? namingSide.dataset.mergeSide : "";
    namingForm.dataset.renameAction = "/unused/rename";
    namingForm.dispatchEvent(new CustomEvent("person-picker-reset", { detail: { name: namingSide ? namingSide.dataset.sideName : "" } }));
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
    if (namingSide) {
      if (namingSide.dataset.handled || namingSide.dataset.sideName || target === Number(namingSide.dataset.mergeSide)) return;
      var write = { card: card, side: namingSide, body: {
        action: target ? "assign" : "name",
        operation_id: namingOperation, dataset: namingSession.dataset,
        revision: Number(namingSide.dataset.sideRevision), name: name,
        target_id: target, target_revision: target ? Number(targetInput.dataset.revision) : 0,
        allow_duplicate: targetInput.value === "0"
      } };
      namingBusy = true; pending.add(id); revision++; syncBusy(); namingControls(true);
      namingForm.dispatchEvent(new CustomEvent("person-picker-close"));
      sideWrites.set(id, write);
      try { await saveSide(write, false); dialog.close(); }
      catch (error) {
        if (error.code === "name_exists") {
          namingStatus.textContent = "Dieser Name existiert bereits. Bitte eine vorhandene Person oder ausdrücklich einen neuen Eintrag auswählen.";
        } else {
          uncertain.add(id); refreshButton.hidden = false;
          notify("Die Aktion konnte nicht bestätigt werden. Bitte Vorschläge aktualisieren, um die Aktion zu prüfen."); dialog.close();
        }
      } finally { namingBusy = false; namingControls(false); pending.delete(id); revision++; syncBusy(); if (refreshRequested) refresh(); }
      return;
    }
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
