(function () {
  "use strict";
  var root = document.querySelector("[data-person-detail]");
  var dialog = document.querySelector("[data-person-details-dialog]");
  if (!root || !dialog) return;
  var form = dialog.querySelector("form"), fields = dialog.querySelector("[data-person-details-fields]");
  var status = dialog.querySelector("[data-person-details-status]"), reload = dialog.querySelector("[data-person-details-reload]");
  var siblings = dialog.querySelector("[data-person-details-siblings]"), marriages = dialog.querySelector("[data-person-details-marriages]");
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
  function personField(row, title, person) {
    if (person && person.id) {
      var link = document.createElement("a"); link.href = "/photos/people/" + encodeURIComponent(person.id); link.textContent = person.name;
      row.append(link);
    }
    if (!editable) return;
    var picker = dialog.querySelector("[data-person-details-picker]").content.firstElementChild.cloneNode(true);
    var id = "person-relative-" + (++serial), input = picker.querySelector("[data-person-search]");
    picker.dataset.personExclude = personID;
    input.id = id; input.setAttribute("aria-controls", id + "-options");
    picker.querySelector("label").textContent = title; picker.querySelector("label").htmlFor = id;
    picker.querySelector("[data-person-options]").id = id + "-options";
    picker.querySelector("[data-person-options]").setAttribute("aria-label", title + " auswählen");
    input.value = person && person.id ? person.name + " (#" + person.id + ")" : "";
    picker.querySelector("[data-person-target]").value = person && person.id ? person.id : "0";
    row.append(picker); window.initializePersonPickers(row);
  }
  function dateField(row, title, key, value) {
    var label = document.createElement("label"); label.textContent = title;
    var input = document.createElement("input"); input.type = "date"; input.min = "0001-01-01"; input.max = "9999-12-31";
    input.dataset.date = key; input.value = value || ""; input.readOnly = !editable; label.append(input); row.append(label);
  }
  function addRow(kind, value) {
    var container = kind === "sibling" ? siblings : marriages;
    if (container.children.length >= 100) return;
    var row = document.createElement("fieldset"), legend = document.createElement("legend");
    row.dataset.relation = kind; legend.textContent = kind === "sibling" ? "Geschwister" : "Ehe"; row.append(legend);
    if (kind === "sibling") personField(row, "Geschwisterperson", value);
    else {
      row.dataset.marriageId = value ? value.id : "0";
      personField(row, "Ehepartner", value && value.spouse);
      var dates = document.createElement("div"); dates.className = "person-details-dates";
      dateField(dates, "Hochzeitsdatum", "wedding_date", value && value.wedding_date);
      dateField(dates, "Scheidungsdatum", "divorce_date", value && value.divorce_date); row.append(dates);
    }
    if (editable) {
      var remove = document.createElement("button"); remove.type = "button"; remove.className = "secondary-button";
      remove.textContent = kind === "sibling" ? "Geschwisterzuordnung entfernen" : "Ehe entfernen";
      remove.addEventListener("click", function () { closePickers(row); row.remove(); updateAddButtons(); }); row.append(remove);
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
      revision = data.revision; form.elements.birth_date.value = data.birth_date; form.elements.death_date.value = data.death_date;
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
