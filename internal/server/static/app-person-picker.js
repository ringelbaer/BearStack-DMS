// Reusable person search/selection. Callers own form actions, validation and submission.
(function () {
  "use strict";
  if (window.BearStackPersonPicker) return;
  var instances = new WeakMap();
  function bind(element, options) {
    if (instances.has(element)) return instances.get(element);
    options = options || {};
    var input = element.querySelector("[data-person-search]");
    var target = element.querySelector("[data-person-target]");
    var list = element.querySelector("[data-person-options]");
    var popup = element.querySelector("[data-person-popup]");
    var feedback = element.querySelector("[data-person-feedback]");
    var matchButton = element.querySelector("[data-person-face-match]");
    var matching = false, loading = false;
    var allowCreate = !!options.allowCreate;
    var items = [], active = -1, revision = 0, pointerPerson, touchChoice;
    var timer, controller, pendingDirection, renderFrame, pendingRender;

    function cancel() {
      clearTimeout(timer);
      if (renderFrame) cancelAnimationFrame(renderFrame);
      renderFrame = 0;
      pendingRender = null;
      if (controller) controller.abort();
      revision++;
      matching = false; loading = false;
      if (matchButton) matchButton.removeAttribute("aria-busy");
    }
    function close() {
      cancel();
      popup.hidden = true;
      input.setAttribute("aria-expanded", "false");
      input.removeAttribute("aria-activedescendant");
      active = -1;
      pointerPerson = undefined;
      touchChoice = null;
    }
    function selection() {
      return { name: input.value.trim(), target: target.value,
        assigned: !!target.value && target.value !== "0",
        revision: target.dataset.revision || "", targetName: target.dataset.name || "",
        searching: loading, matching: matching };
    }
    function changed() { if (options.onChange) options.onChange(selection()); }
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
      target.dataset.name = person.name || "";
      input.value = person.create ? person.name : (person.name || "Unbenannt") + " (#" + person.id + ")";
      changed();
      close();
      if (options.onChoose) options.onChoose(selection());
    }
    async function load(direction, faceMatch) {
      cancel();
      matching = !!faceMatch; loading = true;
      var context = options.context ? options.context() : {};
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
        var url = faceMatch ? "/photos/faces/" + encodeURIComponent(context.faceID) + "/suggestions" : (context.suggestionsURL || "/photos/people?format=suggestions&q=") + encodeURIComponent(query);
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
        items = data.people.filter(function (person) { return typeof person.name === "string" && person.name.trim() && String(person.id) !== context.exclude; });
        if (allowCreate && query && !faceMatch) items.push({ id: 0, name: query, create: true });
        var fragment = document.createDocumentFragment();
        items.forEach(function (person, index) {
          var key = JSON.stringify(person);
          var option = previous.get(key);
          if (option) {
            option.id = list.id + "-" + index;
            option.dataset.personOption = String(index);
            fragment.append(option);
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
          if (options.showThumbnails && !person.create && Number.isSafeInteger(person.face_id) && person.face_id > 0) {
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
          fragment.append(option);
        });
        list.replaceChildren(fragment);
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
        if (request === revision) { loading = false; matching = false; if (matchButton) matchButton.removeAttribute("aria-busy"); }
      }
    }
    if (matchButton) {
      matchButton.addEventListener("pointerdown", function (event) { event.preventDefault(); });
      matchButton.addEventListener("click", function () {
        var context = options.context ? options.context() : {};
        if (input.disabled || matching || !context.faceID) return;
        input.focus({ preventScroll: true });
        target.value = "";
        changed();
        load(undefined, true);
      });
    }
    input.addEventListener("focus", function () { load(); });
    input.addEventListener("click", function () { if (popup.hidden) load(); });
    input.addEventListener("input", function (event) {
      close();
      target.value = !input.required && !input.value.trim() ? "0" : "";
      changed();
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
      if (event.isPrimary === false || event.button !== 0) return;
      var option = event.target.closest("[data-person-option]");
      if (option) {
        pointerPerson = items[Number(option.dataset.personOption)].id;
        touchChoice = event.pointerType === "touch" ? { id: event.pointerId, x: event.clientX, y: event.clientY } : null;
        event.preventDefault();
      }
    });
    // Touch release remains reliable when the browser suppresses the follow-up
    // click after drawing. Scrolling/cancelled gestures never select a person.
    list.addEventListener("pointercancel", function () { touchChoice = null; pointerPerson = undefined; });
    list.addEventListener("pointermove", function (event) {
      if (touchChoice && touchChoice.id === event.pointerId &&
          (Math.abs(event.clientX - touchChoice.x) > 10 || Math.abs(event.clientY - touchChoice.y) > 10)) touchChoice = null;
    });
    list.addEventListener("pointerup", function (event) {
      var touch = touchChoice;
      touchChoice = null;
      if (!touch || touch.id !== event.pointerId) return;
      if (Math.abs(event.clientX - touch.x) <= 10 && Math.abs(event.clientY - touch.y) <= 10) {
        choose(items.findIndex(function (person) { return person.id === pointerPerson; }));
      }
      pointerPerson = undefined;
    });
    list.addEventListener("click", function (event) {
      if (event.pointerType === "touch") return;
      var option = event.target.closest("[data-person-option]");
      if (option) choose(pointerPerson !== undefined ? items.findIndex(function (person) { return person.id === pointerPerson; }) : Number(option.dataset.personOption));
      pointerPerson = undefined;
    });
    input.addEventListener("blur", close);
    var picker = {
      selection: selection,
      close: close,
      reset: function (name) {
        close();
        target.value = "";
        input.value = name || "";
        changed();
      }
    };
    instances.set(element, picker);
    changed();
    return picker;
  }
  window.BearStackPersonPicker = { bind: bind };
}());
