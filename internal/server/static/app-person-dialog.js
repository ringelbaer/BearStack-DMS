(function () {
  "use strict";

  // Shared naming/assignment controller. Each page owns its selection, loading
  // state and refresh; the dialog owns search, submission and focus restoration.
  function createEditButton(label) {
    var edit = document.createElement("button");
    edit.type = "button"; edit.className = "person-edit-button secondary-button";
    edit.dataset.personEdit = ""; edit.title = "Benennen / zuordnen";
    var icon = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    icon.setAttribute("viewBox", "0 0 24 24"); icon.setAttribute("width", "16"); icon.setAttribute("height", "16");
    icon.setAttribute("aria-hidden", "true"); icon.setAttribute("fill", "none");
    icon.setAttribute("stroke", "currentColor"); icon.setAttribute("stroke-width", "1.8");
    icon.setAttribute("stroke-linecap", "round"); icon.setAttribute("stroke-linejoin", "round");
    var path = document.createElementNS("http://www.w3.org/2000/svg", "path");
    path.setAttribute("d", "m16 3 5 5M3 21l5-1L21 7a2 2 0 0 0-5-5L3 15Z");
    icon.append(path); edit.append(icon);
    edit.setAttribute("aria-label", label);
    return edit;
  }

  function bind(options) {
    var personSurface = options.surface;
    var status = options.status;
    var personDialog = document.querySelector("[data-person-dialog]");
    if (!personDialog || !personSurface) return { open: function () {} };
    var busy = false, owner = {};
    function ownsDialog() { return personDialog.bearstackOwner === owner; }
    var dialogForm = personDialog.querySelector("form");
    var dialogStatus = personDialog.querySelector("[data-person-dialog-status]");
    var ignoreButton = personDialog.querySelector("[data-person-dialog-ignore]");
    var ignoreRequest;
    var preview = personDialog.querySelector("[data-person-preview]");
    var previewImage = personDialog.querySelector("[data-person-preview-image]");
    var previewBox = personDialog.querySelector("[data-person-preview-box]");
    var previewStatus = personDialog.querySelector("[data-person-preview-status]");
    var previewPath = personDialog.querySelector("[data-person-preview-path]");
    var previewRegion;
    function drawPreview() {
      if (!preview || !previewRegion || !previewImage.complete || !previewImage.naturalWidth) return;
      var frame = preview.getBoundingClientRect();
      if (!frame.width || !frame.height) return;
      var scale = Math.min(frame.width / previewImage.naturalWidth, frame.height / previewImage.naturalHeight);
      var width = previewImage.naturalWidth * scale, height = previewImage.naturalHeight * scale;
      var baseLeft = (frame.width - width) / 2, baseTop = (frame.height - height) / 2;
      var zoom = Math.max(1, Math.min(frame.width / (3 * previewRegion.width * width), frame.height / (3 * previewRegion.height * height)));
      width *= zoom; height *= zoom;
      var left = frame.width / 2 - (previewRegion.x + previewRegion.width / 2) * width;
      var top = frame.height / 2 - (previewRegion.y + previewRegion.height / 2) * height;
      left = width <= frame.width ? (frame.width - width) / 2 : Math.max(frame.width - width, Math.min(0, left));
      top = height <= frame.height ? (frame.height - height) / 2 : Math.max(frame.height - height, Math.min(0, top));
      previewImage.style.transform = "translate(" + (left - baseLeft * zoom) + "px, " + (top - baseTop * zoom) + "px) scale(" + zoom + ")";
      previewBox.style.left = (left + previewRegion.x * width) + "px";
      previewBox.style.top = (top + previewRegion.y * height) + "px";
      previewBox.style.width = (previewRegion.width * width) + "px";
      previewBox.style.height = (previewRegion.height * height) + "px";
      previewBox.hidden = false;
    }
    function showPreview(card) {
      if (!preview) return;
      previewRegion = null;
      var displayPath = options.getPreviewPath ? options.getPreviewPath(card) : card.dataset.displayPath;
      if (previewPath) { previewPath.textContent = displayPath || ""; previewPath.hidden = !displayPath; }
      previewBox.hidden = true;
      previewImage.style.transform = "";
      previewImage.hidden = true;
      previewImage.removeAttribute("src");
      var id = card.dataset.groupFace || card.dataset.faceId;
      var x = Number(card.dataset.x), y = Number(card.dataset.y), w = Number(card.dataset.width), h = Number(card.dataset.height);
      var left = Math.max(0, Math.min(1, x)), top = Math.max(0, Math.min(1, y));
      var right = Math.max(left, Math.min(1, x + w)), bottom = Math.max(top, Math.min(1, y + h));
      if (!id || ![x,y,w,h].every(Number.isFinite) || w <= 0 || h <= 0 || right <= left || bottom <= top) {
        previewStatus.textContent = "Keine Fotovorschau verfügbar.";
        return;
      }
      previewRegion = { x: left, y: top, width: right - left, height: bottom - top };
      previewStatus.textContent = "Fotovorschau wird geladen …";
      previewImage.hidden = false;
      previewImage.src = "/photos/people/groups/image/" + encodeURIComponent(id);
    }
    if (preview) {
      previewImage.addEventListener("load", function () { previewStatus.textContent = ""; drawPreview(); });
      previewImage.addEventListener("error", function () {
        if (!previewImage.getAttribute("src")) return;
        previewBox.hidden = true; previewImage.hidden = true;
        previewStatus.textContent = "Fotovorschau konnte nicht geladen werden.";
      });
      if (window.ResizeObserver) new ResizeObserver(drawPreview).observe(preview);
      else window.addEventListener("resize", drawPreview);
    }
    var opener, sourceCard, dialogIDs = [];
    function openPersonDialog(ids, button) {
      if (!ids.length || busy || options.isBusy() || button.disabled) return;
      if (personDialog.open && !ownsDialog()) return;
      personDialog.bearstackOwner = owner;
      dialogIDs = ids;
      var card = personSurface.querySelector('[data-person-id="' + ids[0] + '"]');
      if (!card) return;
      opener = button;
      var faceCard = (options.getPreviewCard ? options.getPreviewCard() : null) || button.closest("[data-person-id]") || card;
      delete dialogForm.dataset.personFaceSelection;
      delete dialogForm.dataset.personRestore;
      ignoreButton.hidden = false;
      dialogForm.querySelector("[data-person-face-match]").hidden = false;
      dialogForm.dataset.personModal = "";
      dialogForm.dataset.personFaceId = faceCard.dataset.groupFace || faceCard.dataset.faceId || "";
      dialogForm.dataset.personExclude = ids.length === 1 ? ids[0] : "";
      dialogForm.dataset.personCount = String(ids.length);
      dialogForm.dataset.renameAction = "/photos/people/" + encodeURIComponent(ids[0]) + "/rename";
      dialogForm.querySelectorAll("input, button").forEach(function (control) { control.disabled = false; });
      dialogStatus.textContent = "";
      personDialog.querySelector("#person-dialog-title").textContent = ids.length > 1 ? ids.length + " Gruppen benennen oder zuordnen" : "Person benennen oder zuordnen";
      personDialog.querySelector("#overview-person-hint").textContent = ids.length > 1 ? "Alle markierten Gruppen werden unter dem neuen Namen oder mit der ausgewählten Person zusammengeführt." : "Ein neuer Name benennt diese Gruppe. Eine vorhandene Person auswählen, um die gesamte Gruppe mit ihr zusammenzuführen.";
      dialogForm.dispatchEvent(new CustomEvent("person-picker-reset", { detail: { name: ids.length === 1 ? card.dataset.personName : "" } }));
      if (options.configureDialog) options.configureDialog({ dialog: personDialog, form: dialogForm, card: faceCard });
      personDialog.showModal();
      sourceCard = (options.getPreviewCard ? options.getPreviewCard() : null) || button.closest("[data-person-id]");
      if (sourceCard) sourceCard.setAttribute("data-person-dialog-source", "");
      showPreview(sourceCard || card);
      var ignoreCards = sourceCard ? [sourceCard] : ids.map(function (id) { return personSurface.querySelector('[data-person-id="' + id + '"]'); });
      ignoreRequest = options.getIgnoreRequest ? options.getIgnoreRequest(ignoreCards) : null;
      ignoreButton.disabled = !ignoreRequest;
      ignoreButton.title = ignoreRequest ? (ids.length > 1 ? "Angezeigte Gesichter der ausgewählten Gruppen ignorieren" : "Angezeigtes Gesicht ignorieren") : "Nur unbenannte, aktive Gesichter können hier ignoriert werden";
    }
    personSurface.addEventListener("click", function (event) {
      var button = event.target.closest("[data-person-edit]");
      if (!button || busy || options.isBusy() || button.disabled) return;
      var card = button.closest("[data-person-id]");
      openPersonDialog([card.dataset.personId], button);
    });
    personDialog.querySelector("[data-person-dialog-cancel]").addEventListener("click", function () { if (ownsDialog() && !busy) personDialog.close(); });
    personDialog.addEventListener("cancel", function (event) { if (ownsDialog() && busy) event.preventDefault(); });
    personDialog.addEventListener("close", function () {
      if (!ownsDialog()) return;
      if (sourceCard) sourceCard.removeAttribute("data-person-dialog-source");
      sourceCard = null;
      if (previewPath) { previewPath.textContent = ""; previewPath.hidden = true; }
      previewRegion = null;
      if (preview) { previewImage.removeAttribute("src"); previewBox.hidden = true; }
      dialogForm.dispatchEvent(new CustomEvent("person-picker-close"));
      var candidates = opener ? [opener] : [];
      candidates = candidates.concat(Array.from(personSurface.querySelectorAll("a, button:not([disabled])")));
      var focus = candidates.find(function (control) { return control.isConnected && !control.disabled && !control.closest("[hidden]") && control.getClientRects().length; });
      if (!focus) focus = document.querySelector('.people-filter-tabs [aria-current="page"], [data-group-unnamed]');
      if (focus) focus.focus({ preventScroll: true });
    });
    async function savePerson(ignoring) {
      if (!ownsDialog() || busy || (ignoring && (!ignoreRequest || ignoreButton.disabled))) return;
      var body = ignoring ? ignoreRequest.body : new URLSearchParams(new FormData(dialogForm));
      var action = ignoring ? ignoreRequest.action : dialogForm.action;
      var merging = action.endsWith("/merge");
      if (!ignoring && dialogIDs.length > 1) {
        var targetID = body.get("target");
        if (!targetID || targetID === "0") {
          targetID = dialogIDs[0];
          body.set("new_name", body.get("name").trim());
        }
        var sources = dialogIDs.filter(function (id) { return id !== targetID; });
        body.set("target", targetID);
        sources.slice(1).forEach(function (id) { body.append("person_id", id); });
        action = "/photos/people/" + encodeURIComponent(sources[0]) + "/merge";
        merging = true;
      }

      if (!ignoring && options.getSaveRequest) {
        var custom = options.getSaveRequest({ action: action, body: body });
        if (!custom) return;
        action = custom.action; body = custom.body;
        merging = action.endsWith("/merge");
      }
      busy = true;
      options.onBusy(busy);
      dialogForm.dispatchEvent(new CustomEvent("person-picker-close"));
      dialogForm.querySelectorAll("input, button").forEach(function (control) { control.disabled = true; });
      personDialog.setAttribute("aria-busy", "true");
      dialogStatus.textContent = ignoring ? "Gesicht wird ignoriert …" : "Person wird gespeichert …";
      var saved = false, conflict = false;
      try {
        var response = await fetch(action, {
          method: "POST", credentials: "same-origin", redirect: "error",
          headers: { Accept: "application/json" }, body: body
        });
        if (!response.ok) {
          if (ignoring && response.status === 409 && options.onIgnoreConflict) {
            conflict = true;
            await options.onIgnoreConflict();
            throw new Error("Das Gruppenbild wurde inzwischen geändert. Bitte den Dialog schließen und die aktualisierte Auswahl erneut prüfen.");
          }
          throw new Error((ignoring ? "Gesicht konnte nicht ignoriert werden" : "Person konnte nicht gespeichert werden") + " (HTTP " + response.status + ").");
        }
        var result = await response.json();
        if (result.ok !== true) throw new Error("Person konnte nicht gespeichert werden.");
        saved = true;
        await options.onSave(dialogIDs, { action: action, body: body, ignoring: ignoring });
        status.textContent = options.savedMessage ? options.savedMessage(ignoring) : ignoring ? (dialogIDs.length > 1 ? "Angezeigte Gesichter ignoriert." : "Gesicht ignoriert.") : (merging ? "Personen zusammengeführt." : "Person benannt.");
        personDialog.close();
      } catch (error) {
        if (options.onSaveError) conflict = options.onSaveError(error, saved) === true || conflict;
        dialogStatus.textContent = saved ? "Gespeichert, aber die Ansicht konnte nicht aktualisiert werden. Bitte die Seite neu laden." : error.message;
        // Do not offer the same mutation again after a successful save.
        if (saved && opener && opener.isConnected) opener.disabled = true;
      } finally {
        busy = false;
        options.onBusy(busy);
        personDialog.removeAttribute("aria-busy");
        dialogForm.querySelectorAll("input, button").forEach(function (control) { control.disabled = saved || conflict; });
        ignoreButton.disabled = saved || conflict || !ignoreRequest;
        personDialog.querySelector("[data-person-dialog-cancel]").disabled = false;
      }
    }
    dialogForm.addEventListener("submit", function (event) { event.preventDefault(); savePerson(false); });
    ignoreButton.addEventListener("click", function () { savePerson(true); });
    return { open: openPersonDialog };
  }

  window.BearStackPersonDialog = { bind: bind, createEditButton: createEditButton };

  document.querySelectorAll("[data-person-picker]").forEach(function (form) {
    var input = form.querySelector("[data-person-search]");
    var target = form.querySelector("[data-person-target]");
    var list = form.querySelector("[data-person-options]");
    var popup = form.querySelector("[data-person-popup]");
    var feedback = form.querySelector("[data-person-feedback]");
    var matchButton = form.querySelector("[data-person-face-match]");
    var matching = false;
    var allowCreate = form.hasAttribute("data-person-create");
    var renameAction = allowCreate ? form.action : "";
    var items = [], active = -1, revision = 0, pointerPerson;
    var timer, controller, pendingDirection, renderFrame, pendingRender;

    function cancel() {
      clearTimeout(timer);
      if (renderFrame) cancelAnimationFrame(renderFrame);
      renderFrame = 0;
      pendingRender = null;
      if (controller) controller.abort();
      revision++;
      matching = false;
      if (matchButton) matchButton.removeAttribute("aria-busy");
    }
    function close() {
      cancel();
      popup.hidden = true;
      input.setAttribute("aria-expanded", "false");
      input.removeAttribute("aria-activedescendant");
      active = -1;
      pointerPerson = undefined;
    }
    function validate() {
      if (form.hasAttribute("data-person-restore")) {
        var assigned = target.value && target.value !== "0";
        form.querySelector("[data-person-submit]").textContent = assigned ? "Zuordnen und wiederherstellen" : "Benennen und wiederherstellen";
        input.setCustomValidity(assigned || input.value.trim() ? "" : "Bitte einen Namen eingeben oder eine Person auswählen.");
        return;
      }
      if (form.hasAttribute("data-person-face-selection")) {
        form.querySelector("[data-person-submit]").textContent = target.value && target.value !== "0" ? "Auswahl zuordnen" : input.value.trim() ? "Auswahl benennen" : "Als neue Gruppe abtrennen";
        input.setCustomValidity("");
        return;
      }
      if (form.hasAttribute("data-person-manual-create")) {
        input.setCustomValidity((target.value && target.value !== "0") || input.value.trim() ? "" : "Bitte einen Namen eingeben oder eine Person auswählen.");
        return;
      }
      if (allowCreate) {
        var merging = target.value && target.value !== "0";
        form.action = merging ? renameAction.replace(/\/rename$/, "/merge") : renameAction;
        form.querySelector("[data-person-submit]").textContent = merging ? "Gruppen zusammenführen" : (Number(form.dataset.personCount) > 1 ? "Benennen und zusammenführen" : "Benennen");
        input.setCustomValidity(Number(form.dataset.personCount) > 1 && !merging && !input.value.trim() ? "Bitte einen Namen eingeben oder eine Person auswählen." : "");
        return;
      }
      input.setCustomValidity(target.value && (target.value !== "0" || !input.required) ? "" :
        "Bitte eine Person aus den Vorschlägen auswählen.");
    }
    function activate(index) {
      active = index;
      Array.from(list.children).forEach(function (option, i) {
        option.setAttribute("aria-selected", String(i === active));
      });
      if (active >= 0) {
        var option = list.children[active];
        input.setAttribute("aria-activedescendant", option.id);
        option.scrollIntoView({ block: "nearest" });
      } else {
        input.removeAttribute("aria-activedescendant");
      }
    }
    function choose(index) {
      var person = items[index];
      if (!person || input.disabled || popup.hidden) return;
      target.value = String(person.id);
      target.dataset.revision = String(person.revision || 0);
      input.value = person.create ? person.name : (person.name || "Unbenannt") + " (#" + person.id + ")";
      validate();
      close();
      if (form.hasAttribute("data-person-modal")) form.requestSubmit();
    }
    async function load(direction, faceMatch) {
      cancel();
      matching = !!faceMatch;
      if (matchButton && matching) matchButton.setAttribute("aria-busy", "true");
      pendingDirection = direction;
      var request = revision;
      controller = new AbortController();
      list.replaceChildren();
      items = [];
      activate(-1);
      popup.hidden = false;
      input.setAttribute("aria-expanded", "true");
      feedback.textContent = faceMatch ? "Gesicht wird mit benannten Personen abgeglichen …" : "Personen werden geladen …";
      // A selected label contains the ID; opening it again lists alternatives.
      var query = target.value && target.value !== "0" ? "" : input.value.trim();
      try {
        var url = faceMatch ? "/photos/faces/" + encodeURIComponent(form.dataset.personFaceId) + "/suggestions" : (form.dataset.personSuggestionsUrl || "/photos/people?format=suggestions&q=") + encodeURIComponent(query);
        var response = await fetch(url, {
          signal: controller.signal, credentials: "same-origin", redirect: "error",
          headers: { Accept: faceMatch ? "application/x-ndjson" : "application/json" }
        });
        if (!response.ok) throw new Error(faceMatch ? "Gesichtsabgleich fehlgeschlagen. Bitte erneut versuchen oder die Ansicht aktualisieren." : "Personen konnten nicht geladen werden. Bitte erneut suchen.");
        function render(data, pending) {
        if (data.error) throw new Error(data.error);
        if (!Array.isArray(data.people)) throw new Error("Ungültige Antwort der Personensuche.");
        if (request !== revision || document.activeElement !== input) return;
        var activeID = active >= 0 && items[active] ? String(items[active].id) : null;
        var previous = new Map(Array.from(list.children).map(function (node) { return [node.dataset.personKey, node]; }));
        var scrollTop = popup.scrollTop;
        items = data.people.filter(function (person) { return typeof person.name === "string" && person.name.trim() && String(person.id) !== form.dataset.personExclude; });
        if (allowCreate && query && !faceMatch) items.push({ id: 0, name: query, create: true });
        var options = document.createDocumentFragment();
        items.forEach(function (person, index) {
          var key = JSON.stringify(person);
          var option = previous.get(key);
          if (option) {
            option.id = list.id + "-" + index;
            option.dataset.personOption = String(index);
            options.append(option);
            return;
          }
          option = document.createElement("div");
          option.dataset.personKey = key;
          option.id = list.id + "-" + index;
          option.dataset.personOption = String(index);
          option.setAttribute("role", "option");
          option.setAttribute("aria-selected", "false");
          var label = document.createElement("span");
          label.textContent = person.create ? "Neu anlegen: „" + person.name + "“" : (person.name || "Unbenannt") + " (#" + person.id + ", " +
            person.count + (person.count === 1 ? " Foto)" : " Fotos)");
          if (form.hasAttribute("data-person-modal") && !person.create && Number.isSafeInteger(person.face_id) && person.face_id > 0) {
            var thumbnail = document.createElement("img");
            thumbnail.className = "person-picker-thumbnail";
            thumbnail.width = 40; thumbnail.height = 40;
            thumbnail.alt = ""; thumbnail.loading = "lazy"; thumbnail.decoding = "async";
            // A missing preview must not hide or disable the person's name.
            thumbnail.addEventListener("error", function () { this.style.visibility = "hidden"; }, { once: true });
            thumbnail.src = "/photos/faces/" + encodeURIComponent(person.face_id) + "/thumbnail";
            option.append(thumbnail);
          }
          option.append(label);
          options.append(option);
        });
        list.replaceChildren(options);
        popup.scrollTop = scrollTop;
        feedback.textContent = pending ? items.length + " Kandidaten gefunden – Abgleich läuft …" : data.has_next ? "Weitere Personen vorhanden. Bitte die Suche eingrenzen." :
          (items.length ? items.length + (items.length === 1 ? " Vorschlag verfügbar." : " Vorschläge verfügbar.") : (faceMatch ? "Keine ähnlichen benannten Personen gefunden." : "Keine passende Person gefunden."));
        if (activeID !== null) activate(items.findIndex(function (person) { return String(person.id) === activeID; }));
        else if (items.length && pendingDirection) { activate(pendingDirection === "last" ? items.length - 1 : 0); pendingDirection = undefined; }
        else activate(-1);
        }
        function renderStream(update) {
          if (update.error) throw new Error(update.error);
          if (!Array.isArray(update.people) || update.people.length > 20 || typeof update.done !== "boolean") throw new Error("Ungültige Antwort des Gesichtsabgleichs.");
          if (update.done) {
            if (renderFrame) cancelAnimationFrame(renderFrame);
            renderFrame = 0; pendingRender = null;
            render(update, false);
            return;
          }
          // A proxy or a busy tab can deliver many frames at once. Only the
          // newest ranking needs layout; final results and errors stay immediate.
          pendingRender = update;
          if (!renderFrame) renderFrame = requestAnimationFrame(function () {
            if (request !== revision) return;
            renderFrame = 0;
            var latest = pendingRender; pendingRender = null;
            if (!latest) return;
            try { render(latest, true); }
            catch (error) {
              cancel(); items = []; list.replaceChildren(); activate(-1);
              feedback.textContent = error.message;
            }
          });
        }
        if (faceMatch && (response.headers.get("Content-Type") || "").includes("application/x-ndjson")) {
          var reader = response.body.getReader();
          var decoder = new TextDecoder(), buffer = "", complete = false;
          try {
            while (!complete) {
              var chunk = await reader.read();
              if (request !== revision) return;
              buffer += decoder.decode(chunk.value, { stream: !chunk.done });
              var newline;
              while ((newline = buffer.indexOf("\n")) >= 0) {
                var line = buffer.slice(0, newline); buffer = buffer.slice(newline + 1);
                if (!line.trim()) continue;
                var update = JSON.parse(line);
                renderStream(update);
                if (update.done) { complete = true; break; }
              }
              if (buffer.length > 128 * 1024) throw new Error("Ungültige Antwort des Gesichtsabgleichs.");
              if (chunk.done && !complete) throw new Error("Gesichtsabgleich unterbrochen. Bitte erneut versuchen.");
            }
          } finally { await reader.cancel().catch(function () {}); }
        } else {
          render(await response.json(), false);
        }
      } catch (error) {
        if (request === revision && error.name !== "AbortError") {
          if (renderFrame) cancelAnimationFrame(renderFrame);
          renderFrame = 0; pendingRender = null;
          if (faceMatch) { items = []; list.replaceChildren(); activate(-1); }
          feedback.textContent = error.message;
        }
      } finally {
        if (request === revision) { matching = false; if (matchButton) matchButton.removeAttribute("aria-busy"); }
      }
    }
    if (matchButton) {
      matchButton.addEventListener("pointerdown", function (event) { event.preventDefault(); });
      matchButton.addEventListener("click", function () {
        if (input.disabled || matching || !form.dataset.personFaceId) return;
        input.focus({ preventScroll: true });
        target.value = "";
        validate();
        load(undefined, true);
      });
    }
    input.addEventListener("focus", function () { load(); });
    input.addEventListener("click", function () { if (popup.hidden) load(); });
    input.addEventListener("input", function (event) {
      close();
      target.value = !input.required && !input.value.trim() ? "0" : "";
      validate();
      if (!event.isComposing) timer = setTimeout(load, 250);
    });
    input.addEventListener("keydown", function (event) {
      if (event.isComposing) return;
      if (event.key === "ArrowDown" || event.key === "ArrowUp") {
        event.preventDefault();
        var down = event.key === "ArrowDown";
        if (popup.hidden) load(down ? "first" : "last");
        else if (items.length) activate(active < 0 ? (down ? 0 : items.length - 1) :
          (active + (down ? 1 : -1) + items.length) % items.length);
        else pendingDirection = down ? "first" : "last";
      } else if (event.key === "Enter" && !popup.hidden && active >= 0) {
        event.preventDefault();
        choose(active);
      } else if (event.key === "Escape" && !popup.hidden) {
        event.preventDefault();
        event.stopPropagation();
        close();
      }
    });
    // Keep keyboard focus on the combobox when clicking or tapping an option.
    list.addEventListener("pointerdown", function (event) {
      var option = event.target.closest("[data-person-option]");
      if (option) {
        pointerPerson = items[Number(option.dataset.personOption)].id;
        event.preventDefault();
      }
    });
    list.addEventListener("click", function (event) {
      var option = event.target.closest("[data-person-option]");
      if (option) choose(pointerPerson !== undefined ? items.findIndex(function (person) { return person.id === pointerPerson; }) : Number(option.dataset.personOption));
      pointerPerson = undefined;
    });
    form.addEventListener("person-picker-reset", function (event) {
      close();
      renameAction = form.dataset.renameAction;
      target.value = "";
      input.value = event.detail.name || "";
      validate();
    });
    form.addEventListener("person-picker-close", close);
    input.addEventListener("blur", close);
    validate();
  });
}());
