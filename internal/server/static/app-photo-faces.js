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
      var current, controller, generation = 0, busy = false, editing = false, displayPath = "", revision = "";
      function setBusy(value) {
        busy = value;
        analyze.disabled = value || editing;
        refreshButton.disabled = value || editing;
        drawButton.disabled = value || editing;
        grid.querySelectorAll("button").forEach(function (button) { button.disabled = value || editing; });
        section.setAttribute("aria-busy", String(value || editing));
      }
      function reset() {
        generation++;
        if (controller) controller.abort();
        current = null;
        grid.replaceChildren();
        status.textContent = "";
        displayPath = "";
        revision = "";
        setBusy(false);
      }
      function render(photo) {
        if (!photo || photo.path !== current.path || !Array.isArray(photo.faces)) throw new Error("Ungültige Antwort der Gesichtserkennung.");
        displayPath = photo.display_path || "";
        revision = photo.revision || "";
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
        if (!current || busy || (editing && recognize)) return;
        var item = current, token = ++generation;
        if (controller) controller.abort();
        controller = new AbortController();
        setBusy(true);
        status.textContent = recognize ? "Foto wird erkannt und zugeordnet …" : "Gesichter werden geladen …";
        try {
          var response = await fetch(recognize ? "/photos/faces/analyze" : "/photos/faces?path=" + encodeURIComponent(item.path), {
            method: recognize ? "POST" : "GET", credentials: "same-origin", redirect: "error",
            signal: controller.signal, headers: { Accept: "application/json" },
            body: recognize ? new URLSearchParams({ path: item.path }) : undefined
          });
          var result = await response.json();
          if (token !== generation || current !== item) return;
          if (!response.ok) throw new Error(result.error || "Gesichter konnten nicht geladen werden.");
          var count = render(result.photo);
          status.textContent = count ? "" : "Keine aktiven Gesichter gefunden.";
        } catch (error) {
          if (token !== generation || error.name === "AbortError") return;
          status.textContent = error.message || "Gesichtserkennung fehlgeschlagen. Bitte erneut versuchen.";
          throw error;
        } finally {
          if (token === generation) setBusy(false);
        }
      }
      window.BearStackPersonDialog.bind({
        surface: grid, status: status,
        isBusy: function () { return busy; },
        onBusy: function (value) { editing = value; setBusy(busy); },
        getPreviewPath: function () { return displayPath; },
        getIgnoreRequest: function (cards) {
          var card = cards[0];
          if (!current || !revision || cards.length !== 1 || !card || card.dataset.personName || !card.dataset.faceId) return null;
          return { action: "/photos/people/groups/ignore", body: new URLSearchParams({ path: current.path, revision: revision, face_id: card.dataset.faceId }) };
        },
        onIgnoreConflict: function () { return request(false); },
        onSave: function () { return request(false); }
      });
      grid.addEventListener("click", function (event) { if (event.target.closest("[data-person-edit]")) options.stopSlideshow(); }, true);
      analyze.addEventListener("click", function () { options.stopSlideshow(); request(true).catch(function () {}); });
      refreshButton.addEventListener("click", function () { request(false).catch(function () {}); });
      var drawing = window.BearStackFaceDrawing.init({
        onBusy: function (value) { editing = value; setBusy(busy); },
        onSave: function () { return request(false); }
      });
      drawButton.addEventListener("click", function () {
        if (!current || busy || editing) return;
        options.stopSlideshow();
        drawing.open(current.path, displayPath, drawButton);
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
