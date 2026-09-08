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
    var busy = false;
    var dialogForm = personDialog.querySelector("form");
    var dialogStatus = personDialog.querySelector("[data-person-dialog-status]");
    var opener, sourceCard, dialogIDs = [];
    function openPersonDialog(ids, button) {
      if (!ids.length || busy || options.isBusy() || button.disabled) return;
      dialogIDs = ids;
      var card = personSurface.querySelector('[data-person-id="' + ids[0] + '"]');
      if (!card) return;
      opener = button;
      dialogForm.dataset.personExclude = ids.length === 1 ? ids[0] : "";
      dialogForm.dataset.personCount = String(ids.length);
      dialogForm.dataset.renameAction = "/photos/people/" + encodeURIComponent(ids[0]) + "/rename";
      dialogForm.querySelectorAll("input, button").forEach(function (control) { control.disabled = false; });
      dialogStatus.textContent = "";
      personDialog.querySelector("#person-dialog-title").textContent = ids.length > 1 ? ids.length + " Gruppen benennen oder zuordnen" : "Person benennen oder zuordnen";
      personDialog.querySelector("#overview-person-hint").textContent = ids.length > 1 ? "Alle markierten Gruppen werden unter dem neuen Namen oder mit der ausgewählten Person zusammengeführt." : "Ein neuer Name benennt diese Gruppe. Eine vorhandene Person auswählen, um die gesamte Gruppe mit ihr zusammenzuführen.";
      dialogForm.dispatchEvent(new CustomEvent("person-picker-reset", { detail: { name: ids.length === 1 ? card.dataset.personName : "" } }));
      personDialog.showModal();
      sourceCard = button.closest("[data-person-id]");
      if (sourceCard) sourceCard.setAttribute("data-person-dialog-source", "");
    }
    personSurface.addEventListener("click", function (event) {
      var button = event.target.closest("[data-person-edit]");
      if (!button || busy || options.isBusy() || button.disabled) return;
      var card = button.closest("[data-person-id]");
      openPersonDialog([card.dataset.personId], button);
    });
    personDialog.querySelector("[data-person-dialog-cancel]").addEventListener("click", function () { if (!busy) personDialog.close(); });
    personDialog.addEventListener("cancel", function (event) { if (busy) event.preventDefault(); });
    personDialog.addEventListener("close", function () {
      if (sourceCard) sourceCard.removeAttribute("data-person-dialog-source");
      sourceCard = null;
      dialogForm.dispatchEvent(new CustomEvent("person-picker-close"));
      var focus = opener && opener.isConnected && !opener.disabled && !opener.closest("[hidden]") ? opener : personSurface.querySelector("a, button:not([disabled])");
      if (focus) focus.focus({ preventScroll: true });
    });
    dialogForm.addEventListener("submit", async function (event) {
      event.preventDefault();
      if (busy) return;
      var body = new URLSearchParams(new FormData(dialogForm));
      var action = dialogForm.action;
      var merging = action.endsWith("/merge");
      if (dialogIDs.length > 1) {
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

      busy = true;
      options.onBusy(busy);
      dialogForm.dispatchEvent(new CustomEvent("person-picker-close"));
      dialogForm.querySelectorAll("input, button").forEach(function (control) { control.disabled = true; });
      personDialog.setAttribute("aria-busy", "true");
      dialogStatus.textContent = "Person wird gespeichert …";
      var saved = false;
      try {
        var response = await fetch(action, {
          method: "POST", credentials: "same-origin", redirect: "error",
          headers: { Accept: "application/json" }, body: body
        });
        if (!response.ok) throw new Error("Person konnte nicht gespeichert werden (HTTP " + response.status + ").");
        var result = await response.json();
        if (result.ok !== true) throw new Error("Person konnte nicht gespeichert werden.");
        saved = true;
        await options.onSave(dialogIDs);
        status.textContent = merging ? "Personen zusammengeführt." : "Person benannt.";
        personDialog.close();
      } catch (error) {
        dialogStatus.textContent = saved ? "Gespeichert, aber die Ansicht konnte nicht aktualisiert werden. Bitte die Seite neu laden." : error.message;
        // Do not offer the same mutation again after a successful save.
        if (saved && opener && opener.isConnected) opener.disabled = true;
      } finally {
        busy = false;
        options.onBusy(busy);
        personDialog.removeAttribute("aria-busy");
        dialogForm.querySelectorAll("input, button").forEach(function (control) { control.disabled = saved; });
        personDialog.querySelector("[data-person-dialog-cancel]").disabled = false;
      }
    });
    return { open: openPersonDialog };
  }

  window.BearStackPersonDialog = { bind: bind, createEditButton: createEditButton };

  document.querySelectorAll("[data-person-picker]").forEach(function (form) {
    var input = form.querySelector("[data-person-search]");
    var target = form.querySelector("[data-person-target]");
    var list = form.querySelector("[data-person-options]");
    var popup = form.querySelector("[data-person-popup]");
    var feedback = form.querySelector("[data-person-feedback]");
    var allowCreate = form.hasAttribute("data-person-create");
    var renameAction = allowCreate ? form.action : "";
    var items = [], active = -1, revision = 0;
    var timer, controller, pendingDirection;

    function cancel() {
      clearTimeout(timer);
      if (controller) controller.abort();
      revision++;
    }
    function close() {
      cancel();
      popup.hidden = true;
      input.setAttribute("aria-expanded", "false");
      input.removeAttribute("aria-activedescendant");
      active = -1;
    }
    function validate() {
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
      input.value = person.create ? person.name : (person.name || "Unbenannt") + " (#" + person.id + ")";
      validate();
      close();
      if (form.hasAttribute("data-person-modal")) form.requestSubmit();
    }
    async function load(direction) {
      cancel();
      pendingDirection = direction;
      var request = revision;
      controller = new AbortController();
      list.replaceChildren();
      items = [];
      activate(-1);
      popup.hidden = false;
      input.setAttribute("aria-expanded", "true");
      feedback.textContent = "Personen werden geladen …";
      // A selected label contains the ID; opening it again lists alternatives.
      var query = target.value && target.value !== "0" ? "" : input.value.trim();
      try {
        var response = await fetch("/photos/people?format=suggestions&q=" + encodeURIComponent(query), {
          signal: controller.signal, credentials: "same-origin", redirect: "error",
          headers: { Accept: "application/json" }
        });
        if (!response.ok) throw new Error("Personen konnten nicht geladen werden. Bitte erneut suchen.");
        var data = await response.json();
        if (!Array.isArray(data.people)) throw new Error("Ungültige Antwort der Personensuche.");
        if (request !== revision || document.activeElement !== input) return;
        items = data.people.filter(function (person) { return typeof person.name === "string" && person.name.trim() && String(person.id) !== form.dataset.personExclude; });
        if (allowCreate && query) items.push({ id: 0, name: query, create: true });
        var options = document.createDocumentFragment();
        items.forEach(function (person, index) {
          var option = document.createElement("div");
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
        feedback.textContent = data.has_next ? "Weitere Personen vorhanden. Bitte die Suche eingrenzen." :
          (items.length ? items.length + (items.length === 1 ? " Vorschlag verfügbar." : " Vorschläge verfügbar.") : "Keine passende Person gefunden.");
        if (items.length && pendingDirection) activate(pendingDirection === "last" ? items.length - 1 : 0);
      } catch (error) {
        if (request === revision && error.name !== "AbortError") feedback.textContent = error.message;
      }
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
      if (event.target.closest("[data-person-option]")) event.preventDefault();
    });
    list.addEventListener("click", function (event) {
      var option = event.target.closest("[data-person-option]");
      if (option) choose(Number(option.dataset.personOption));
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
