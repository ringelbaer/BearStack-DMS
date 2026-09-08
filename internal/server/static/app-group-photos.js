(function () {
  "use strict";
  var surface = document.querySelector("[data-group-photos]");
  if (!surface) return;
  var filter = document.querySelector("[data-group-filter]");
  var minimumInput = filter.querySelector('[name="min"]');
  var grid = surface.querySelector("[data-group-grid]");
  var stage = surface.querySelector("[data-group-stage]");
  var image = surface.querySelector("[data-group-image]");
  var box = surface.querySelector("[data-group-box]");
  var status = surface.querySelector("[data-people-status]");
  var ignoreForm = surface.querySelector("[data-group-ignore-form]");
  var ignoreButton = surface.querySelector("[data-group-ignore]");
  var skip = surface.querySelector("[data-group-skip]");
  var retry = surface.querySelector("[data-group-retry]");
  var minimum = Number(surface.dataset.minimum);
  var currentPath = surface.dataset.path || "";
  var remaining = grid.querySelectorAll('[data-person-name=""][data-ignored="false"]').length;
  var busy = false, navigationPending = false, request = 0, controller, retryParams;
  var hovered, focused, zoomed;
  var storageKey = "bearstack.people.groupMinimum:" + surface.dataset.groupUser;

  function setBusy(value) {
    busy = value;
    surface.setAttribute("aria-busy", String(value));
    filter.querySelectorAll("input, button").forEach(function (control) { control.disabled = value; });
    grid.querySelectorAll("button").forEach(function (control) { control.disabled = value || navigationPending; });
    ignoreButton.disabled = value || navigationPending || !remaining;
    skip.setAttribute("aria-disabled", String(value));
    retry.disabled = value;
  }

  function faceBounds(card) {
    if (!card || !card.isConnected) return null;
    var x = Number(card.dataset.x), y = Number(card.dataset.y);
    var width = Number(card.dataset.width), height = Number(card.dataset.height);
    if (![x, y, width, height].every(Number.isFinite) || width <= 0 || height <= 0) return null;
    var left = Math.max(0, Math.min(1, x)), top = Math.max(0, Math.min(1, y));
    var right = Math.max(left, Math.min(1, x + width)), bottom = Math.max(top, Math.min(1, y + height));
    if (right === left || bottom === top) return null;
    return { left: left, top: top, width: right - left, height: bottom - top };
  }

  function setZoom(card) {
    if (zoomed) zoomed.querySelector("[data-group-highlight]").setAttribute("aria-pressed", "false");
    zoomed = card;
    if (zoomed) zoomed.querySelector("[data-group-highlight]").setAttribute("aria-pressed", "true");
  }

  function highlight() {
    box.hidden = true;
    if (zoomed && !faceBounds(zoomed)) setZoom(null);
    if (!image.complete || !image.naturalWidth || !image.naturalHeight) { image.style.transform = ""; return; }
    var frame = stage.getBoundingClientRect();
    var frameWidth = frame.width, frameHeight = frame.height;
    if (!frameWidth || !frameHeight) { image.style.transform = ""; return; }
    var scale = Math.min(frameWidth / image.naturalWidth, frameHeight / image.naturalHeight);
    var displayWidth = image.naturalWidth * scale, displayHeight = image.naturalHeight * scale;
    var baseLeft = (frameWidth - displayWidth) / 2, baseTop = (frameHeight - displayHeight) / 2;
    var left = baseLeft, top = baseTop;
    var region = faceBounds(zoomed);
    if (region) {
      // Fit a region twice the face's width and height; retain context at photo edges.
      var zoom = Math.max(1, Math.min(frameWidth / (2 * region.width * displayWidth), frameHeight / (2 * region.height * displayHeight)));
      displayWidth *= zoom; displayHeight *= zoom;
      left = frameWidth / 2 - (region.left + region.width / 2) * displayWidth;
      top = frameHeight / 2 - (region.top + region.height / 2) * displayHeight;
      left = displayWidth <= frameWidth ? (frameWidth - displayWidth) / 2 : Math.max(frameWidth - displayWidth, Math.min(0, left));
      top = displayHeight <= frameHeight ? (frameHeight - displayHeight) / 2 : Math.max(frameHeight - displayHeight, Math.min(0, top));
      image.style.transform = "translate(" + (left - baseLeft * zoom) + "px, " + (top - baseTop * zoom) + "px) scale(" + zoom + ")";
    } else image.style.transform = "";
    region = region || faceBounds(hovered || focused);
    if (!region) return;
    box.style.left = (left + region.left * displayWidth) + "px";
    box.style.top = (top + region.top * displayHeight) + "px";
    box.style.width = (region.width * displayWidth) + "px";
    box.style.height = (region.height * displayHeight) + "px";
    box.hidden = false;
  }
  grid.addEventListener("click", function (event) {
    var preview = event.target.closest("[data-group-highlight]");
    if (!preview || busy || navigationPending || !image.complete || !image.naturalWidth) return;
    var card = preview.closest("[data-group-face]");
    if (!faceBounds(card)) return;
    setZoom(zoomed === card ? null : card);
    highlight();
  });
  grid.addEventListener("pointerover", function (event) { hovered = event.target.closest("[data-group-face]"); highlight(); });
  grid.addEventListener("pointerleave", function () { hovered = null; highlight(); });
  grid.addEventListener("focusin", function (event) { focused = event.target.closest("[data-group-face]"); highlight(); });
  grid.addEventListener("focusout", function () { focused = null; highlight(); });
  image.addEventListener("load", highlight);
  function imageFailed() {
    if (!currentPath) return;
    setZoom(null); image.style.transform = "";
    box.hidden = true;
    status.textContent = "Das Foto konnte nicht geladen werden. Du kannst die Ansicht erneut laden oder das Foto überspringen.";
    retryParams = { path: currentPath }; retry.hidden = false;
  }
  image.addEventListener("error", imageFailed);
  if (image.getAttribute("src") && image.complete && !image.naturalWidth) imageFailed();
  if (window.ResizeObserver) new ResizeObserver(highlight).observe(stage);
  else window.addEventListener("resize", highlight);

  function faceCard(face, existing) {
    var card = existing || document.createElement("div");
    card.className = "group-photo-face person-card";
    card.dataset.groupFace = String(face.id);
    card.dataset.personId = String(face.person_id);
    card.dataset.personName = face.name || "";
    card.dataset.ignored = String(face.ignored);
    ["x", "y", "width", "height"].forEach(function (key) { card.dataset[key] = String(face[key]); });
    var preview = card.querySelector("[data-group-highlight]");
    if (!preview) {
      preview = document.createElement("button"); preview.type = "button"; preview.className = "group-face-highlight"; preview.dataset.groupHighlight = "";
      var img = document.createElement("img"); img.width = 160; img.height = 160; img.loading = "lazy"; img.alt = "Gesicht";
      img.src = "/photos/faces/" + encodeURIComponent(face.id) + "/thumbnail";
      preview.append(img); card.append(preview);
    }
    var name = face.name || (face.ignored ? "Ignoriert" : "Unbenannt");
    preview.setAttribute("aria-label", "Gesicht im Foto vergrößern: " + name);
    preview.setAttribute("aria-pressed", String(card === zoomed));
    var title = card.querySelector("[data-group-name]");
    if (!title) { title = document.createElement("strong"); title.dataset.groupName = ""; card.append(title); }
    title.textContent = name;
    var ignored = card.querySelector("[data-group-ignored-label]");
    if (face.name && face.ignored && !ignored) { ignored = document.createElement("span"); ignored.dataset.groupIgnoredLabel = ""; ignored.textContent = "Ignoriert"; card.append(ignored); }
    if ((!face.name || !face.ignored) && ignored) ignored.remove();
    var edit = card.querySelector("[data-person-edit]");
    if (!face.ignored && !face.name && !edit) {
      edit = window.BearStackPersonDialog.createEditButton("Person benennen oder zuordnen");
      card.append(edit);
    }
    if ((face.ignored || face.name) && edit) edit.remove();
    var ignore = card.querySelector("[data-group-ignore-face]");
    if (!face.ignored && !face.name && !ignore) {
      ignore = document.createElement("button"); ignore.type = "submit"; ignore.className = "person-ignore-button";
      ignore.setAttribute("form", ignoreForm.id); ignore.name = "face_id"; ignore.value = String(face.id); ignore.dataset.groupIgnoreFace = "";
      ignore.setAttribute("aria-label", "Dieses Gesicht ignorieren"); ignore.title = "Dieses Gesicht ignorieren"; ignore.textContent = "×";
      card.insertBefore(ignore, edit || null);
    }
    if ((face.ignored || face.name) && ignore) ignore.remove();
    return card;
  }

  function render(page, params) {
    var photo = page.photo;
    if (photo && (!Array.isArray(photo.faces) || typeof photo.path !== "string" || typeof photo.revision !== "string")) throw new Error("Ungültige Gruppenbild-Antwort.");
    var changedPhoto = !photo || photo.path !== currentPath;
    currentPath = photo ? photo.path : "";
    surface.dataset.path = currentPath;
    surface.dataset.revision = photo ? photo.revision : "";
    remaining = photo ? photo.remaining : 0;
    surface.querySelector("[data-group-layout]").hidden = !photo;
    surface.querySelector("[data-group-toolbar]").hidden = !photo;
    surface.querySelector("[data-group-empty]").hidden = Boolean(photo);
    surface.querySelector("[data-group-title]").textContent = photo ? photo.display_path : "";
    surface.querySelector("[data-group-count]").textContent = photo ? remaining + " unbearbeitete von " + photo.faces.length + " erkannten Gesichtern" : "";
    ignoreForm.elements.path.value = currentPath;
    ignoreForm.elements.revision.value = surface.dataset.revision;
    ignoreForm.elements.min.value = String(minimum);
    var nextURL = new URL("/photos/people/groups", window.location.origin);
    nextURL.searchParams.set("min", String(minimum)); nextURL.searchParams.set("after", currentPath);
    skip.href = nextURL.pathname + nextURL.search;
    var old = new Map();
    grid.querySelectorAll("[data-group-face]").forEach(function (card) { old.set(card.dataset.groupFace, card); });
    var cards = document.createDocumentFragment();
    if (photo) photo.faces.forEach(function (face) { cards.append(faceCard(face, old.get(String(face.id)))); old.delete(String(face.id)); });
    old.forEach(function (card) { card.remove(); });
    grid.append(cards);
    if (changedPhoto) { hovered = null; focused = null; setZoom(null); image.style.transform = ""; box.hidden = true; }
    if (photo && photo.image_face_id) {
      // Ignoring the image's reference face does not change the photo. Keep the
      // loaded preview while that detection still exists, including when ignored.
      var source = image.getAttribute("src");
      if (changedPhoto || !photo.faces.some(function (face) { return source === "/photos/people/groups/image/" + encodeURIComponent(face.id); })) {
        source = "/photos/people/groups/image/" + encodeURIComponent(photo.image_face_id);
      }
      if (image.getAttribute("src") !== source || (image.complete && !image.naturalWidth)) image.src = source;
      image.alt = photo.display_path;
    } else if (changedPhoto) image.removeAttribute("src");
    navigationPending = false;
    retry.hidden = true;
    var address = new URL("/photos/people/groups", window.location.origin);
    address.searchParams.set("min", String(minimum));
    if (photo) address.searchParams.set("path", photo.path);
    else if (params.after) address.searchParams.set("after", params.after);
    window.history.replaceState(null, "", address.pathname + address.search);
    highlight();
  }

  async function loadPhoto(params) {
    var serial = ++request;
    if (controller) controller.abort();
    controller = new AbortController();
    retryParams = params;
    setBusy(true);
    var address = new URL("/photos/people/groups", window.location.origin);
    address.searchParams.set("format", "json"); address.searchParams.set("min", String(minimum));
    Object.keys(params).forEach(function (key) { address.searchParams.set(key, params[key]); });
    try {
      var response = await fetch(address, { signal: controller.signal, credentials: "same-origin", redirect: "error", headers: { Accept: "application/json" } });
      var result = await response.json();
      if (!response.ok) throw new Error(result.error || "Das Gruppenbild konnte nicht geladen werden.");
      if (serial === request) render(result, params);
    } catch (error) {
      if (serial === request && error.name !== "AbortError") { retry.hidden = false; status.textContent = error.message; }
      throw error;
    } finally {
      if (serial === request) setBusy(false);
    }
  }

  skip.addEventListener("click", function (event) {
    event.preventDefault();
    if (busy || !currentPath) return;
    status.textContent = "";
    loadPhoto({ after: currentPath }).catch(function () {});
  });
  ignoreForm.addEventListener("submit", async function (event) {
    event.preventDefault();
    if (busy || navigationPending || !remaining) return;
    var submitter = event.submitter;
    if (!submitter || (submitter !== ignoreButton && !submitter.hasAttribute("data-group-ignore-face"))) return;
    var single = submitter !== ignoreButton;
    var faceID = single ? submitter.value : "";
    var focusPreview = single && document.activeElement === submitter;
    var path = currentPath;
    var body = new URLSearchParams(new FormData(ignoreForm));
    if (single) body.set("face_id", faceID);
    setBusy(true); status.textContent = "";
    try {
      var response = await fetch(ignoreForm.action, { method: "POST", credentials: "same-origin", redirect: "error", headers: { Accept: "application/json" }, body: body });
      var result = await response.json();
      if (!response.ok || result.ok !== true) {
        if (response.status === 409) await loadPhoto({ path: path });
        throw new Error(result.error || "Die Gesichter konnten nicht ignoriert werden.");
      }
      navigationPending = true;
      await loadPhoto(single ? { path: path } : { after: path });
      status.textContent = single ? "Gesicht ignoriert." : result.ignored + " Gesichter ignoriert.";
      if (focusPreview) {
        var card = grid.querySelector('[data-group-face="' + faceID + '"]');
        if (card) card.querySelector("[data-group-highlight]").focus({ preventScroll: true });
      }
    } catch (error) {
      status.textContent = navigationPending ? (single ? "Gesicht gespeichert. Die Ansicht konnte nicht aktualisiert werden; bitte die Ansicht erneut laden." : "Gesichter gespeichert. Das nächste Foto konnte nicht geladen werden; bitte die Ansicht erneut laden.") : error.message;
    } finally { setBusy(false); }
  });
  retry.addEventListener("click", function () {
    if (busy || !retryParams) return;
    status.textContent = "";
    loadPhoto(retryParams).catch(function () {});
  });
  window.BearStackPersonDialog.bind({
    surface: surface,
    status: status,
    isBusy: function () { return busy || navigationPending; },
    onBusy: setBusy,
    onSave: function () { return loadPhoto({ path: currentPath }); }
  });
  filter.addEventListener("submit", function () { try { window.localStorage.setItem(storageKey, minimumInput.value); } catch (_) {} });
  if (!new URL(window.location.href).searchParams.has("min")) {
    var saved;
    try { saved = window.localStorage.getItem(storageKey); } catch (_) {}
    if (saved !== null && saved !== undefined && /^\d+$/.test(saved) && Number(saved) <= 255 && Number(saved) !== minimum) {
      minimum = Number(saved); minimumInput.value = saved;
      loadPhoto({}).catch(function () {});
    }
  }
}());
