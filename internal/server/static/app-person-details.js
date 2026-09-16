(function () {
  "use strict";
  var root = document.querySelector("[data-person-detail]");
  var dialog = document.querySelector("[data-person-details-dialog]");
  if (!root || !dialog) return;
  var form = dialog.querySelector("form"), fields = dialog.querySelector("[data-person-details-fields]");
  var status = dialog.querySelector("[data-person-details-status]"), reload = dialog.querySelector("[data-person-details-reload]");
  var siblings = dialog.querySelector("[data-person-details-siblings]"), marriages = dialog.querySelector("[data-person-details-marriages]");
  var parents = dialog.querySelector("[data-person-details-parents]"), lifeDates = dialog.querySelector("[data-person-details-dates]");
  var save = dialog.querySelector("[data-person-details-save]");
  var editable = dialog.dataset.canEdit === "true", revision = "", personID, request, saving = false, serial = 0;
  var more = root.querySelector(".person-detail-more");
  function closePickers(container) {
    container.querySelectorAll("[data-person-picker]").forEach(function (picker) { picker.dispatchEvent(new CustomEvent("person-picker-close")); });
  }
  function updateAddButtons() {
    dialog.querySelectorAll("[data-person-details-add]").forEach(function (button) {
      button.disabled = saving || (button.dataset.personDetailsAdd === "sibling" ? siblings : marriages).children.length >= 100;
    });
  }
  function iconButton(title, symbol, action) {
    var button = document.createElement("button"); button.type = "button"; button.className = "secondary-button person-details-icon";
    button.textContent = symbol; button.title = title; button.setAttribute("aria-label", title);
    button.addEventListener("click", action); return button;
  }
  function personField(row, title, person, optional) {
    if (!editable) {
      var text = document.createElement(person && person.id ? "a" : "span");
      if (person && person.id) text.href = "/photos/people/" + encodeURIComponent(person.id);
      text.textContent = (optional ? title + ": " : "") + (person && person.id ? person.name : "Nicht angegeben");
      row.append(text); return;
    }
    var picker = dialog.querySelector("[data-person-details-picker]").content.firstElementChild.cloneNode(true);
    var id = "person-relative-" + (++serial), input = picker.querySelector("[data-person-search]");
    picker.dataset.personExclude = personID;
    input.id = id; input.setAttribute("aria-controls", id + "-options"); input.setAttribute("aria-label", title);
    input.placeholder = title + " suchen"; input.required = !optional;
    picker.querySelector("label").remove();
    picker.querySelector("[data-person-options]").id = id + "-options";
    picker.querySelector("[data-person-options]").setAttribute("aria-label", title + " auswählen");
    input.value = person && person.id ? person.name + " (#" + person.id + ")" : "";
    picker.querySelector("[data-person-target]").value = person && person.id ? person.id : "0";
    row.append(picker); window.initializePersonPickers(row);
    if (optional) row.append(iconButton(title + " entfernen", "×", function () {
      input.value = ""; input.dispatchEvent(new Event("input", { bubbles: true })); input.focus(); closePickers(row);
    }));
  }
  function dateField(row, title, key, value, optional, named) {
    var slot = document.createElement("div"); slot.className = "person-details-date";
    var label = document.createElement("label"); label.textContent = title;
    var input = document.createElement("input"); input.type = "date"; input.min = "0001-01-01"; input.max = "9999-12-31";
    input.dataset.date = key; if (named) input.name = key;
    input.value = value || ""; input.readOnly = !editable; label.append(input); slot.append(label);
    label.hidden = optional && !value;
    if (optional && !value && editable) {
      var reveal = iconButton(title + " ergänzen", "+", function () {
        label.hidden = false; reveal.hidden = true; input.focus();
      });
      reveal.setAttribute("aria-expanded", "false");
      input.id = "person-date-" + (++serial); reveal.setAttribute("aria-controls", input.id);
      reveal.addEventListener("click", function () { reveal.setAttribute("aria-expanded", "true"); });
      slot.append(reveal);
    }
    if (optional && !value && !editable) slot.hidden = true;
    row.append(slot);
  }
  function addRow(kind, value) {
    var container = kind === "sibling" ? siblings : marriages;
    if (container.children.length >= 100) return;
    var row = document.createElement("div"), line = document.createElement("div");
    row.dataset.relation = kind; line.className = "person-details-relation-line";
    personField(line, kind === "sibling" ? "Geschwister" : "Ehepartner", kind === "sibling" ? value : value && value.spouse);
    if (editable) line.append(iconButton(kind === "sibling" ? "Geschwisterzuordnung entfernen" : "Ehe entfernen", "×", function () {
      closePickers(row); row.remove(); updateAddButtons();
    }));
    row.append(line);
    if (kind === "marriage") {
      row.dataset.marriageId = value ? value.id : "0";
      var dates = document.createElement("div"); dates.className = "person-details-dates";
      dateField(dates, "Hochzeitsdatum", "wedding_date", value && value.wedding_date);
      dateField(dates, "Scheidungsdatum", "divorce_date", value && value.divorce_date, true); row.append(dates);
    }
    container.append(row); updateAddButtons(); return row;
  }
  async function load() {
    if (request) request.abort(); request = new AbortController(); var current = request;
    var timer = setTimeout(function () { current.abort(); }, 20000);
    fields.hidden = true; reload.hidden = true; if (save) save.disabled = true;
    status.textContent = "Stammdaten werden geladen …";
    closePickers(dialog);
    try {
      var response = await fetch("/photos/people/" + encodeURIComponent(personID) + "/details", { credentials: "same-origin", redirect: "error", headers: { Accept: "application/json" }, signal: current.signal });
      var data = await response.json();
      if (!response.ok) throw new Error(data.error || "Stammdaten konnten nicht geladen werden.");
      if (current !== request || !dialog.open) return;
      revision = data.revision;
      lifeDates.replaceChildren(); parents.replaceChildren();
      dateField(lifeDates, "Geburtsdatum", "birth_date", data.birth_date, false, true);
      dateField(lifeDates, "Sterbedatum", "death_date", data.death_date, true, true);
      ["mother", "father"].forEach(function (role) {
        var row = document.createElement("div"); row.className = "person-details-relation-line"; row.dataset.parentRole = role;
        personField(row, role === "mother" ? "Mutter" : "Vater", data.parents && data.parents[role], true); parents.append(row);
      });
      siblings.replaceChildren(); marriages.replaceChildren();
      data.siblings.forEach(function (person) { addRow("sibling", person); });
      data.marriages.forEach(function (marriage) { addRow("marriage", marriage); });
      fields.hidden = false; status.textContent = ""; if (save) save.disabled = false; updateAddButtons();
    } catch (error) {
      if (current !== request || !dialog.open) return;
      status.textContent = error.name === "AbortError" ? "Laden dauert zu lange. Bitte erneut versuchen." : error.message;
      reload.hidden = false;
    } finally { clearTimeout(timer); }
  }
  root.addEventListener("click", function (event) {
    var button = event.target.closest("[data-person-details-open]"); if (!button || button.disabled) return;
    personID = root.dataset.personId; more.open = false;
    dialog.querySelector("h2").textContent = "Stammdaten: " + root.dataset.personName;
    dialog.showModal(); load();
  });
  dialog.querySelectorAll("[data-person-details-close]").forEach(function (button) { button.addEventListener("click", function () { if (!saving) dialog.close(); }); });
  dialog.addEventListener("cancel", function (event) { if (saving) event.preventDefault(); });
  dialog.addEventListener("close", function () { if (request) request.abort(); request = null; closePickers(dialog); more.querySelector("summary").focus({ preventScroll: true }); });
  reload.addEventListener("click", load);
  dialog.querySelectorAll("[data-person-details-add]").forEach(function (button) {
    button.addEventListener("click", function () { var row = addRow(button.dataset.personDetailsAdd); if (row) row.querySelector("[data-person-search]").focus(); });
  });
  form.addEventListener("submit", async function (event) {
    event.preventDefault(); if (!editable || saving || !revision) return;
    var input = { revision: revision, birth_date: form.elements.birth_date.value, death_date: form.elements.death_date.value,
      mother_id: Number(parents.querySelector('[data-parent-role="mother"] [data-person-target]').value),
      father_id: Number(parents.querySelector('[data-parent-role="father"] [data-person-target]').value),
      sibling_ids: Array.from(siblings.children).map(function (row) { return Number(row.querySelector("[data-person-target]").value); }),
      marriages: Array.from(marriages.children).map(function (row) { return { id: Number(row.dataset.marriageId), spouse_id: Number(row.querySelector("[data-person-target]").value), wedding_date: row.querySelector('[data-date="wedding_date"]').value, divorce_date: row.querySelector('[data-date="divorce_date"]').value }; }) };
    saving = true; closePickers(dialog); reload.hidden = true; status.textContent = "Stammdaten werden gespeichert …";
    var controls = Array.from(dialog.querySelectorAll("button, input")); controls.forEach(function (control) { control.disabled = true; });
    var controller = new AbortController(), timer = setTimeout(function () { controller.abort(); }, 20000);
    try {
      var response = await fetch("/photos/people/" + encodeURIComponent(personID) + "/details", { method: "PUT", credentials: "same-origin", redirect: "error", headers: { "Content-Type": "application/json", Accept: "application/json" }, body: JSON.stringify(input), signal: controller.signal });
      var result = await response.json();
      if (!response.ok || result.ok !== true) throw new Error(result.error || "Stammdaten konnten nicht gespeichert werden.");
      root.querySelector("[data-detail-status]").textContent = "Stammdaten gespeichert."; dialog.close();
    } catch (error) {
      status.textContent = error.name === "AbortError" || error instanceof TypeError ? "Speicherstatus unklar. Bitte die Stammdaten neu laden und prüfen." : error.message;
      reload.hidden = false;
    } finally { clearTimeout(timer); saving = false; controls.forEach(function (control) { control.disabled = false; }); updateAddButtons(); }
  });
}());
