(function () {
  "use strict";
  var pageNavigation = document.querySelector("[data-people-page]");
  var pageStorageKey = pageNavigation ? "bearstack.people.lastPage:" + pageNavigation.dataset.peopleUser : "";
  function savePeoplePage(page) {
    if (!pageStorageKey) return;
    var address = new URL(window.location.href);
    if (address.pathname !== "/photos/people") return;
    var saved = { page: page, q: address.searchParams.get("q") || "", known: address.searchParams.get("known") === "1", ignored: address.searchParams.get("ignored") === "1" };
    try { window.localStorage.setItem(pageStorageKey, JSON.stringify(saved)); } catch (_) {}
  }
  if (pageNavigation) {
    var address = new URL(window.location.href);
    var saved;
    try { saved = JSON.parse(window.localStorage.getItem(pageStorageKey)); } catch (_) {}
    if (saved && Number.isInteger(saved.page) && saved.page >= 1 && saved.page <= 1000000 && typeof saved.q === "string") {
      var destination = new URL("/photos/people", address.origin);
      destination.searchParams.set("page", String(saved.page));
      destination.searchParams.set("q", saved.q);
      if (saved.known === true) destination.searchParams.set("known", "1");
      if (saved.ignored === true) destination.searchParams.set("ignored", "1");
      if (address.pathname === "/photos/people" && !["page", "q", "known", "ignored"].some(function (key) { return address.searchParams.has(key); })) {
        if (address.searchParams.has("notice")) destination.searchParams.set("notice", address.searchParams.get("notice"));
        window.location.replace(destination.pathname + destination.search);
        return;
      }
      if (address.pathname !== "/photos/people") {
        document.querySelectorAll('a[href="/photos/people"]').forEach(function (link) { link.href = destination.pathname + destination.search; });
      }
    }
    savePeoplePage(Number(pageNavigation.dataset.peoplePage));
  }
  var favoriteStatus = document.querySelector("[data-face-favorite-status]");
  document.querySelectorAll("button[data-face-favorite]").forEach(function (button) {
    button.addEventListener("click", async function (event) {
      event.preventDefault();
      if (button.disabled) return;
      var favorite = button.getAttribute("aria-pressed") !== "true";
      button.disabled = true;
      favoriteStatus.textContent = "";
      try {
        var response = await fetch("/api/photos/labeling/v1/faces/" + encodeURIComponent(button.dataset.faceFavorite) + "/favorite", {
          method: "PUT", credentials: "same-origin", headers: { "Content-Type": "application/json", "Accept": "application/json" },
          body: JSON.stringify({ person_id: Number(button.dataset.personId), favorite: favorite })
        });
        var result = await response.json();
        if (!response.ok) throw new Error(result.error || "Favorisierung konnte nicht gespeichert werden.");
        if (typeof result.favorite !== "boolean") throw new Error("Ungültige Antwort beim Speichern der Favorisierung.");
        button.setAttribute("aria-pressed", String(result.favorite));
        button.value = result.favorite ? "0" : "1";
        button.title = result.favorite ? "Favorisierung aufheben" : "Als Vergleichsgesicht favorisieren";
        button.setAttribute("aria-label", button.title);
        favoriteStatus.textContent = result.favorite ? "Gesicht als Vergleichsbild favorisiert." : "Favorisierung aufgehoben.";
      } catch (error) {
        favoriteStatus.textContent = error.message || "Favorisierung konnte nicht gespeichert werden.";
      } finally {
        button.disabled = false;
      }
    });
  });

  var overview = document.querySelector("[data-people-overview]");
  var status = document.querySelector("[data-people-status]");
  var busy = false;
  var selected = new Set();
  var merge = document.querySelector("[data-people-merge]");
  var mergeButton = document.querySelector("[data-people-merge-button]");
  var editSelectedButton = document.querySelector("[data-people-edit-button]");

  function mergeSelection() {
    var ids = Array.from(selected);
    var named = ids.findIndex(function (id) {
      var card = overview.querySelector('[data-person-id="' + id + '"]');
      return card && Boolean(card.dataset.personName);
    });
    if (named > 0) ids.unshift(ids.splice(named, 1)[0]);
    return ids;
  }

  function updateSelection() {
    if (!merge) return;
    var cards = new Map();
    overview.querySelectorAll("[data-person-id]").forEach(function (card) { cards.set(card.dataset.personId, card); });
    selected.forEach(function (id) { if (!cards.has(id)) selected.delete(id); });
    overview.querySelectorAll("[data-person-select]").forEach(function (input) {
      input.checked = selected.has(input.value);
      input.disabled = busy;
    });
    merge.hidden = selected.size < 1;
    mergeButton.hidden = selected.size < 2;
    editSelectedButton.disabled = busy;
    mergeButton.disabled = busy;
    if (selected.size) {
      var target = cards.get(mergeSelection()[0]);
      document.querySelector("[data-people-merge-target]").textContent =
        selected.size + " ausgewählt · Ziel: " + target.querySelector("strong").textContent;
    }
  }


  function personCard(person, existing) {
    var name = person.name || "Unbenannt";
    var thumbnail = "/photos/faces/" + encodeURIComponent(person.face_id) + "/thumbnail";
    var countText = person.count + (person.count === 1 ? " Foto" : " Fotos");
    // Update text in place so a changed count/name never reloads the image.
    if (existing) {
      existing.dataset.personName = person.name || "";
      var existingImage = existing.querySelector("img");
      if (existingImage.getAttribute("src") !== thumbnail) existingImage.src = thumbnail;
      existing.querySelector("strong").textContent = name;
      existing.querySelector(".person-card > span").textContent = countText;
      var checkbox = existing.querySelector("[data-person-select]");
      if (checkbox) checkbox.setAttribute("aria-label", "Person auswählen: " + name);
      var ignore = existing.querySelector("[data-ignore-face]");
      if (ignore) {
        ignore.dataset.ignoreFace = person.face_id;
        ignore.setAttribute("aria-label", "Angezeigtes Gesicht ignorieren: " + name);
      }
      var edit = existing.querySelector("[data-person-edit]");
      if (edit) edit.setAttribute("aria-label", "Benennen oder zuordnen: " + name);
      return existing;
    }
    var card = document.createElement("div");
    card.className = "person-overview-card";
    card.dataset.personId = person.id;
    card.dataset.personName = person.name || "";
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
      label.append(checkbox);
      card.append(label);
      var button = document.createElement("button");
      button.type = "button"; button.className = "person-ignore-button";
      button.dataset.ignoreFace = person.face_id;
      button.textContent = "×";
      button.title = "Angezeigtes Gesicht ignorieren";
      button.setAttribute("aria-label", "Angezeigtes Gesicht ignorieren: " + name);
      card.append(button);
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
      edit.setAttribute("aria-label", "Benennen oder zuordnen: " + name);
      card.append(edit);
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
    var address = new URL(window.location.href);
    address.searchParams.set("page", String(data.page));
    window.history.replaceState(window.history.state, "", address);
    savePeoplePage(data.page);
    var existing = new Map();
    overview.querySelectorAll("[data-person-id]").forEach(function (card) { existing.set(card.dataset.personId, card); });
    var keep = new Set();
    data.people.forEach(function (person, index) {
      var card = personCard(person, existing.get(String(person.id)));
      keep.add(card);
      if (overview.children[index] !== card) overview.insertBefore(card, overview.children[index] || null);
    });
    Array.from(overview.children).forEach(function (card) { if (!keep.has(card)) card.remove(); });
    if (!data.people.length) {
      var empty = document.createElement("p"); empty.textContent = "Keine weiteren Personen gefunden."; overview.append(empty);
    }
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
    function boundary(page, label, relation, enabled) {
      if (enabled) return pageLink(page, label, relation);
      var disabled = document.createElement("span");
      disabled.className = "secondary-button"; disabled.setAttribute("aria-disabled", "true"); disabled.textContent = label;
      return disabled;
    }
    parts.append(boundary(1, "Erste Seite", "first", data.has_prev));
    if (data.has_prev) parts.append(pageLink(data.page - 1, "Zurück", "prev"));
    var current = document.createElement("span"); current.className = "people-pagination-current";
    current.setAttribute("aria-current", "page"); current.textContent = "Seite " + data.page + " von " + data.total_pages; parts.append(current);
    if (data.has_next) parts.append(pageLink(data.page + 1, "Weiter", "next"));
    parts.append(boundary(data.total_pages, "Letzte Seite", "last", data.has_next));
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
      var ids = mergeSelection();
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

  var groupPhotos = document.querySelector("[data-group-photos]");
  var personSurface = overview || groupPhotos;
  var personDialog = document.querySelector("[data-person-dialog]");
  if (personSurface && personDialog) {
    var dialogForm = personDialog.querySelector("form");
    var dialogStatus = personDialog.querySelector("[data-person-dialog-status]");
    var opener, dialogIDs = [];
    function openPersonDialog(ids, button) {
      if (!ids.length || busy || button.disabled) return;
      dialogIDs = ids;
      var card = personSurface.querySelector('[data-person-id="' + ids[0] + '"]');
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
    }
    if (editSelectedButton) editSelectedButton.addEventListener("click", function () { openPersonDialog(Array.from(selected), editSelectedButton); });
    personSurface.addEventListener("click", function (event) {
      var button = event.target.closest("[data-person-edit]");
      if (!button || busy || button.disabled) return;
      var card = button.closest("[data-person-id]");
      openPersonDialog([card.dataset.personId], button);
    });
    personDialog.querySelector("[data-person-dialog-cancel]").addEventListener("click", function () { if (!busy) personDialog.close(); });
    personDialog.addEventListener("cancel", function (event) { if (busy) event.preventDefault(); });
    personDialog.addEventListener("close", function () {
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
      updateSelection();
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
        dialogIDs.forEach(function (id) { selected.delete(id); });
        if (groupPhotos) {
          // The photo controller supplies its refresh promise synchronously, so
          // the shared modal can retain its normal save/error/focus lifecycle.
          var change = { refresh: Promise.resolve() };
          groupPhotos.dispatchEvent(new CustomEvent("person-edit-saved", { detail: change }));
          await change.refresh;
        } else {
          await refreshPeople();
        }
        status.textContent = merging ? "Personen zusammengeführt." : "Person benannt.";
        personDialog.close();
      } catch (error) {
        dialogStatus.textContent = saved ? "Gespeichert, aber die Ansicht konnte nicht aktualisiert werden. Bitte die Seite neu laden." : error.message;
        // Do not offer the same mutation again after a successful save.
        if (saved && opener && opener.isConnected) opener.disabled = true;
      } finally {
        busy = false;
        updateSelection();
        personDialog.removeAttribute("aria-busy");
        dialogForm.querySelectorAll("input, button").forEach(function (control) { control.disabled = saved; });
        personDialog.querySelector("[data-person-dialog-cancel]").disabled = false;
      }
    });
  }

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
        var response = await fetch("/photos/people?format=json&known=1&q=" + encodeURIComponent(query), {
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
          option.textContent = person.create ? "Neu anlegen: „" + person.name + "“" : (person.name || "Unbenannt") + " (#" + person.id + ", " +
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
