(function () {
  "use strict";

  // Shared naming/assignment controller. Each page owns its selection, loading
  // state and refresh; the dialog connects the picker to submission and focus restoration.
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

  // The same 300% face-context preview is used by every naming dialog.
  function bindPreview(personDialog) {
    if (personDialog.bearstackPreview) return personDialog.bearstackPreview;
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
    function showPreview(card, displayPath) {
      if (!preview) return;
      previewRegion = null;
      if (displayPath === undefined) displayPath = card.dataset.displayPath;
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
    personDialog.addEventListener("close", function () {
      if (previewPath) { previewPath.textContent = ""; previewPath.hidden = true; }
      previewRegion = null;
      if (preview) { previewImage.removeAttribute("src"); previewImage.style.transform = ""; previewBox.hidden = true; }
    });
    personDialog.bearstackPreview = { show: showPreview };
    return personDialog.bearstackPreview;
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
    var photoPreview = bindPreview(personDialog);
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
      dialogForm.dataset.personFaceId = faceCard.dataset.searchFaceId || faceCard.dataset.groupFace || faceCard.dataset.faceId || "";
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
      var previewCard = sourceCard || card;
      photoPreview.show(previewCard, options.getPreviewPath ? options.getPreviewPath(previewCard) : undefined);
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
      var saveController = options.requestTimeoutMS ? new AbortController() : null;
      var saveTimer = saveController ? setTimeout(function () { saveController.abort(); }, options.requestTimeoutMS) : null;
      try {
        var response = await fetch(action, {
          method: "POST", credentials: "same-origin", redirect: "error",
          headers: { Accept: "application/json" }, body: body,
          signal: saveController ? saveController.signal : undefined
        });
        if (!response.ok) {
          if (ignoring && response.status === 409 && options.onIgnoreConflict) {
            conflict = true;
            await options.onIgnoreConflict();
            throw new Error("Das Gruppenbild wurde inzwischen geändert. Bitte den Dialog schließen und die aktualisierte Auswahl erneut prüfen.");
          }
          var saveError = new Error((ignoring ? "Gesicht konnte nicht ignoriert werden" : "Person konnte nicht gespeichert werden") + " (HTTP " + response.status + ").");
          saveError.status = response.status;
          throw saveError;
        }
        var result = await response.json();
        if (result.ok !== true) throw new Error("Person konnte nicht gespeichert werden.");
        saved = true;
        await options.onSave(dialogIDs, { action: action, body: body, ignoring: ignoring, result: result });
        status.textContent = options.savedMessage ? options.savedMessage(ignoring) : ignoring ? (dialogIDs.length > 1 ? "Angezeigte Gesichter ignoriert." : "Gesicht ignoriert.") : (merging ? "Personen zusammengeführt." : "Person benannt.");
        personDialog.close();
      } catch (error) {
        if (options.onSaveError) conflict = options.onSaveError(error, saved) === true || conflict;
        dialogStatus.textContent = saved ? "Gespeichert, aber die Ansicht konnte nicht aktualisiert werden. Bitte die Seite neu laden." : error.message;
        // Do not offer the same mutation again after a successful save.
        if (saved && opener && opener.isConnected) opener.disabled = true;
      } finally {
        if (saveTimer !== null) clearTimeout(saveTimer);
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

  window.BearStackPersonDialog = { bind: bind, bindPreview: bindPreview, createEditButton: createEditButton };

  // Adapter for the shipped forms. The picker itself has no route, button-label
  // or submit policy and can also be used by a component without a form.
  function initializePersonPickers(root) {
    root.querySelectorAll("[data-person-picker]").forEach(function (form) {
      if (form.dataset.personPickerReady) return;
      form.dataset.personPickerReady = "1";
      var input = form.querySelector("[data-person-search]");
      var allowCreate = form.hasAttribute("data-person-create");
      var renameAction = form.dataset.renameAction || form.getAttribute("action");
      function validate(state) {
        if (form.hasAttribute("data-person-restore")) {
          form.querySelector("[data-person-submit]").textContent = state.assigned ? "Zuordnen und wiederherstellen" : "Benennen und wiederherstellen";
          input.setCustomValidity(state.assigned || state.name ? "" : "Bitte einen Namen eingeben oder eine Person auswählen.");
        } else if (form.hasAttribute("data-person-face-selection")) {
          var subject = form.dataset.personFaceSelection === "single" ? "Gesicht" : "Auswahl";
          form.querySelector("[data-person-submit]").textContent = state.assigned ? subject + " zuordnen" : state.name ? subject + " benennen" : "Als neue Gruppe abtrennen";
          input.setCustomValidity("");
        } else if (form.hasAttribute("data-person-manual-create")) {
          input.setCustomValidity(state.assigned || state.name ? "" : "Bitte einen Namen eingeben oder eine Person auswählen.");
        } else if (allowCreate) {
          if (renameAction) form.action = state.assigned ? renameAction.replace(/\/rename$/, "/merge") : renameAction;
          form.querySelector("[data-person-submit]").textContent = state.assigned ? "Gruppen zusammenführen" : (Number(form.dataset.personCount) > 1 ? "Benennen und zusammenführen" : "Benennen");
          input.setCustomValidity(Number(form.dataset.personCount) > 1 && !state.assigned && !state.name ? "Bitte einen Namen eingeben oder eine Person auswählen." : "");
        } else {
          input.setCustomValidity(state.target && (state.target !== "0" || !input.required) ? "" : "Bitte eine Person aus den Vorschlägen auswählen.");
        }
      }
      var picker = window.BearStackPersonPicker.bind(form, {
        allowCreate: allowCreate,
        showThumbnails: form.hasAttribute("data-person-modal"),
        context: function () { return { faceID: form.dataset.personFaceId, exclude: form.dataset.personExclude,
          suggestionsURL: form.dataset.personSuggestionsUrl }; },
        onChange: validate,
        onChoose: function () { if (form.hasAttribute("data-person-modal")) form.requestSubmit(); }
      });
      form.addEventListener("person-picker-reset", function (event) {
        renameAction = form.dataset.renameAction;
        picker.reset(event.detail && event.detail.name);
      });
      form.addEventListener("person-picker-close", picker.close);
    });
  }
  window.initializePersonPickers = initializePersonPickers;
  initializePersonPickers(document);
}());
