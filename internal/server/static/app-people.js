(function () {
  "use strict";
  var overview = document.querySelector("[data-people-overview]");
  var status = document.querySelector("[data-people-status]");
  var busy = false;
  var selected = new Set();
  var merge = document.querySelector("[data-people-merge]");
  var mergeButton = document.querySelector("[data-people-merge-button]");

  function updateSelection() {
    if (!merge) return;
    var cards = new Map();
    overview.querySelectorAll("[data-person-id]").forEach(function (card) { cards.set(card.dataset.personId, card); });
    selected.forEach(function (id) { if (!cards.has(id)) selected.delete(id); });
    overview.querySelectorAll("[data-person-select]").forEach(function (input) {
      input.checked = selected.has(input.value);
      input.disabled = busy;
    });
    merge.hidden = selected.size < 2;
    mergeButton.disabled = busy;
    if (selected.size) {
      var target = cards.get(selected.values().next().value);
      document.querySelector("[data-people-merge-target]").textContent =
        selected.size + " ausgewählt · Ziel: " + target.querySelector("strong").textContent;
    }
  }


  function personCard(person, existing) {
    var name = person.name || "Unbenannt";
    var thumbnail = "/photos/faces/" + encodeURIComponent(person.face_id) + "/thumbnail";
    var countText = person.count + (person.count === 1 ? " Foto" : " Fotos");
    // Keep unchanged cards and their loaded thumbnails across Ajax updates.
    if (existing && existing.querySelector("img").getAttribute("src") === thumbnail &&
        existing.querySelector("strong").textContent === name &&
        existing.querySelector(".person-card > span").textContent === countText) return existing;
    var card = document.createElement("div");
    card.className = "person-overview-card";
    card.dataset.personId = person.id;
    var link = document.createElement("a");
    link.className = "person-card";
    link.href = "/photos/people/" + encodeURIComponent(person.id);
    var img = document.createElement("img");
    img.loading = "lazy"; img.width = 160; img.height = 160; img.alt = "";
    img.src = thumbnail;
    var title = document.createElement("strong"); title.textContent = name;
    var count = document.createElement("span");
    count.textContent = countText;
    link.append(img, title, count); card.append(link);
    if (overview.dataset.canIgnore === "true") {
      var label = document.createElement("label");
      label.className = "person-select";
      var checkbox = document.createElement("input");
      checkbox.type = "checkbox"; checkbox.dataset.personSelect = ""; checkbox.value = person.id;
      checkbox.setAttribute("aria-label", "Person auswählen: " + name);
      label.append(checkbox, document.createTextNode(" Auswählen"));
      card.append(label);
      var button = document.createElement("button");
      button.type = "button"; button.className = "person-ignore-button";
      button.dataset.ignoreFace = person.face_id;
      button.textContent = "×";
      button.title = "Angezeigtes Gesicht ignorieren";
      button.setAttribute("aria-label", "Angezeigtes Gesicht ignorieren: " + name);
      card.append(button);
    }
    return card;
  }

  async function refreshPeople() {
    var url = new URL(window.location.href);
    url.searchParams.set("format", "json");
    async function load() {
      var response = await fetch(url, { credentials: "same-origin", redirect: "error", headers: { Accept: "application/json" } });
      if (!response.ok) throw new Error("Personen konnten nicht geladen werden.");
      var data = await response.json();
      if (!Array.isArray(data.people)) throw new Error("Ungültige Antwort der Personenübersicht.");
      return data;
    }
    var data = await load();
    if (!data.people.length && data.has_prev) {
      url.searchParams.set("page", String(Math.max(1, data.page - 1)));
      data = await load();
      var address = new URL(window.location.href);
      address.searchParams.set("page", String(data.page));
      window.history.replaceState(window.history.state, "", address);
    }
    var cards = document.createDocumentFragment();
    var existing = new Map();
    overview.querySelectorAll("[data-person-id]").forEach(function (card) { existing.set(card.dataset.personId, card); });
    data.people.forEach(function (person) { cards.append(personCard(person, existing.get(String(person.id)))); });
    if (!data.people.length) {
      var empty = document.createElement("p"); empty.textContent = "Keine weiteren Personen gefunden."; cards.append(empty);
    }
    overview.replaceChildren(cards);
    updateSelection();
    var pagination = document.querySelector(".people-pagination");
    var parts = document.createDocumentFragment();
    function pageLink(page, label, relation) {
      var link = document.createElement("a");
      var target = new URL(window.location.href);
      target.searchParams.delete("format"); target.searchParams.set("page", String(page));
      link.href = target.pathname + target.search;
      link.className = "secondary-button"; link.rel = relation; link.textContent = label;
      return link;
    }
    if (data.has_prev) parts.append(pageLink(data.page - 1, "Zurück", "prev"));
    var current = document.createElement("span"); current.className = "people-pagination-current";
    current.setAttribute("aria-current", "page"); current.textContent = "Seite " + data.page; parts.append(current);
    if (data.has_next) parts.append(pageLink(data.page + 1, "Weiter", "next"));
    pagination.replaceChildren(parts);
  }

  if (overview && overview.dataset.canIgnore === "true") {
    overview.addEventListener("change", function (event) {
      var input = event.target.closest("[data-person-select]");
      if (!input || busy) return;
      if (input.checked) selected.add(input.value); else selected.delete(input.value);
      updateSelection();
    });
    mergeButton.addEventListener("click", async function () {
      if (busy || selected.size < 2) return;
      var ids = Array.from(selected);
      var body = new URLSearchParams({ target: ids[0] });
      ids.slice(2).forEach(function (id) { body.append("person_id", id); });
      busy = true;
      updateSelection();
      overview.setAttribute("aria-busy", "true");
      overview.querySelectorAll("[data-ignore-face]").forEach(function (item) { item.disabled = true; });
      status.textContent = "Personen werden zusammengeführt …";
      var saved = false;
      try {
        var response = await fetch("/photos/people/" + encodeURIComponent(ids[1]) + "/merge", {
          method: "POST", credentials: "same-origin", redirect: "error",
          headers: { Accept: "application/json" }, body: body
        });
        if (!response.ok) throw new Error("Personen konnten nicht zusammengeführt werden (HTTP " + response.status + ").");
        var result = await response.json();
        if (result.ok !== true) throw new Error("Personen konnten nicht zusammengeführt werden.");
        saved = true;
        selected.clear();
        await refreshPeople();
        status.textContent = "Personen zusammengeführt.";
        var target = overview.querySelector('[data-person-id="' + ids[0] + '"] a');
        if (target) target.focus({ preventScroll: true });
      } catch (error) {
        status.textContent = saved ? "Personen zusammengeführt, aber die Ansicht konnte nicht aktualisiert werden. Bitte die Seite neu laden." : error.message;
      } finally {
        busy = false;
        updateSelection();
        overview.removeAttribute("aria-busy");
        overview.querySelectorAll("[data-ignore-face]").forEach(function (item) { item.disabled = item.dataset.ignoreSaved === "true"; });
        updateSelection();
      }
    });
    updateSelection();
    overview.addEventListener("click", async function (event) {
      var button = event.target.closest("[data-ignore-face]");
      if (!button || !overview.contains(button)) return;
      event.preventDefault();
      if (busy || button.disabled) return;
      busy = true;
      updateSelection();
      overview.setAttribute("aria-busy", "true");
      overview.querySelectorAll("[data-ignore-face]").forEach(function (item) { item.disabled = true; });
      status.textContent = "Gesicht wird ignoriert …";
      var saved = false;
      var position = Array.from(overview.children).indexOf(button.closest(".person-overview-card"));
      try {
        var response = await fetch("/photos/faces/edit", {
          method: "POST", credentials: "same-origin", redirect: "error",
          headers: { Accept: "application/json" },
          body: new URLSearchParams({ face_id: button.dataset.ignoreFace, action: "ignore" })
        });
        if (!response.ok) throw new Error("Gesicht konnte nicht ignoriert werden (HTTP " + response.status + ").");
        var result = await response.json();
        if (result.ok !== true) throw new Error("Gesicht konnte nicht ignoriert werden.");
        saved = true;
        await refreshPeople();
        status.textContent = "Gesicht ignoriert.";
        var buttons = overview.querySelectorAll("[data-ignore-face]");
        if (buttons.length) buttons[Math.min(position, buttons.length - 1)].focus({ preventScroll: true });
      } catch (error) {
        status.textContent = saved ? "Gesicht ignoriert, aber die Ansicht konnte nicht aktualisiert werden. Bitte die Seite neu laden." : error.message;
        if (saved) button.dataset.ignoreSaved = "true";
      } finally {
        busy = false;
        updateSelection();
        overview.removeAttribute("aria-busy");
        overview.querySelectorAll("[data-ignore-face]").forEach(function (item) { item.disabled = item.dataset.ignoreSaved === "true"; });
      }
    });
  }

  document.querySelectorAll("[data-person-picker]").forEach(function (form) {
    var input = form.querySelector("[data-person-search]");
    var target = form.querySelector("[data-person-target]");
    var list = form.querySelector("[data-person-options]");
    var popup = form.querySelector("[data-person-popup]");
    var feedback = form.querySelector("[data-person-feedback]");
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
      if (!person) return;
      target.value = String(person.id);
      input.value = (person.name || "Unbenannt") + " (#" + person.id + ")";
      validate();
      close();
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
        var response = await fetch("/photos/people?format=json&q=" + encodeURIComponent(query), {
          signal: controller.signal, credentials: "same-origin", redirect: "error",
          headers: { Accept: "application/json" }
        });
        if (!response.ok) throw new Error("Personen konnten nicht geladen werden. Bitte erneut suchen.");
        var data = await response.json();
        if (!Array.isArray(data.people)) throw new Error("Ungültige Antwort der Personensuche.");
        if (request !== revision || document.activeElement !== input) return;
        items = data.people.filter(function (person) { return String(person.id) !== form.dataset.personExclude; });
        var options = document.createDocumentFragment();
        items.forEach(function (person, index) {
          var option = document.createElement("div");
          option.id = list.id + "-" + index;
          option.dataset.personOption = String(index);
          option.setAttribute("role", "option");
          option.setAttribute("aria-selected", "false");
          option.textContent = (person.name || "Unbenannt") + " (#" + person.id + ", " +
            person.count + (person.count === 1 ? " Foto)" : " Fotos)");
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
      } else if (event.key === "Escape") {
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
    input.addEventListener("blur", close);
    validate();
  });
}());
