(function () {
  "use strict";
  var list = document.querySelector("[data-merge-suggestions]");
  if (!list) return;
  var status = document.querySelector("[data-merge-status]");
  var refreshButton = document.querySelector("[data-merge-refresh]");
  var hint = document.querySelector("[data-merge-hint]");
  var busy = false;

  function notify(message) {
    status.textContent = message;
    status.hidden = false;
  }

  function setBusy(value) {
    busy = value;
    list.setAttribute("aria-busy", String(value));
    list.querySelectorAll("button").forEach(function (button) { button.disabled = value; });
    refreshButton.disabled = value;
  }

  async function refresh() {
    var response = await fetch("/photos/people/merge-suggestions", {
      credentials: "same-origin", redirect: "error", cache: "no-store", headers: { Accept: "text/html" }
    });
    if (!response.ok) throw new Error("Vorschläge konnten nicht aktualisiert werden.");
    var documentView = new DOMParser().parseFromString(await response.text(), "text/html");
    var updated = documentView.querySelector("[data-merge-suggestions]");
    if (!updated) throw new Error("Vorschläge konnten nicht aktualisiert werden. Bitte prüfe deine Anmeldung.");
    var existing = new Map();
    list.querySelectorAll("[data-merge-id]").forEach(function (card) {
      // Ignore the temporary disabled state when comparing server-rendered cards.
      card.querySelectorAll("button").forEach(function (button) { button.disabled = false; });
      existing.set(card.dataset.mergeId, card);
    });
    var children = Array.from(updated.children).map(function (card) {
      var old = existing.get(card.dataset.mergeId);
      return old && old.isEqualNode(card) ? old : document.importNode(card, true);
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

  refreshButton.addEventListener("click", async function () {
    if (busy) return;
    setBusy(true);
    try {
      await refresh();
      notify("Vorschläge aktualisiert. Bitte die Gruppen erneut prüfen.");
    } catch (error) {
      notify(error.message);
    } finally {
      setBusy(false);
    }
  });

  list.addEventListener("submit", async function (event) {
    var form = event.target;
    if (!form.matches("form")) return;
    event.preventDefault();
    if (busy) return;
    var submitter = event.submitter;
    var action = submitter && submitter.getAttribute("formaction") || form.action;
    var card = form.closest("[data-merge-id]");
    var body = new URLSearchParams(new FormData(form));
    var message = "";
    setBusy(true);
    try {
      var response = await fetch(action, {
        method: "POST", credentials: "same-origin", redirect: "error",
        headers: { Accept: "application/json" }, body: body
      });
      if (response.status === 409 || response.status === 404) {
        message = "Die Personengruppen haben sich geändert. Bitte die aktualisierten Vorschläge erneut prüfen.";
      } else {
        var result = await response.json();
        if (!response.ok || result.ok !== true) throw new Error(result.error || "Die Entscheidung konnte nicht gespeichert werden.");
        message = action.endsWith("/reject") ? "Die Gruppen bleiben getrennt." : "Personengruppen zusammengeführt.";
        card.remove();
      }
      notify(message);
      try {
        await refresh();
      } catch (error) {
        notify(message + " " + error.message);
        refreshButton.hidden = false;
      }
    } catch (_) {
      notify("Die Entscheidung konnte nicht bestätigt werden. Bitte die Vorschläge aktualisieren, bevor du es erneut versuchst.");
      refreshButton.hidden = false;
    } finally {
      setBusy(false);
      if (submitter && !submitter.isConnected && document.activeElement === document.body) {
        var next = list.querySelector("button");
        if (next) next.focus({ preventScroll: true });
        else { status.tabIndex = -1; status.focus({ preventScroll: true }); }
      }
    }
  });
})();
