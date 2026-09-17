(function () {
  "use strict";
  var peopleSorts = ["name_asc", "name_desc", "count_asc", "count_desc", "folder_asc", "folder_desc", "date_asc", "date_desc"];
  var pageNavigation = document.querySelector("[data-people-page]");
  var pageStorageKey = pageNavigation ? "bearstack.people.lastPage:" + pageNavigation.dataset.peopleUser : "";
  function savePeoplePage(page) {
    if (!pageStorageKey) return;
    var address = new URL(window.location.href);
    if (address.pathname !== "/photos/people") return;
    var mode = address.searchParams.get("filter");
    var unknown = mode ? mode === "unknown" : address.searchParams.get("unknown") === "1";
    var saved = { sort: peopleSorts.includes(address.searchParams.get("sort")) ? address.searchParams.get("sort") : "name_asc", page: page, q: address.searchParams.get("q") || "", unknown: unknown, known: mode ? mode === "known" : !unknown && address.searchParams.get("known") === "1", ignored: mode ? mode === "ignored" : !unknown && address.searchParams.get("ignored") === "1" };
    try { window.localStorage.setItem(pageStorageKey, JSON.stringify(saved)); } catch (_) {}
  }
  document.addEventListener("people-page-updated", function (event) { savePeoplePage(event.detail); });
  if (pageNavigation) {
    var address = new URL(window.location.href);
    var saved;
    try { saved = JSON.parse(window.localStorage.getItem(pageStorageKey)); } catch (_) {}
    if (saved && Number.isInteger(saved.page) && saved.page >= 1 && saved.page <= 1000000 && typeof saved.q === "string") {
      var destination = new URL("/photos/people", address.origin);
      destination.searchParams.set("page", String(saved.page));
      destination.searchParams.set("q", saved.q);
      if (peopleSorts.includes(saved.sort)) destination.searchParams.set("sort", saved.sort);
      if (saved.unknown === true) destination.searchParams.set("unknown", "1");
      else {
        if (saved.known === true) destination.searchParams.set("known", "1");
        if (saved.ignored === true) destination.searchParams.set("ignored", "1");
      }
      if (address.pathname === "/photos/people" && !["page", "q", "known", "unknown", "ignored", "filter", "sort"].some(function (key) { return address.searchParams.has(key); })) {
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
  var sortSelect = document.querySelector('[data-people-filter] select[name="sort"]');
  if (sortSelect) sortSelect.addEventListener("change", function () {
    sortSelect.form.requestSubmit();
  });
  var peopleView = document.querySelector("[data-people-view]");
  if (peopleView) {
    var displayMenu = peopleView.querySelector("[data-people-display]");
    var folderSetting = displayMenu.querySelector("[data-display-folders]");
    var countSetting = displayMenu.querySelector("[data-display-count]");
    var sizeSetting = displayMenu.querySelector("[data-display-size]");
    var displayKey = "bearstack.people.display:" + peopleView.dataset.peopleUser;
    var displaySettings;
    try { displaySettings = JSON.parse(window.localStorage.getItem(displayKey)); } catch (_) {}
    if (displaySettings && typeof displaySettings === "object") {
      folderSetting.checked = displaySettings.folders === true;
      countSetting.checked = displaySettings.count !== false;
      sizeSetting.value = ["s", "m", "l"].includes(displaySettings.size) ? displaySettings.size : "s";
    }
    function applyDisplaySettings() {
      peopleView.dataset.showFolders = String(folderSetting.checked);
      peopleView.dataset.showCount = String(countSetting.checked);
      peopleView.dataset.size = sizeSetting.value;
    }
    applyDisplaySettings();
    displayMenu.hidden = false;
    displayMenu.addEventListener("change", function () {
      applyDisplaySettings();
      try { window.localStorage.setItem(displayKey, JSON.stringify({ folders: folderSetting.checked, count: countSetting.checked, size: sizeSetting.value })); } catch (_) {}
    });
    displayMenu.addEventListener("keydown", function (event) {
      if (event.key === "Escape") { displayMenu.open = false; displayMenu.querySelector("summary").focus(); }
    });
    document.addEventListener("click", function (event) { if (!displayMenu.contains(event.target)) displayMenu.open = false; });
  }
  var favoriteStatus = document.querySelector("[data-face-favorite-status]");
  document.querySelectorAll("button[data-face-favorite]:not([data-detail-favorite])").forEach(function (button) {
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
  var selectionMode = false;
  var selectionModeButton = document.querySelector("[data-people-selection-mode]");
  var merge = document.querySelector("[data-people-merge]");
  var mergeButton = document.querySelector("[data-people-merge-button]");
  var editSelectedButton = document.querySelector("[data-people-edit-button]");
  var ignoreSelectedButton = document.querySelector("[data-people-ignore-button]");
  var retryButton = document.querySelector("[data-people-retry]");
  var needsRefresh = false;

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
    if (selectionModeButton) selectionModeButton.disabled = busy;
    if (selectionModeButton) overview.querySelectorAll(".person-overview-card > .person-card").forEach(function (link) {
      if (selectionMode) {
        link.removeAttribute("href");
        link.setAttribute("role", "button");
        link.setAttribute("tabindex", "0");
        link.setAttribute("aria-pressed", String(selected.has(link.parentElement.dataset.personId)));
        link.setAttribute("aria-label", "Person auswählen: " + (link.parentElement.dataset.personName || "Unbenannt"));
      } else if (link.getAttribute("role") === "button") {
        link.href = "/photos/people/" + encodeURIComponent(link.parentElement.dataset.personId);
        link.removeAttribute("role");
        link.removeAttribute("tabindex");
        link.removeAttribute("aria-pressed");
        link.setAttribute("aria-label", "Person anzeigen: " + (link.parentElement.dataset.personName || "Unbenannt"));
      }
    });
    merge.hidden = selected.size < 1;
    mergeButton.hidden = selected.size < 2;
    merge.querySelectorAll("[data-bulk-tags-open]").forEach(function (button) { button.disabled = busy; });
    editSelectedButton.disabled = busy;
    mergeButton.disabled = busy;
    if (ignoreSelectedButton) ignoreSelectedButton.disabled = busy || !selected.size;
    overview.querySelectorAll("[data-ignore-face], [data-person-edit]").forEach(function (button) {
      button.disabled = busy || button.dataset.ignoreSaved === "true";
    });
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
      existing.dataset.faceId = String(person.face_id);
      existing.dataset.searchFaceId = String(person.search_face_id || person.face_id);
      existing.dataset.displayPath = person.display_path || "";
      ["x", "y", "width", "height"].forEach(function (key) { if (person.portrait) existing.dataset[key] = String(person.portrait[key]); else delete existing.dataset[key]; });
      existing.querySelector(".person-card").setAttribute("aria-label", "Person anzeigen: " + name);
      var existingImage = existing.querySelector("img");
      if (existingImage.getAttribute("src") !== thumbnail) existingImage.src = thumbnail;
      existing.querySelector("strong").textContent = name;
      existing.querySelector("strong").hidden = overview.dataset.unknownOnly === "true" && !person.name;
      existing.querySelector("[data-person-count]").textContent = countText;
      var folder = existing.querySelector("[data-person-folder]");
      if (folder) { folder.textContent = person.folder_name || ""; folder.title = person.display_path || ""; }
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
    card.dataset.faceId = String(person.face_id);
    card.dataset.searchFaceId = String(person.search_face_id || person.face_id);
    card.dataset.displayPath = person.display_path || "";
    ["x", "y", "width", "height"].forEach(function (key) { if (person.portrait) card.dataset[key] = String(person.portrait[key]); });
    var link = document.createElement("a");
    link.className = "person-card";
    link.setAttribute("aria-label", "Person anzeigen: " + name);
    link.href = "/photos/people/" + encodeURIComponent(person.id);
    var img = document.createElement("img");
    img.loading = "lazy"; img.width = 160; img.height = 160; img.alt = "";
    img.src = thumbnail;
    var title = document.createElement("strong"); title.textContent = name;
    title.hidden = overview.dataset.unknownOnly === "true" && !person.name;
    var count = document.createElement("span");
    count.textContent = countText; count.dataset.personCount = "";
    var folder = document.createElement("span"); folder.dataset.personFolder = ""; folder.title = person.display_path || ""; folder.textContent = person.folder_name || "";
    if (overview.dataset.unknownOnly === "true") {
      link.append(folder, img, title, count);
    } else link.append(img, title, count, folder);
    card.append(link);
    if (overview.dataset.canIgnore === "true") {
      var label = document.createElement("label");
      label.className = "person-select";
      var checkbox = document.createElement("input");
      checkbox.type = "checkbox"; checkbox.name = "ids"; checkbox.dataset.personSelect = ""; checkbox.value = person.id;
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
      card.append(window.BearStackPersonDialog.createEditButton("Benennen oder zuordnen: " + name));
    }
    return card;
  }

  async function refreshPeople(signal) {
    var url = new URL(window.location.href);
    url.searchParams.set("format", "json");
    async function load() {
      var response = await fetch(url, { credentials: "same-origin", redirect: "error", headers: { Accept: "application/json" }, signal: signal });
      if (!response.ok) throw new Error("Personen konnten nicht geladen werden.");
      var data = await response.json();
      if (!Array.isArray(data.people)) throw new Error("Ungültige Antwort der Personenübersicht.");
      return data;
    }
    var data = await load();
    overview.dataset.unknownOnly = String(data.unknown_only === true);
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
    function pageLabel(element, label, symbol) {
      element.setAttribute("aria-label", label);
      var icon = document.createElement("span"); icon.className = "people-page-symbol";
      icon.setAttribute("aria-hidden", "true"); icon.textContent = symbol;
      var text = document.createElement("span"); text.className = "people-page-label"; text.textContent = label;
      element.append(icon, text);
    }
    var pageSymbols = { first: "«", prev: "‹", next: "›", last: "»" };
    function pageLink(page, label, relation) {
      var link = document.createElement("a");
      var target = new URL(window.location.href);
      target.searchParams.delete("format"); target.searchParams.set("page", String(page));
      link.href = target.pathname + target.search;
      link.className = "secondary-button"; link.rel = relation; link.title = label;
      pageLabel(link, label, pageSymbols[relation]);
      return link;
    }
    function boundary(page, label, relation, enabled) {
      if (enabled) return pageLink(page, label, relation);
      var disabled = document.createElement("span");
      disabled.className = "secondary-button"; disabled.setAttribute("aria-disabled", "true");
      pageLabel(disabled, label, pageSymbols[relation]);
      return disabled;
    }
    if (data.total_pages > 1) parts.append(boundary(1, "Erste Seite", "first", data.has_prev));
    if (data.has_prev) parts.append(pageLink(data.page - 1, "Zurück", "prev"));
    var current = document.createElement("span"); current.className = "people-pagination-current";
    current.setAttribute("aria-current", "page");
    current.setAttribute("aria-label", "Seite " + data.page + " von " + data.total_pages);
    current.dataset.shortPage = data.page + " / " + data.total_pages;
    var currentLabel = document.createElement("span"); currentLabel.className = "people-page-label"; currentLabel.textContent = current.getAttribute("aria-label");
    current.append(currentLabel); parts.append(current);
    if (data.has_next) parts.append(pageLink(data.page + 1, "Weiter", "next"));
    if (data.total_pages > 1) parts.append(boundary(data.total_pages, "Letzte Seite", "last", data.has_next));
    pagination.replaceChildren(parts);
  }

  if (overview && overview.dataset.canIgnore === "true") {
    if (ignoreSelectedButton) ignoreSelectedButton.addEventListener("click", async function () {
      if (busy || !selected.size) return;
      var cards = Array.from(overview.querySelectorAll("[data-person-id]")).filter(function (card) { return selected.has(card.dataset.personId); });
      if (cards.length !== selected.size || cards.some(function (card) { return card.dataset.personName || !card.dataset.faceId; })) return;
      var body = new URLSearchParams({ action: "ignore" });
      cards.forEach(function (card) { body.append("face_id", card.dataset.faceId); });
      busy = true;
      updateSelection();
      overview.setAttribute("aria-busy", "true");
      status.textContent = "Ausgewählte Gesichter werden ignoriert …";
      var saved = false;
      var controller = new AbortController();
      var timer = setTimeout(function () { controller.abort(); }, 20000);
      // An interrupted write may already have committed. Only a fresh read
      // can unlock another action, without replaying the old selection.
      needsRefresh = true;
      try {
        var response = await fetch("/photos/faces/edit", {
          method: "POST", credentials: "same-origin", redirect: "error",
          headers: { Accept: "application/json" }, body: body, signal: controller.signal
        });
        if (!response.ok) {
          needsRefresh = false;
          throw new Error("Gesichter konnten nicht ignoriert werden (HTTP " + response.status + ").");
        }
        var result = await response.json();
        if (result.ok !== true) throw new Error("Ignorieren wurde nicht bestätigt.");
        saved = true;
        selected.clear();
        await refreshPeople(controller.signal);
        needsRefresh = false;
        status.textContent = "Ausgewählte Gesichter ignoriert.";
      } catch (error) {
        status.textContent = needsRefresh
          ? (saved ? "Gesichter ignoriert, aber die Ansicht konnte nicht aktualisiert werden." : "Ignorieren wurde nicht bestätigt.") + " Bitte die Ansicht erneut laden."
          : error.message;
      } finally {
        clearTimeout(timer);
        busy = needsRefresh;
        retryButton.hidden = !needsRefresh;
        overview.removeAttribute("aria-busy");
        updateSelection();
      }
    });
    if (retryButton) retryButton.addEventListener("click", async function () {
      if (!needsRefresh || retryButton.disabled) return;
      retryButton.disabled = true;
      overview.setAttribute("aria-busy", "true");
      var controller = new AbortController();
      var timer = setTimeout(function () { controller.abort(); }, 20000);
      try {
        await refreshPeople(controller.signal);
        selected.clear();
        needsRefresh = false;
        busy = false;
        retryButton.hidden = true;
        status.textContent = "Ansicht aktualisiert. Bitte die Auswahl erneut prüfen.";
      } catch (_) {
        status.textContent = "Ansicht konnte nicht aktualisiert werden. Bitte erneut laden.";
      } finally {
        clearTimeout(timer);
        retryButton.disabled = false;
        overview.removeAttribute("aria-busy");
        updateSelection();
      }
    });
    if (selectionModeButton && overview.dataset.unknownOnly === "true") {
      selectionModeButton.hidden = false;
      selectionModeButton.addEventListener("click", function () {
        if (busy) return;
        selectionMode = !selectionMode;
        selectionModeButton.setAttribute("aria-pressed", String(selectionMode));
        overview.dataset.selectionMode = String(selectionMode);
        updateSelection();
      });
      // Capture the whole tile before link, ignore and dialog handlers run.
      overview.addEventListener("click", function (event) {
        if (!selectionMode) return;
        var card = event.target.closest(".person-overview-card");
        if (!card || !overview.contains(card)) return;
        if (!busy && event.target.closest(".person-select")) return;
        event.preventDefault();
        event.stopImmediatePropagation();
        if (busy) return;
        var input = card.querySelector("[data-person-select]");
        input.checked = !input.checked;
        input.dispatchEvent(new Event("change", { bubbles: true }));
      }, true);
      overview.addEventListener("keydown", function (event) {
        if (!selectionMode || !event.target.matches(".person-card") || !["Enter", " "].includes(event.key)) return;
        event.preventDefault();
        if (!event.repeat) event.target.click();
      });
    }
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

  if (overview) {
    var personDialog = window.BearStackPersonDialog.bind({
      surface: overview,
      status: status,
      isBusy: function () { return busy; },
      onBusy: function (value) { busy = value; updateSelection(); },
      getIgnoreRequest: function (cards) {
        if (!cards.length || cards.some(function (card) { return !card || card.dataset.personName || !card.dataset.faceId; })) return null;
        var body = new URLSearchParams({ action: "ignore" });
        cards.forEach(function (card) { body.append("face_id", card.dataset.faceId); });
        return { action: "/photos/faces/edit", body: body };
      },
      onSave: async function (ids) {
        ids.forEach(function (id) { selected.delete(id); });
        await refreshPeople();
      }
    });
    if (editSelectedButton) editSelectedButton.addEventListener("click", function () { personDialog.open(Array.from(selected), editSelectedButton); });
  }
}());
