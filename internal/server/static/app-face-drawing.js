(function () {
  "use strict";
  window.BearStackFaceDrawing = {
    init: function (options) {
      var dialog = document.querySelector("[data-face-drawing-dialog]");
      var form = dialog.querySelector("form");
      var stage = dialog.querySelector("[data-face-drawing-stage]");
      var image = dialog.querySelector("[data-face-drawing-image]");
      var outline = dialog.querySelector("[data-face-drawing-box]");
      var existing = dialog.querySelector("[data-face-drawing-existing]");
      var status = dialog.querySelector("[data-face-drawing-status]");
      var center = dialog.querySelector("[data-face-drawing-center]");
      var submit = form.querySelector("[data-person-submit]");
      var cancel = dialog.querySelector("[data-face-drawing-cancel]");
      var path, revision, opener, region, pointer, controller;
      var generation = 0, loading = false, saving = false, saved = false;
      function clamp(value, min, max) { return Math.max(min, Math.min(max, value)); }
      function sync() {
        center.disabled = loading || saving || saved;
        cancel.disabled = saving;
        submit.disabled = loading || saving || saved || !region;
        form.querySelector("[data-person-search]").disabled = loading || saving || saved;
        dialog.setAttribute("aria-busy", String(loading || saving));
      }
      function imageRect() {
        var rect = stage.getBoundingClientRect();
        var scale = Math.min(rect.width / image.naturalWidth, rect.height / image.naturalHeight);
        var width = image.naturalWidth * scale, height = image.naturalHeight * scale;
        return { x: (rect.width - width) / 2, y: (rect.height - height) / 2, width: width, height: height, left: rect.left, top: rect.top };
      }
      function showExisting(faces) {
        var boxes = document.createDocumentFragment();
        faces.forEach(function (face) {
          var x = face.x, y = face.y, width = face.width, height = face.height;
          if (![x, y, width, height].every(Number.isFinite) || width <= 0 || height <= 0) return;
          var left = clamp(x, 0, 1), top = clamp(y, 0, 1);
          var right = clamp(x + width, left, 1), bottom = clamp(y + height, top, 1);
          if (right <= left || bottom <= top) return;
          var box = document.createElement("div");
          box.className = "face-drawing-existing-box";
          box.style.left = (left * 100) + "%"; box.style.top = (top * 100) + "%";
          box.style.width = ((right - left) * 100) + "%"; box.style.height = ((bottom - top) * 100) + "%";
          if (face.ignored) box.dataset.ignored = "";
          var label = document.createElement("span");
          label.textContent = (face.name || "Unbenannt") + (face.ignored ? " (ignoriert)" : "");
          box.title = label.textContent;
          box.append(label); boxes.append(box);
        });
        existing.replaceChildren(boxes);
      }
      function resize() {
        existing.hidden = loading || !image.naturalWidth || !existing.childElementCount;
        if (!existing.hidden) {
          // Scale one layer on resize; pointer moves only update the new region.
          var rect = imageRect();
          existing.style.left = rect.x + "px"; existing.style.top = rect.y + "px";
          existing.style.width = rect.width + "px"; existing.style.height = rect.height + "px";
        }
        draw();
      }
      function draw() {
        outline.hidden = !region;
        if (!region || !image.naturalWidth) return;
        var rect = imageRect();
        outline.style.left = (rect.x + region.x * rect.width) + "px";
        outline.style.top = (rect.y + region.y * rect.height) + "px";
        outline.style.width = (region.width * rect.width) + "px";
        outline.style.height = (region.height * rect.height) + "px";
      }
      function point(event, allowOutside) {
        var rect = imageRect();
        var x = (event.clientX - rect.left - rect.x) / rect.width;
        var y = (event.clientY - rect.top - rect.y) / rect.height;
        if (!allowOutside && (x < 0 || x > 1 || y < 0 || y > 1)) return null;
        return { x: clamp(x, 0, 1), y: clamp(y, 0, 1) };
      }
      function update(event) {
        var p = point(event, true);
        region = { x: Math.min(pointer.start.x, p.x), y: Math.min(pointer.start.y, p.y), width: Math.abs(p.x - pointer.start.x), height: Math.abs(p.y - pointer.start.y) };
        draw();
      }
      stage.addEventListener("pointerdown", function (event) {
        if (loading || saving || saved || pointer || event.button !== 0 || !image.naturalWidth) return;
        var start = point(event, false);
        if (!start) return;
        event.preventDefault();
        pointer = { id: event.pointerId, start: start, previous: region };
        stage.setPointerCapture(event.pointerId);
        submit.disabled = true;
        update(event);
      });
      stage.addEventListener("pointermove", function (event) { if (pointer && pointer.id === event.pointerId) update(event); });
      function finish(event) {
        if (!pointer || pointer.id !== event.pointerId) return;
        if (event.type === "pointercancel") region = pointer.previous;
        else {
          update(event);
          if (region.width < .002 || region.height < .002) region = null;
        }
        pointer = null;
        if (stage.hasPointerCapture(event.pointerId)) stage.releasePointerCapture(event.pointerId);
        status.textContent = region ? "Rahmen gesetzt. Namen eingeben oder Person auswählen." : "Bitte einen größeren Rahmen ziehen.";
        draw(); sync();
      }
      stage.addEventListener("pointerup", finish);
      stage.addEventListener("pointercancel", finish);
      center.addEventListener("click", function () {
        region = { x: .35, y: .35, width: .3, height: .3 };
        draw(); sync(); outline.focus();
      });
      outline.addEventListener("keydown", function (event) {
        if (!region || loading || saving || saved || !event.key.startsWith("Arrow")) return;
        event.preventDefault(); event.stopPropagation();
        var dx = event.key === "ArrowLeft" ? -.01 : event.key === "ArrowRight" ? .01 : 0;
        var dy = event.key === "ArrowUp" ? -.01 : event.key === "ArrowDown" ? .01 : 0;
        if (event.shiftKey) {
          region.width = clamp(region.width + dx, .002, 1 - region.x);
          region.height = clamp(region.height + dy, .002, 1 - region.y);
        } else {
          region.x = clamp(region.x + dx, 0, 1 - region.width);
          region.y = clamp(region.y + dy, 0, 1 - region.height);
        }
        draw();
      });
      new ResizeObserver(resize).observe(stage);
      cancel.addEventListener("click", function () { if (!saving) dialog.close(); });
      dialog.addEventListener("cancel", function (event) { if (saving) event.preventDefault(); });
      dialog.addEventListener("close", function () {
        generation++;
        if (controller) controller.abort();
        form.dispatchEvent(new CustomEvent("person-picker-close"));
        image.removeAttribute("src");
        existing.replaceChildren(); existing.hidden = true;
        pointer = null;
        options.onBusy(false);
        if (opener && opener.isConnected) opener.focus({ preventScroll: true });
      });
      form.addEventListener("submit", async function (event) {
        event.preventDefault();
        if (loading || saving || saved || pointer) return;
        if (!region) { status.textContent = "Bitte zuerst einen Rahmen um das Gesicht ziehen."; return; }
        var body = new URLSearchParams(new FormData(form));
        body.set("path", path); body.set("source_revision", revision);
        ["x", "y", "width", "height"].forEach(function (key) { body.set(key, String(region[key])); });
        form.dispatchEvent(new CustomEvent("person-picker-close"));
        saving = true; sync(); status.textContent = "Gesicht wird gespeichert …";
        controller = new AbortController();
        try {
          var response = await fetch("/photos/faces/manual", { method: "POST", credentials: "same-origin", redirect: "error", headers: { Accept: "application/json" }, body: body, signal: controller.signal });
          var result = await response.json();
          if (!response.ok || result.ok !== true) throw new Error(response.status === 409 ? "Das Foto oder die Zuordnung wurde geändert, der Rahmen ist bereits erfasst oder das Foto enthält zu viele Gesichter. Bitte abbrechen und die Auswahl neu prüfen." : "Gesicht konnte nicht gespeichert werden. Bitte erneut versuchen.");
          saved = true;
          await options.onSave();
          dialog.close();
        } catch (error) {
          status.textContent = saved ? "Gesicht gespeichert. Die Ansicht konnte nicht aktualisiert werden. Bitte schließen und Gesichter aktualisieren." : error.message;
        } finally { saving = false; sync(); }
      });
      return {
        open: async function (photoPath, displayPath, button, faces) {
          path = photoPath; opener = button; region = null; pointer = null; saved = false; loading = true;
          var token = ++generation;
          options.onBusy(true);
          form.dispatchEvent(new CustomEvent("person-picker-reset", { detail: { name: "" } }));
          dialog.querySelector("[data-face-drawing-path]").textContent = displayPath || path;
          status.textContent = "Foto wird geladen …";
          existing.hidden = true; showExisting(faces || []);
          draw(); sync(); dialog.showModal();
          dialog.querySelector(".app-dialog-body").scrollTop = 0;
          controller = new AbortController();
          try {
            var response = await fetch("/photos/faces/drawing-image?path=" + encodeURIComponent(path), { credentials: "same-origin", redirect: "error", signal: controller.signal });
            if (!response.ok) throw new Error("Foto konnte nicht geladen werden. Bitte schließen und erneut versuchen.");
            revision = response.headers.get("X-Photo-Source-Revision");
            if (!revision) throw new Error("Foto konnte nicht geprüft werden. Bitte erneut öffnen.");
            var blob = await response.blob();
            if (token !== generation) return;
            var imageURL = await new Promise(function (resolve, reject) {
              var reader = new FileReader();
              reader.onload = function () { resolve(reader.result); };
              reader.onerror = function () { reject(new Error("Foto konnte nicht gelesen werden.")); };
              reader.readAsDataURL(blob);
            });
            if (token !== generation) return;
            image.src = imageURL;
            await image.decode();
            if (token !== generation) return;
            loading = false; resize(); sync(); status.textContent = "Ziehe einen Rahmen um das Gesicht.";
          } catch (error) {
            if (token === generation && error.name !== "AbortError") status.textContent = error.message;
          }
        }
      };
    }
  };
}());
