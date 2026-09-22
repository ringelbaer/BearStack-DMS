(function () {
  "use strict";
  var dialog = document.querySelector("[data-image-group-dialog]");
  var opener;
  if (dialog) {
    var create = document.querySelector("[data-image-group-create]");
    var form = document.querySelector("[data-photo-bulk-form]");
    function selected() { return Array.from(form.querySelectorAll('input[name="ids"]:checked')).map(function (input) { return input.closest("[data-photo-item]"); }); }
    function groups(items) { return items.filter(function (item) { return Number(item.dataset.imageGroupId); }); }
    function eligible(items) { return items.length >= 2 && items.length <= 500 && groups(items).length <= 1 && items.every(function (item) { return item.dataset.photoType === "image"; }); }
    function update() {
      var items = selected(), hint = document.querySelector("[data-image-group-hint]");
      var groupCount = groups(items).length;
      create.disabled = !eligible(items);
      create.textContent = groupCount === 1 ? "Zur Bildgruppe hinzufügen …" : "Bilder gruppieren …";
      if (!hint) return;
      hint.dataset.contextHelpTitle = groupCount === 1 ? "Zur Bildgruppe hinzufügen" : "Ausgewählte Bilder gruppieren";
      hint.dataset.contextHelp = items.length > 500 ? "Bitte höchstens 500 Bilder auswählen." :
        groupCount > 1 ? "Bitte höchstens eine Bildgruppe auswählen. Mehrere Bildgruppen können nicht zusammengeführt werden." :
        items.some(function (item) { return item.dataset.photoType !== "image"; }) ? "Bildgruppen können nur Bilder enthalten. Bitte Videos und Audios abwählen." :
        groupCount === 1 ? "Eine Bildgruppe und einzelne Bilder auswählen. Die ausgewählten Bilder werden zur Gruppe hinzugefügt; das bisherige Hauptbild bleibt erhalten. Eine Gruppe darf insgesamt höchstens 500 Bilder enthalten." :
        "Zum Erstellen einer Bildgruppe mindestens zwei und höchstens 500 ungruppierte Bilder auswählen. Im nächsten Schritt das Hauptbild über die Vorschaubilder wählen. Eine bestehende Bildgruppe und einzelne Bilder auswählen, um sie hinzuzufügen. Originaldateien bleiben unverändert.";
    }
    if (create && form) {
      var choices = dialog.querySelector("[data-image-group-primary]");
      var inputs = dialog.querySelector("[data-image-group-inputs]");
      var preview = dialog.querySelector("[data-image-group-preview]");
      var previewStatus = dialog.querySelector("[data-image-group-preview-status]");
      var dialogForm = dialog.querySelector("form"), submit = dialog.querySelector("[data-image-group-submit]");
      var formStatus = dialog.querySelector("[data-image-group-form-status]");
      var readRequest = null, ready = false;
      function hiddenInput(name, value) {
        var input = document.createElement("input"); input.type = "hidden"; input.name = name; input.value = value; inputs.append(input);
      }
      async function loadGroup(group, additions) {
        var controller = new AbortController(); readRequest = controller;
        var timer = window.setTimeout(function () { controller.abort(); }, 10000);
        formStatus.textContent = "Bildgruppe wird geladen …";
        try {
          var response = await fetch(dialogForm.getAttribute("action") + "?format=json", { signal: controller.signal, credentials: "same-origin", headers: { Accept: "application/json" } });
          if (!response.ok || response.redirected) throw new Error("Bildgruppe konnte nicht geladen werden. Bitte die Galerie neu laden.");
          var data = await response.json();
          if (readRequest !== controller || !dialog.open) return;
          var primary = data.members && data.members.find(function (member) { return member.primary; });
          if (!primary || primary.path !== group.dataset.photoPath || data.id !== Number(group.dataset.imageGroupId) || !Number.isSafeInteger(data.revision) || data.revision < 1) throw new Error("Die Bildgruppe wurde inzwischen geändert. Bitte die Galerie neu laden.");
          if (data.members.length + additions > 500) throw new Error("Eine Bildgruppe darf insgesamt höchstens 500 Bilder enthalten. Bitte weniger Bilder auswählen.");
          hiddenInput("revision", data.revision);
          formStatus.textContent = ""; ready = true; submit.disabled = false;
        } catch (error) {
          if (readRequest === controller && dialog.open) formStatus.textContent = error.name === "AbortError" ? "Das Laden dauert zu lange. Schließe die Auswahl und versuche es erneut." : error.message;
        } finally {
          window.clearTimeout(timer);
          if (readRequest === controller) readRequest = null;
        }
      }
      function thumbnail(item) {
        var image = item.querySelector("[data-photo-thumb-image]");
        return item.dataset.photoThumb || (image && image.dataset.photoThumbSrc) || "";
      }
      function showPrimary(item) {
        var img = document.createElement("img"), thumb = thumbnail(item);
        var src = item.dataset.imageGroupPreview || thumb;
        img.alt = "Hauptbild: " + item.dataset.photoDisplayPath;
        img.decoding = "async";
        previewStatus.textContent = "Bildvorschau wird geladen …";
        img.onload = function () {
          if (preview.contains(img)) previewStatus.textContent = "";
        };
        img.onerror = function () {
          if (!preview.contains(img)) return;
          if (thumb && src !== thumb) { src = thumb; img.src = thumb; return; }
          previewStatus.textContent = "Bildvorschau konnte nicht geladen werden. Wähle ein anderes Bild oder öffne die Auswahl erneut.";
        };
        preview.replaceChildren(img);
        dialog.querySelector("[data-image-group-preview-path]").textContent = item.dataset.photoDisplayPath;
        img.src = src;
      }
      create.hidden = false;
      form.addEventListener("photo-selection-changed", update);
      create.addEventListener("click", function () {
        var items = selected(); if (!eligible(items)) return;
        var group = groups(items)[0], adding = Boolean(group);
        var candidates = items.filter(function (item) { return !Number(item.dataset.imageGroupId); });
        opener = create;
        choices.replaceChildren(); inputs.replaceChildren();
        ready = !adding; submit.disabled = adding; formStatus.textContent = "";
        dialogForm.setAttribute("action", adding ? "/photos/image-groups/" + Number(group.dataset.imageGroupId) : "/photos/image-groups");
        if (!adding) hiddenInput("return", form.querySelector('input[name="return"]').value);
        dialog.querySelector("#image-group-create-title").textContent = adding ? "Bilder zur Bildgruppe hinzufügen" : "Ausgewählte Bilder gruppieren";
        dialog.querySelector("[data-image-group-description]").textContent = adding ? "Diese Bilder werden zur ausgewählten Bildgruppe hinzugefügt. Das bisherige Hauptbild und alle Originaldateien bleiben erhalten." : "Klicke auf das Vorschaubild, das die Gruppe vertreten soll. Alle Originalbilder bleiben erhalten.";
        dialog.querySelector("[data-image-group-preview-title]").textContent = adding ? "Bisheriges Hauptbild bleibt erhalten" : "Gewähltes Hauptbild";
        dialog.querySelector("[data-image-group-choice-title]").textContent = adding ? "Hinzuzufügende Bilder" : "Hauptbild wählen";
        submit.textContent = adding ? "Zur Bildgruppe hinzufügen" : "Bildgruppe erstellen";
        dialog.dataset.adding = String(adding);
        if (adding) hiddenInput("action", "add");
        var fragment = document.createDocumentFragment();
        candidates.forEach(function (item, index) {
          var label = document.createElement(adding ? "div" : "label"); label.className = "image-group-choice";
          var img = document.createElement("img"); img.alt = ""; img.loading = "lazy"; img.decoding = "async"; img.src = thumbnail(item);
          var caption = document.createElement("span"); caption.className = "image-group-choice-path"; caption.textContent = item.dataset.photoDisplayPath;
          if (!adding) {
            var radio = document.createElement("input"); radio.type = "radio"; radio.name = "primary"; radio.value = item.dataset.photoPath; radio.required = true; radio.checked = index === 0;
            var badge = document.createElement("span"); badge.className = "image-group-choice-badge"; badge.textContent = "✓ Hauptbild"; badge.setAttribute("aria-hidden", "true");
            radio.addEventListener("change", function () { if (radio.checked) showPrimary(item); });
            label.append(radio, img, caption, badge);
          } else { label.append(img, caption); }
          fragment.append(label); hiddenInput("ids", item.dataset.photoPath);
        });
        choices.append(fragment);
        dialog.querySelector("[data-image-group-summary]").textContent = adding ? candidates.length + (candidates.length === 1 ? " Bild wird hinzugefügt." : " Bilder werden hinzugefügt.") : candidates.length + " ausgewählte Bilder werden zu einer Bildgruppe zusammengefasst.";
        dialog.showModal();
        showPrimary(group || candidates[0]);
        if (adding) loadGroup(group, candidates.length);
        else choices.querySelector("input:checked").focus({ preventScroll: true });
        dialog.querySelector(".app-dialog-body").scrollTop = 0;
      });
      dialogForm.addEventListener("submit", function (event) { if (!ready) event.preventDefault(); });
      dialog.querySelector("[data-image-group-cancel]").addEventListener("click", function () { dialog.close(); });
      dialog.addEventListener("close", function () {
        if (readRequest) { readRequest.abort(); readRequest = null; }
        ready = false;
        preview.replaceChildren(); choices.replaceChildren(); inputs.replaceChildren();
        if (opener) opener.focus();
      });
      update();
    }
  }
  document.querySelectorAll("[data-image-group-confirm]").forEach(function (form) {
    var approved = false, pending = false;
    form.addEventListener("submit", async function (event) {
      if (approved) return;
      event.preventDefault(); if (pending) return;
      pending = true;
      try {
        if (await showAppConfirm(form.dataset.imageGroupConfirm, "Bildgruppe ändern")) { approved = true; form.requestSubmit(event.submitter); }
      } finally { pending = false; }
    });
  });
}());
