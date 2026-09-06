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
      var response = await fetch("/settings/photos/faces?format=json", {credentials:"same-origin", signal:controller.signal});
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
