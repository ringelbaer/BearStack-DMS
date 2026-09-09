(function () {
  "use strict";
  var state = document.querySelector("[data-face-worker-state]");
  if (!state) return;
  var busy = false;
  var controller;
  async function refresh() {
    if (busy || document.hidden) return;
    busy = true;
    controller = new AbortController();
    try {
      var response = await fetch("/settings/photos/faces?format=json&progress=1", {credentials:"same-origin", signal:controller.signal});
      if (!response.ok) return;
      var data = await response.json();
      document.querySelectorAll("[data-face-count]").forEach(function (element) {
        var value = data.status[element.dataset.faceCount];
        if (typeof value === "number") element.textContent = String(value);
      });
      state.textContent = data.running ? "Verarbeitung läuft" : data.settings.enabled ? "Aktiviert – wartet auf nächsten Lauf" : "Pausiert / ausgeschaltet";
      var error = document.querySelector("[data-face-worker-error]");
      error.textContent = data.error || "";
      error.hidden = !data.error;
      var reconciliationState = document.querySelector("[data-face-reconciliation-state]");
      if (reconciliationState && data.reconciliation) {
        reconciliationState.textContent = !data.settings.reconcile_enabled ? "Pausiert" : data.reconciliation_running ? "Abgleich läuft" : data.reconciliation.pending ? "Abgleich ausstehend" : "Abgleich abgeschlossen";
        document.querySelectorAll("[data-face-reconciliation-count]").forEach(function (element) {
          var value = data.reconciliation[element.dataset.faceReconciliationCount];
          if (typeof value === "number") element.textContent = String(value);
        });
        var reconciliationError = document.querySelector("[data-face-reconciliation-error]");
        if (reconciliationError) {
          reconciliationError.textContent = data.reconciliation_error || "";
          reconciliationError.hidden = !data.reconciliation_error;
        }
      }
    } catch (_) {
      // A temporary connection failure must not discard unsaved form changes.
    } finally {
      busy = false;
    }
  }
  var timer = setInterval(refresh, 5000);
  document.addEventListener("visibilitychange", refresh);
  window.addEventListener("pagehide", function () { clearInterval(timer); if (controller) controller.abort(); });
}());
