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
})();
