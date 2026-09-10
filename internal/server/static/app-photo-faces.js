(function () {
  "use strict";
  window.BearStackPhotoFaces = {
    init: function (options) {
      var section = options.dialog.querySelector("[data-photo-face-tools]");
      if (!section || !window.BearStackPersonDialog) return null;
      var grid = section.querySelector("[data-photo-face-list]");
      var status = section.querySelector("[data-photo-face-status]");
      var analyze = section.querySelector("[data-photo-face-analyze]");
      var refreshButton = section.querySelector("[data-photo-face-refresh]");
      var drawButton = section.querySelector("[data-photo-face-draw]");
      var restoreButton = section.querySelector("[data-photo-face-unignore]");
      var ignoredCount = 0, ignoredPeople = [], uncertain = false;
      var currentFaces = [];
      var current, controller, generation = 0, busy = false, editing = false, displayPath = "", revision = "";
      function setBusy(value) {
        busy = value;
        analyze.disabled = value || editing || uncertain;
        refreshButton.disabled = value || editing;
        drawButton.disabled = value || editing || uncertain;
        restoreButton.disabled = value || editing || uncertain || !ignoredCount || !revision;
        grid.querySelectorAll("button").forEach(function (button) { button.disabled = value || editing || uncertain; });
        section.setAttribute("aria-busy", String(value || editing));
      }
      function reset() {
        generation++;
        if (controller) controller.abort();
        current = null;
        currentFaces = [];
        grid.replaceChildren();
        status.textContent = "";
        displayPath = "";
        revision = ""; ignoredCount = 0; ignoredPeople = []; uncertain = false;
        setBusy(false);
      }
      function render(photo) {
        if (!photo || photo.path !== current.path || !Array.isArray(photo.faces)) throw new Error("Ungültige Antwort der Gesichtserkennung.");
        displayPath = photo.display_path || "";
        revision = photo.revision || "";
        currentFaces = photo.faces;
        ignoredCount = photo.faces.filter(function (face) { return face.ignored; }).length;
        ignoredPeople = Array.from(new Set(photo.faces.filter(function (face) { return face.ignored; }).map(function (face) { return String(face.person_id); })));
        var faces = photo.faces.filter(function (face) { return !face.ignored; });
        grid.replaceChildren();
        faces.forEach(function (face) {
          var card = document.createElement("div");
          card.className = "photo-info-face";
          card.dataset.personId = String(face.person_id);
          card.dataset.personName = face.name || "";
          card.dataset.faceId = String(face.id);
          card.dataset.displayPath = displayPath;
          ["x", "y", "width", "height"].forEach(function (key) { card.dataset[key] = String(face[key]); });
          var image = document.createElement("img");
          image.src = "/photos/faces/" + encodeURIComponent(face.id) + "/thumbnail";
          image.width = 64; image.height = 64; image.alt = ""; image.loading = "lazy";
          var label = document.createElement("span");
          label.textContent = face.name || "Unbenannt";
          var edit = window.BearStackPersonDialog.createEditButton("Benennen oder zuordnen: " + label.textContent);
          card.append(image, label, edit);
          grid.append(card);
        });
        options.onFaces(current, faces);
        return faces.length;
      }
      async function request(recognize) {
        var restoring = recognize === "restore";
        var writing = recognize === true || restoring;
        if (!current || busy || (writing && (editing || uncertain)) || (restoring && (!ignoredCount || !revision))) return;
        var restoredPeople = ignoredPeople.slice();
        var item = current, token = ++generation;
        if (controller) controller.abort();
        controller = new AbortController();
        setBusy(true);
        status.textContent = restoring ? "Ignorierte Gesichter werden wiederhergestellt …" : recognize ? "Foto wird erkannt und zugeordnet …" : "Gesichter werden geladen …";
        try {
          var response = await fetch(restoring ? "/photos/faces/unignore" : recognize ? "/photos/faces/analyze" : "/photos/faces?path=" + encodeURIComponent(item.path), {
            method: recognize ? "POST" : "GET", credentials: "same-origin", redirect: "error",
            signal: controller.signal, headers: { Accept: "application/json" },
            body: restoring ? new URLSearchParams({ path: item.path, revision: revision }) : recognize ? new URLSearchParams({ path: item.path }) : undefined
          });
          var result = await response.json();
          if (token !== generation || current !== item) return;
          if (!response.ok) throw new Error(result.error || "Gesichter konnten nicht geladen werden.");
          var restored = 0;
          if (restoring) {
            if (result.ok !== true || !Number.isInteger(result.restored)) throw new Error("Wiederherstellen konnte nicht bestätigt werden.");
            restored = result.restored;
            document.dispatchEvent(new CustomEvent("photo-people-updated", { detail: { ids: restoredPeople, target: null } }));
            var refreshed = await fetch("/photos/faces?path=" + encodeURIComponent(item.path), { credentials: "same-origin", redirect: "error", cache: "no-store", signal: controller.signal, headers: { Accept: "application/json" } });
            if (!refreshed.ok) throw new Error("Gesichter konnten nicht aktualisiert werden.");
            result = await refreshed.json();
            if (token !== generation || current !== item) return;
          }
          var count = render(result.photo);
          if (!writing && uncertain) document.dispatchEvent(new CustomEvent("photo-people-updated", { detail: { ids: restoredPeople, target: null } }));
          uncertain = false;
          status.textContent = restoring ? (restored === 1 ? "Ein Gesicht wiederhergestellt." : restored + " Gesichter wiederhergestellt.") : count ? "" : "Keine aktiven Gesichter gefunden.";
        } catch (error) {
          if (token !== generation || error.name === "AbortError") return;
          if (restoring) uncertain = true;
          status.textContent = (error.message || "Gesichtserkennung fehlgeschlagen. Bitte erneut versuchen.") + (restoring ? " Bitte zuerst die Gesichter aktualisieren." : "");
          throw error;
        } finally {
          if (token === generation) setBusy(false);
        }
      }
      window.BearStackPersonDialog.bind({
        surface: grid, status: status,
        isBusy: function () { return busy || uncertain; },
        onBusy: function (value) { editing = value; setBusy(busy); },
        getPreviewPath: function () { return displayPath; },
        getIgnoreRequest: function (cards) {
          var card = cards[0];
          if (!current || !revision || cards.length !== 1 || !card || card.dataset.personName || !card.dataset.faceId) return null;
          return { action: "/photos/people/groups/ignore", body: new URLSearchParams({ path: current.path, revision: revision, face_id: card.dataset.faceId }) };
        },
        onIgnoreConflict: function () { return request(false); },
        onSave: function (ids, saved) {
          document.dispatchEvent(new CustomEvent("photo-people-updated", { detail: { ids: ids, target: saved.action.endsWith("/merge") ? saved.body.get("target") : null } }));
          return request(false);
        }
      });
      grid.addEventListener("click", function (event) { if (event.target.closest("[data-person-edit]")) options.stopSlideshow(); }, true);
      analyze.addEventListener("click", function () { options.stopSlideshow(); request(true).catch(function () {}); });
      restoreButton.addEventListener("click", function () { options.stopSlideshow(); request("restore").catch(function () {}); });
      refreshButton.addEventListener("click", function () { request(false).catch(function () {}); });
      var drawing = window.BearStackFaceDrawing.init({
        onBusy: function (value) { editing = value; setBusy(busy); },
        onSave: function () { return request(false); }
      });
      drawButton.addEventListener("click", function () {
        if (!current || busy || editing || uncertain) return;
        options.stopSlideshow();
        drawing.open(current.path, displayPath, drawButton, currentFaces);
      });
      return {
        reset: reset,
        show: function (item) {
          if (current === item) return;
          reset();
          section.hidden = !item || item.type !== "image" || !item.path;
          if (section.hidden) return;
          current = item;
          request(false).catch(function () {});
        }
      };
    }
  };
}());
