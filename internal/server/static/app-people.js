(function () {
  "use strict";
  var overview = document.querySelector("[data-people-overview]");
  var status = document.querySelector("[data-people-status]");
  var busy = false;

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
    overview.addEventListener("click", async function (event) {
      var button = event.target.closest("[data-ignore-face]");
      if (!button || !overview.contains(button)) return;
      event.preventDefault();
      if (busy || button.disabled) return;
      busy = true;
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
        overview.removeAttribute("aria-busy");
        overview.querySelectorAll("[data-ignore-face]").forEach(function (item) { item.disabled = item.dataset.ignoreSaved === "true"; });
      }
    });
  }

  document.querySelectorAll("[data-person-picker]").forEach(function (form) {
    var input = form.querySelector("[data-person-search]");
    var select = form.querySelector("[data-person-options]");
    var timer, controller;
    function load() {
      if (controller) controller.abort();
      controller = new AbortController();
      fetch("/photos/people?format=json&q=" + encodeURIComponent(input.value), {signal: controller.signal, credentials: "same-origin"})
        .then(function (response) { if (!response.ok) throw new Error("Personen konnten nicht geladen werden"); return response.json(); })
        .then(function (data) {
          while (select.options.length > 1) select.remove(1);
          (data.people || []).forEach(function (person) {
            var option = document.createElement("option"); option.value = person.id;
            option.textContent = (person.name || "Unbenannt") + " (#" + person.id + ", " + person.count + (person.count === 1 ? " Foto)" : " Fotos)");
            select.appendChild(option);
          });
          input.setCustomValidity("");
        }).catch(function (error) { if (error.name !== "AbortError") input.setCustomValidity(error.message); });
    }
    input.addEventListener("input", function () { clearTimeout(timer); timer = setTimeout(load, 250); });
    load();
  });
}());
