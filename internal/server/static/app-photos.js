(function () {
  var photoSelectionControllers = new WeakMap();
  var photoDetailBatchWait = new Map();
  var photoDetailBatchTimer = 0;
  var photoDetailBatchInFlight = false;
  var photoDetailBatchLimit = 100;
  var photoJustifiedGuardrailCardLimit = 800;
  var photoJustifiedDeferredBatchSize = 2;
  var photoMap = (window.BearStack && window.BearStack.photos && window.BearStack.photos.map) || {};
  var photoThumbnails = (window.BearStack && window.BearStack.photos && window.BearStack.photos.thumbnails) || {};

  function photoModule() {
    return document.querySelector("[data-photo-module]");
  }

  function isPhotoEditMode() {
    var module = photoModule();
    return module && module.dataset.photoMode === "edit";
  }

  function collectItems(root) {
    return Array.from(root.querySelectorAll("[data-photo-item]")).map(photoItemFromNode).filter(function (item) {
      return item.path || item.src || item.thumb;
    });
  }

  function photoItemFromNode(node) {
    var type = node.dataset.photoType || "image";
    var original = node.dataset.photoSrc || "";
    var preview = node.dataset.photoPreview || "";
    var largePreview = node.dataset.photoLargePreview || "";
    var thumb = node.dataset.photoThumb || "";
    var directMedia = type === "video" || type === "audio";
    return {
      node: node,
      path: node.dataset.photoPath || "",
      src: directMedia ? original : (largePreview || preview || thumb || original),
      original: original,
      preview: preview,
      largePreview: largePreview,
      thumb: thumb,
      type: type,
      title: node.dataset.photoTitle || "Foto",
      date: node.dataset.photoDate || "-",
      camera: node.dataset.photoCamera || "-",
      lens: node.dataset.photoLens || "-",
      rating: node.dataset.photoRating || "",
      size: node.dataset.photoSize || "-",
      resolution: node.dataset.photoResolution || "-",
      width: positiveNumber(node.dataset.photoWidth),
      height: positiveNumber(node.dataset.photoHeight),
      coords: node.dataset.photoCoords || "-",
      lat: node.dataset.photoLat || "",
      lon: node.dataset.photoLon || "",
      detailsLoaded: Boolean(original || preview || largePreview),
      detailPromise: null
    };
  }

  function applyPhotoItemDetails(item, data) {
    if (!item || !data) return item;
    item.path = data.path || item.path || "";
    item.src = data.src || item.src || "";
    item.original = data.original || item.original || "";
    item.preview = data.preview || item.preview || "";
    item.largePreview = data.large_preview || data.largePreview || item.largePreview || "";
    item.thumb = data.thumb || item.thumb || "";
    item.type = data.type || item.type || "image";
    item.title = data.title || data.name || item.title || "Foto";
    item.date = data.date || item.date || "-";
    item.camera = data.camera || "-";
    item.lens = data.lens || "-";
    item.rating = data.rating || "";
    item.size = data.size || "-";
    item.resolution = data.resolution || "-";
    item.width = positiveNumber(data.width || item.width);
    item.height = positiveNumber(data.height || item.height);
    item.coords = data.coords || "-";
    item.lat = data.lat || "";
    item.lon = data.lon || "";
    item.detailsLoaded = true;
    return item;
  }

  function ensurePhotoItemDetails(item) {
    if (!item || item.detailsLoaded || !item.path || typeof window.fetch !== "function") {
      if (item) item.detailsLoaded = true;
      return Promise.resolve(item);
    }
    if (item.detailPromise) return item.detailPromise;
    item.detailPromise = new Promise(function (resolve) {
      queuePhotoDetailRequest(item, resolve);
    });
    return item.detailPromise;
  }

  function queuePhotoDetailRequest(item, resolve) {
    var entry = photoDetailBatchWait.get(item.path);
    if (!entry) {
      entry = { path: item.path, requests: [] };
      photoDetailBatchWait.set(item.path, entry);
    }
    entry.requests.push({ item: item, resolve: resolve });
    schedulePhotoDetailBatch();
  }

  function schedulePhotoDetailBatch() {
    if (photoDetailBatchTimer || photoDetailBatchInFlight) return;
    photoDetailBatchTimer = window.setTimeout(function () {
      photoDetailBatchTimer = 0;
      flushPhotoDetailBatch();
    }, 0);
  }

  function finishPhotoDetailRequest(request, details) {
    if (details) {
      applyPhotoItemDetails(request.item, details);
    } else {
      request.item.detailsLoaded = true;
    }
    request.item.detailPromise = null;
    request.resolve(request.item);
  }

  function flushPhotoDetailBatch() {
    if (photoDetailBatchInFlight || photoDetailBatchWait.size === 0 || typeof window.fetch !== "function") return;
    var entries = [];
    photoDetailBatchWait.forEach(function (entry, key) {
      if (entries.length >= photoDetailBatchLimit) return;
      entries.push(entry);
      photoDetailBatchWait.delete(key);
    });
    if (!entries.length) return;
    photoDetailBatchInFlight = true;
    window.fetch("/photos/media/info", {
      method: "POST",
      credentials: "same-origin",
      headers: { Accept: "application/json", "Content-Type": "application/json" },
      body: JSON.stringify({ paths: entries.map(function (entry) { return entry.path; }) })
    }).then(function (response) {
      if (!response.ok) throw new Error("Foto-Details konnten nicht geladen werden");
      return response.json();
    }).catch(function () {
      return { media: [] };
    }).then(function (payload) {
      var detailsByPath = new Map();
      (payload.media || []).forEach(function (details) {
        if (details && details.path) {
          detailsByPath.set(details.path, details);
        }
      });
      entries.forEach(function (entry) {
        var details = detailsByPath.get(entry.path);
        entry.requests.forEach(function (request) {
          finishPhotoDetailRequest(request, details);
        });
      });
    }).finally(function () {
      photoDetailBatchInFlight = false;
      if (photoDetailBatchWait.size > 0) {
        schedulePhotoDetailBatch();
      }
    });
  }

  function positiveNumber(value) {
    var number = Number.parseFloat(value || "");
    return Number.isFinite(number) && number > 0 ? number : 0;
  }

  function imageURLSize(url) {
    if (!url) return 0;
    try {
      return positiveNumber(new URL(url, window.location.href).searchParams.get("size"));
    } catch (_) {
      return 0;
    }
  }

  function desiredPhotoDisplaySize() {
    var width = window.innerWidth || document.documentElement.clientWidth || 0;
    var height = window.innerHeight || document.documentElement.clientHeight || 0;
    var ratio = Math.max(1, window.devicePixelRatio || 1);
    return Math.ceil(Math.max(width, height) * ratio);
  }

  function bestPhotoDisplaySrc(item) {
    if (!item) return "";
    if (item.type !== "image") return item.src || item.original || "";
    var candidates = [
      { src: item.thumb, size: imageURLSize(item.thumb) },
      { src: item.preview, size: imageURLSize(item.preview) },
      { src: item.largePreview, size: imageURLSize(item.largePreview) }
    ].filter(function (candidate) {
      return candidate.src;
    }).sort(function (a, b) {
      return a.size - b.size;
    });
    if (!candidates.length) return item.original || item.src || "";
    var desired = desiredPhotoDisplaySize();
    for (var i = 0; i < candidates.length; i += 1) {
      if (!candidates[i].size || candidates[i].size >= desired) {
        return candidates[i].src;
      }
    }
    return candidates[candidates.length - 1].src;
  }

  function formatPhotoRating(value) {
    if (value === "") return "-";
    var rating = Number.parseFloat(value || "");
    if (!Number.isFinite(rating)) return "-";
    if (rating < 0) return "Abgelehnt";
    if (rating === 0) return "Unbewertet";
    var rounded = Math.round(rating * 10) / 10;
    var text = Number.isInteger(rounded) ? String(rounded) : String(rounded).replace(".", ",");
    return text + " " + (rounded === 1 ? "Stern" : "Sterne");
  }

  function photoCardSize(card) {
    var width = positiveNumber(card.dataset.photoWidth);
    var height = positiveNumber(card.dataset.photoHeight);
    if ((!width || !height) && card.dataset.photoResolution) {
      var match = card.dataset.photoResolution.match(/(\d+(?:[.,]\d+)?)\s*[x×]\s*(\d+(?:[.,]\d+)?)/i);
      if (match) {
        width = positiveNumber(match[1].replace(",", "."));
        height = positiveNumber(match[2].replace(",", "."));
      }
    }
    return { width: width, height: height };
  }

  function photoCardRatio(card) {
    var size = photoCardSize(card);
    return size.width && size.height ? size.width / size.height : 1;
  }

  function photoCardThumbnailSize(card) {
    var src = card.dataset.photoThumb || card.querySelector("[data-photo-thumb-src]")?.dataset.photoThumbSrc || "";
    if (!src) return 0;
    try {
      return positiveNumber(new URL(src, window.location.href).searchParams.get("size"));
    } catch (_) {
      return 0;
    }
  }

  function photoCardMaxImageHeight(card, ratio) {
    var thumbnailSize = photoCardThumbnailSize(card);
    if (!thumbnailSize) return Number.POSITIVE_INFINITY;
    var size = photoCardSize(card);
    var sourceLongEdge = thumbnailSize;
    if (size.width && size.height) {
      sourceLongEdge = Math.min(sourceLongEdge, Math.max(size.width, size.height));
    }
    return sourceLongEdge / Math.max(1, ratio);
  }

  function photoGalleryCards(gallery) {
    return Array.from(gallery.children).filter(function (child) {
      return child.classList && child.classList.contains("photo-card");
    });
  }

  function photoGalleryCardContainers(gallery) {
    var groups = Array.from(gallery.querySelectorAll("[data-photo-date-group-media]"));
    return groups.length ? groups : [gallery];
  }

  function photoGalleryContentWidth(gallery) {
    var style = window.getComputedStyle(gallery);
    var padding = positiveNumber(style.paddingLeft) + positiveNumber(style.paddingRight);
    return Math.max(0, gallery.clientWidth - padding);
  }

  function calcPhotoRowHeight(row, containerWidth, margin) {
    var ratioSum = row.reduce(function (sum, item) {
      return sum + item.ratio;
    }, 0);
    if (ratioSum <= 0) return 0;
    return ((containerWidth - row.length * (margin * 2) - 1) / ratioSum) + margin * 2;
  }

  function maxPhotoRowImageHeight(row) {
    return row.reduce(function (limit, item) {
      return Math.min(limit, item.maxImageHeight);
    }, Number.POSITIVE_INFINITY);
  }

  function layoutPhotoGalleryContainer(container, containerWidth) {
    var cards = photoGalleryCards(container);
    if (!cards.length) return;

    // Medium grid defaults: target 5 columns and keep row height within 2-5 viewport rows.
    var targetColumnCount = 5;
    var minRowCount = 2;
    var maxRowCount = 5;
    var margin = 2;
    var screenHeight = window.innerHeight || document.documentElement.clientHeight || 800;
    var minRowHeight = screenHeight / maxRowCount;
    var maxRowHeight = screenHeight / minRowCount;
    var index = 0;

    var items = cards.map(function (card) {
      var ratio = photoCardRatio(card);
      return {
        card: card,
        ratio: ratio,
        maxImageHeight: photoCardMaxImageHeight(card, ratio)
      };
    });

    while (index < items.length) {
      var row = [];
      var nextIndex = index;
      var addItem = function () {
        if (nextIndex >= items.length) return false;
        row.push(items[nextIndex]);
        nextIndex += 1;
        return true;
      };

      for (var count = 0; count < targetColumnCount; count++) {
        addItem();
      }

      while (calcPhotoRowHeight(row, containerWidth, margin) > maxRowHeight && addItem()) {}
      while (calcPhotoRowHeight(row, containerWidth, margin) < minRowHeight && row.length > 1) {
        row.pop();
        nextIndex -= 1;
      }
      if (!row.length) {
        addItem();
      }
      while (calcPhotoRowHeight(row, containerWidth, margin) - margin * 2 > maxPhotoRowImageHeight(row) && addItem()) {}

      var rowMaxHeight = maxRowHeight;
      if (row.length > 1) {
        rowMaxHeight *= 1.2;
      }
      var calculatedHeight = calcPhotoRowHeight(row, containerWidth, margin);
      var rowHeight = calculatedHeight > rowMaxHeight ? (minRowHeight + rowMaxHeight) / 2 : Math.min(calculatedHeight, rowMaxHeight);
      var imageHeight = Math.max(1, rowHeight - margin * 2);
      var maxImageHeight = maxPhotoRowImageHeight(row);
      if (Number.isFinite(maxImageHeight)) {
        imageHeight = Math.min(imageHeight, Math.max(1, maxImageHeight));
      }

      row.forEach(function (item) {
        item.card.style.width = (imageHeight * item.ratio).toFixed(3) + "px";
        item.card.style.margin = margin + "px";
        item.card.style.setProperty("--photo-thumb-height", imageHeight.toFixed(3) + "px");
      });

      index = nextIndex;
    }
  }

  function layoutPhotoGallery(gallery, containers, containerWidth) {
    gallery.classList.add("is-justified");
    containerWidth = containerWidth || photoGalleryContentWidth(gallery);
    if (containerWidth <= 0) return;
    (containers || photoGalleryCardContainers(gallery)).forEach(function (container) {
      layoutPhotoGalleryContainer(container, containerWidth);
    });
  }

  function photoContainerLikelyVisible(container) {
    if (!container || typeof container.getBoundingClientRect !== "function") return true;
    var rect = container.getBoundingClientRect();
    var viewportHeight = window.innerHeight || document.documentElement.clientHeight || 0;
    var margin = 520;
    return rect.bottom >= -margin && rect.top <= viewportHeight + margin;
  }

  function initPhotoJustifiedLayout() {
    var galleries = Array.from(document.querySelectorAll("[data-photo-gallery]")).filter(function (gallery) {
      var totalCards = gallery.querySelectorAll(".photo-card").length;
      if (totalCards > photoJustifiedGuardrailCardLimit) {
        gallery.classList.remove("is-justified");
        gallery.dataset.photoJustifiedGuardrail = "1";
        return false;
      }
      delete gallery.dataset.photoJustifiedGuardrail;
      return true;
    });
    if (!galleries.length) return;
    var frame = 0;
    var containerToGallery = new WeakMap();
    var visibilityObserver = "IntersectionObserver" in window ? new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (!entry.isIntersecting) return;
        visibilityObserver.unobserve(entry.target);
        var gallery = containerToGallery.get(entry.target);
        if (!gallery) return;
        var width = photoGalleryContentWidth(gallery);
        if (width <= 0) return;
        layoutPhotoGallery(gallery, [entry.target], width);
      });
    }, { rootMargin: "640px 0px", threshold: 0.01 }) : null;
    var deferredFrame = 0;
    var deferredLayouts = [];
    var deferredPending = new Set();

    function cancelDeferred() {
      if (deferredFrame) {
        window.cancelAnimationFrame(deferredFrame);
        deferredFrame = 0;
      }
      deferredLayouts = [];
      deferredPending.clear();
    }

    function enqueueDeferredLayout(gallery, container, width) {
      if (deferredPending.has(container)) return;
      deferredPending.add(container);
      deferredLayouts.push({ gallery: gallery, container: container, width: width });
    }

    function flushDeferred() {
      deferredFrame = 0;
      if (!deferredLayouts.length) return;
      var batch = deferredLayouts.splice(0, photoJustifiedDeferredBatchSize);
      batch.forEach(function (entry) {
        deferredPending.delete(entry.container);
        layoutPhotoGallery(entry.gallery, [entry.container], entry.width);
      });
      if (deferredLayouts.length) {
        deferredFrame = window.requestAnimationFrame(flushDeferred);
      }
    }

    function render() {
      frame = 0;
      cancelDeferred();
      galleries.forEach(function (gallery) {
        gallery.classList.add("is-justified");
        var width = photoGalleryContentWidth(gallery);
        if (width <= 0) return;
        var containers = photoGalleryCardContainers(gallery);
        if (!containers.length) return;
        var visible = [];
        var hidden = [];
        containers.forEach(function (container) {
          containerToGallery.set(container, gallery);
          if (photoContainerLikelyVisible(container)) {
            visible.push(container);
          } else {
            hidden.push(container);
          }
        });
        if (!visible.length) {
          visible = [containers[0]];
          hidden = containers.slice(1);
        }
        layoutPhotoGallery(gallery, visible, width);
        if (visibilityObserver) {
          visible.forEach(function (container) {
            visibilityObserver.unobserve(container);
          });
          hidden.forEach(function (container) {
            visibilityObserver.observe(container);
          });
          return;
        }
        hidden.forEach(function (container) {
          enqueueDeferredLayout(gallery, container, width);
        });
      });
      if (!visibilityObserver && deferredLayouts.length) {
        deferredFrame = window.requestAnimationFrame(flushDeferred);
      }
    }

    function schedule() {
      if (frame) {
        window.cancelAnimationFrame(frame);
      }
      frame = window.requestAnimationFrame(render);
    }

    schedule();
    window.addEventListener("resize", schedule);
  }

  function initPhotoMode() {
    var module = photoModule();
    var toggle = document.querySelector("[data-photo-mode-toggle]");
    if (!module || !toggle) return;

    function hydrateDeferredPhotoTagSelects(root) {
      var scope = root || document;
      scope.querySelectorAll("[data-photo-tag-select-deferred]").forEach(function (picker) {
        if (picker.dataset.photoTagSelectHydrated === "1") return;
        picker.dataset.photoTagSelectHydrated = "1";
        picker.setAttribute("data-tag-select", "");

        var trigger = document.createElement("button");
        trigger.type = "button";
        trigger.className = "tag-select-trigger";
        trigger.setAttribute("data-tag-select-trigger", "");

        var summary = document.createElement("span");
        summary.className = "tag-select-summary";
        summary.setAttribute("data-tag-select-summary", "");
        trigger.appendChild(summary);
        picker.appendChild(trigger);

        var inputs = document.createElement("div");
        inputs.hidden = true;
        inputs.setAttribute("data-tag-select-inputs", "");
        picker.appendChild(inputs);
      });
    }

    function restoredMode() {
      try {
        if (window.sessionStorage.getItem("bearstackPhotoModeAfterBulk") === "edit") {
          window.sessionStorage.removeItem("bearstackPhotoModeAfterBulk");
          return "edit";
        }
      } catch (_) {}
      return "view";
    }

    function setMode(mode, options) {
      options = options || {};
      var edit = mode === "edit";
      var wasEdit = module.dataset.photoMode === "edit";
      module.dataset.photoMode = edit ? "edit" : "view";
      toggle.setAttribute("aria-pressed", edit ? "true" : "false");
      toggle.classList.toggle("is-edit", edit);
      toggle.setAttribute("aria-label", edit ? "Ansicht einschalten" : "Bearbeiten einschalten");
      toggle.title = edit ? "Ansicht einschalten" : "Bearbeiten einschalten";
      if (edit) {
        initPhotoSelection();
        hydrateDeferredPhotoTagSelects(module);
        if (typeof window.initializeTagSelects === "function") {
          window.initializeTagSelects(document);
        }
      }
      if (!edit && wasEdit && options.clearSelection !== false) {
        document.querySelectorAll("[data-photo-bulk-form]").forEach(function (form) {
          if (!form.querySelector('input[name="ids"]:checked')) return;
          setAllPhotoSelected(form, false);
        });
      }
    }

    toggle.addEventListener("click", function () {
      setMode(isPhotoEditMode() ? "view" : "edit", { clearSelection: true });
    });
    setMode(restoredMode(), { clearSelection: false });
  }

  function photoSelectionController(form) {
    var controller = photoSelectionControllers.get(form);
    if (!controller) {
      controller = createSelectionController(form, {
        countSelector: "[data-photo-selection-count]",
        onItemChange: syncPhotoCard,
        stopItemClickPropagation: true
      });
      photoSelectionControllers.set(form, controller);
    }
    return controller;
  }

  function selectedPhotoCount(form) {
    return photoSelectionController(form).selectedCount();
  }

  function syncPhotoCard(checkbox) {
    var item = checkbox.closest("[data-photo-item]");
    if (item) {
      item.classList.toggle("selected", checkbox.checked);
    }
  }

  function setAllPhotoSelected(form, checked, options) {
    photoSelectionController(form).setAll(checked, options);
  }

  function updatePhotoSelection(form) {
    return photoSelectionController(form).sync();
  }

  function applyPhotoSelectionRange(form, checkbox, checked) {
    return photoSelectionController(form).applyRange(checkbox, checked);
  }

  function initPhotoSelection() {
    document.querySelectorAll("[data-photo-bulk-form]").forEach(function (form) {
      var selection = photoSelectionController(form);
      if (form.dataset.photoSelectionInitialized !== "true") {
        form.dataset.photoSelectionInitialized = "true";
        selection.bind();
      }

      var gallery = form.querySelector("[data-photo-gallery]");
      if (gallery && gallery.dataset.photoSelectionInitialized !== "true") {
        gallery.dataset.photoSelectionInitialized = "true";
        gallery.addEventListener("click", function (event) {
          if (!isPhotoEditMode()) return;
          if (event.target.closest("[data-tag-select]")) return;
          var item = event.target.closest("[data-photo-item]");
          if (!item) return;
          var checkbox = item.querySelector('input[name="ids"]');
          if (!checkbox) return;

          event.preventDefault();
          var selected = selectedPhotoCount(form);
          if (event.shiftKey && applyPhotoSelectionRange(form, checkbox, true)) {
            updatePhotoSelection(form);
            return;
          }
          if (event.ctrlKey || event.metaKey) {
            selection.setItemChecked(checkbox, !checkbox.checked);
            selection.setAnchor(checkbox);
            updatePhotoSelection(form);
            return;
          }

          var next = !(checkbox.checked && selected === 1);
          setAllPhotoSelected(form, false, { update: false });
          selection.setItemChecked(checkbox, next);
          selection.setAnchor(checkbox);
          updatePhotoSelection(form);
        });
      }

      selection.items().forEach(syncPhotoCard);
      updatePhotoSelection(form);
    });
  }

  function initLightbox() {
    var gallery = document.querySelector("[data-photo-gallery], [data-photo-map]");
    var dialog = document.querySelector("[data-photo-lightbox]");
    if (!gallery || !dialog) return;

    var items = [];
    var itemsCollected = false;
    var image = dialog.querySelector("[data-photo-image]");
    var video = dialog.querySelector("[data-photo-video]");
    var audioStage = dialog.querySelector("[data-photo-audio-stage]");
    var audio = dialog.querySelector("[data-photo-audio]");
    var stage = dialog.querySelector(".photo-lightbox-stage");
    var title = dialog.querySelector("[data-photo-title]");
    var slideshowButton = dialog.querySelector("[data-photo-slideshow]");
    var infoButton = dialog.querySelector("[data-photo-info-toggle]");
    var infoClose = dialog.querySelector("[data-photo-info-close]");
    var fullscreenButton = dialog.querySelector("[data-photo-fullscreen]");
    var download = dialog.querySelector("[data-photo-download]");
    var mapCard = dialog.querySelector("[data-photo-info-map]");
    var mapCanvas = dialog.querySelector("[data-photo-info-map-canvas]");
    var mapTiles = dialog.querySelector("[data-photo-info-map-tiles]");
    var mapMarker = dialog.querySelector("[data-photo-info-map-marker]");
    var mapLink = dialog.querySelector("[data-photo-info-map-link]");
    var zoomControls = dialog.querySelector("[data-photo-zoom-controls]");
    var zoomIndicator = dialog.querySelector("[data-photo-zoom-indicator]");
    var zoomInButton = dialog.querySelector("[data-photo-zoom-in]");
    var zoomOutButton = dialog.querySelector("[data-photo-zoom-out]");
    var zoomResetButton = dialog.querySelector("[data-photo-zoom-reset]");
    var current = 0;
    var slideshow = 0;
    var pointerStart = null;
    var pinchState = null;
    var activeTouchPointers = new Map();
    var imagePan = { x: 0, y: 0 };
    var imageZoom = 1;
    var imageZoomMin = 1;
    var imageZoomMax = 10;
    var imageZoomStepFactor = 1.2;
    var suppressClickNavigation = false;
    var imageLoadToken = 0;
    var currentImageTarget = "";
    var preloaded = new Set();
    var infoMapTileCache = new Map();
    var infoMapPosition = null;
    var controlsEdgeDistance = 92;
    var slideshowSeconds = Number.parseInt(dialog.dataset.photoSlideshowSeconds || "5", 10);
    var slideshowDelay = Math.max(2, Math.min(60, Number.isFinite(slideshowSeconds) ? slideshowSeconds : 5)) * 1000;
    var preloadAdjacent = dialog.dataset.photoPreloadAdjacent !== "false";
    var fullscreenTarget = document.documentElement;
    var requestFullscreen = fullscreenTarget.requestFullscreen || fullscreenTarget.webkitRequestFullscreen;
    var exitFullscreen = document.exitFullscreen || document.webkitExitFullscreen;
    var lightboxRequestedFullscreen = false;

    function ensureItemsCollected() {
      if (itemsCollected) return items;
      items = collectItems(gallery);
      itemsCollected = true;
      return items;
    }

    function fullscreenElement() {
      return document.fullscreenElement || document.webkitFullscreenElement || null;
    }

    function isLightboxFullscreen() {
      return lightboxRequestedFullscreen && fullscreenElement() === fullscreenTarget;
    }

    function syncFullscreenButton() {
      if (fullscreenElement() !== fullscreenTarget) {
        lightboxRequestedFullscreen = false;
      }
      var active = isLightboxFullscreen() && dialog.open;
      dialog.classList.toggle("is-fullscreen", active);
      if (!fullscreenButton) return;
      fullscreenButton.setAttribute("aria-pressed", active ? "true" : "false");
      fullscreenButton.setAttribute("aria-label", active ? "Vollbild verlassen" : "Vollbild öffnen");
      fullscreenButton.setAttribute("title", active ? "Vollbild verlassen" : "Vollbild öffnen");
    }

    function enterFullscreen() {
      if (!requestFullscreen) return;
      lightboxRequestedFullscreen = true;
      var result = requestFullscreen.call(fullscreenTarget);
      if (result && typeof result.catch === "function") {
        result.catch(function () {
          lightboxRequestedFullscreen = false;
          syncFullscreenButton();
        });
      }
    }

    function leaveFullscreen() {
      if (!exitFullscreen || !isLightboxFullscreen()) return;
      var result = exitFullscreen.call(document);
      if (result && typeof result.catch === "function") {
        result.catch(function () {});
      }
      lightboxRequestedFullscreen = false;
      syncFullscreenButton();
    }

    function toggleFullscreen() {
      if (isLightboxFullscreen()) {
        leaveFullscreen();
      } else {
        enterFullscreen();
      }
    }

    function setText(selector, value) {
      var node = dialog.querySelector(selector);
      if (node) node.textContent = value || "-";
    }

    function setControlsVisible(visible) {
      dialog.classList.toggle("controls-visible", visible);
    }

    function touchControlsDefaultVisible() {
      if (typeof window.matchMedia !== "function") return false;
      return window.matchMedia("(hover: none), (pointer: coarse)").matches;
    }

    function syncControlsForPointer(event) {
      if (!dialog.open || event.pointerType !== "mouse") return;
      var viewportHeight = window.innerHeight || document.documentElement.clientHeight || dialog.clientHeight || 0;
      var nearTopEdge = event.clientY <= controlsEdgeDistance;
      var nearBottomEdge = viewportHeight > 0 && event.clientY >= viewportHeight - controlsEdgeDistance;
      setControlsVisible(nearTopEdge || nearBottomEdge);
    }

    function clampLightboxNumber(value, min, max) {
      return Math.max(min, Math.min(max, value));
    }

    function currentLightboxItem() {
      return items[current] || null;
    }

    function isImageStageActive() {
      var item = currentLightboxItem();
      return Boolean(dialog.open && stage && image && !image.hidden && item && item.type === "image");
    }

    function isZoomedImageStage() {
      return isImageStageActive() && imageZoom > imageZoomMin + 0.001;
    }

    function parsePhotoResolution(resolution) {
      if (!resolution) return { width: 0, height: 0 };
      var match = String(resolution).match(/(\d+(?:[.,]\d+)?)\s*[x×]\s*(\d+(?:[.,]\d+)?)/i);
      if (!match) return { width: 0, height: 0 };
      return {
        width: positiveNumber(String(match[1]).replace(",", ".")),
        height: positiveNumber(String(match[2]).replace(",", "."))
      };
    }

    function currentImageRenderSize(item) {
      if (!stage) return { width: 1, height: 1 };
      var bounds = stage.getBoundingClientRect();
      var stageWidth = Math.max(1, bounds.width || stage.clientWidth || 1);
      var stageHeight = Math.max(1, bounds.height || stage.clientHeight || 1);
      var naturalWidth = positiveNumber(image && image.naturalWidth);
      var naturalHeight = positiveNumber(image && image.naturalHeight);
      var sourceWidth = naturalWidth || positiveNumber(item && item.width);
      var sourceHeight = naturalHeight || positiveNumber(item && item.height);
      if ((!sourceWidth || !sourceHeight) && item) {
        var parsed = parsePhotoResolution(item.resolution);
        sourceWidth = sourceWidth || parsed.width;
        sourceHeight = sourceHeight || parsed.height;
      }
      if (!sourceWidth || !sourceHeight) {
        return { width: stageWidth, height: stageHeight };
      }
      var stageRatio = stageWidth / stageHeight;
      var sourceRatio = sourceWidth / sourceHeight;
      if (sourceRatio >= stageRatio) {
        return { width: stageWidth, height: Math.max(1, stageWidth / Math.max(0.001, sourceRatio)) };
      }
      return { width: Math.max(1, stageHeight * sourceRatio), height: stageHeight };
    }

    function imageMaxPan(item) {
      var rendered = currentImageRenderSize(item);
      return {
        x: Math.max(0, rendered.width * Math.max(0, imageZoom - 1) / 2),
        y: Math.max(0, rendered.height * Math.max(0, imageZoom - 1) / 2)
      };
    }

    function syncLightboxZoomUI() {
      var active = isImageStageActive();
      var percent = Math.round(imageZoom * 100);
      if (zoomControls) {
        zoomControls.hidden = !active;
      }
      if (zoomIndicator) {
        zoomIndicator.textContent = percent + "%";
        zoomIndicator.hidden = !active || Math.abs(imageZoom - 1) <= 0.001;
      }
      if (zoomOutButton) {
        zoomOutButton.disabled = !active || imageZoom <= imageZoomMin + 0.001;
      }
      if (zoomInButton) {
        zoomInButton.disabled = !active || imageZoom >= imageZoomMax - 0.001;
      }
      if (zoomResetButton) {
        zoomResetButton.disabled = !active || Math.abs(imageZoom - 1) <= 0.001;
      }
    }

    function applyImageTransform(dragging) {
      var active = isImageStageActive();
      var item = currentLightboxItem();
      if (!stage || !image || !active || !item) {
        if (stage) {
          stage.classList.remove("is-zoomable");
          stage.classList.remove("is-zoomed");
          stage.classList.remove("is-dragging");
        }
        if (image) {
          image.style.transform = "";
        }
        syncLightboxZoomUI();
        return;
      }
      var maxPan = imageMaxPan(item);
      imagePan.x = clampLightboxNumber(imagePan.x, -maxPan.x, maxPan.x);
      imagePan.y = clampLightboxNumber(imagePan.y, -maxPan.y, maxPan.y);
      if (imageZoom <= imageZoomMin + 0.001) {
        imageZoom = 1;
        imagePan.x = 0;
        imagePan.y = 0;
      }
      var transform = imageZoom <= 1.001 ? "" : ("translate(" + imagePan.x.toFixed(2) + "px, " + imagePan.y.toFixed(2) + "px) scale(" + imageZoom.toFixed(4) + ")");
      image.style.transform = transform;
      stage.classList.toggle("is-zoomable", active);
      stage.classList.toggle("is-zoomed", imageZoom > 1.001);
      stage.classList.toggle("is-dragging", Boolean(dragging && imageZoom > 1.001));
      syncLightboxZoomUI();
    }

    function setImageZoom(nextZoom, options) {
      options = options || {};
      if (!stage || !image) return;
      if (!isImageStageActive()) {
        imageZoom = 1;
        imagePan.x = 0;
        imagePan.y = 0;
        applyImageTransform(false);
        return;
      }
      var clampedZoom = clampLightboxNumber(Number(nextZoom) || 1, imageZoomMin, imageZoomMax);
      if (!Number.isFinite(clampedZoom)) return;
      var previousZoom = imageZoom;
      if (Math.abs(clampedZoom - previousZoom) > 0.0001) {
        if (options.focusPoint && stage) {
          var bounds = stage.getBoundingClientRect();
          var relativeX = options.focusPoint.x - bounds.width / 2;
          var relativeY = options.focusPoint.y - bounds.height / 2;
          var factor = clampedZoom / Math.max(0.0001, previousZoom);
          imagePan.x = imagePan.x * factor + relativeX * (1 - factor);
          imagePan.y = imagePan.y * factor + relativeY * (1 - factor);
        } else if (previousZoom > 0.0001 && clampedZoom > imageZoomMin + 0.001) {
          var centerFactor = clampedZoom / previousZoom;
          imagePan.x *= centerFactor;
          imagePan.y *= centerFactor;
        }
      }
      imageZoom = clampedZoom;
      applyImageTransform(options.dragging);
    }

    function changeImageZoom(direction, focusPoint) {
      var factor = direction > 0 ? imageZoomStepFactor : (1 / imageZoomStepFactor);
      setImageZoom(imageZoom * factor, { focusPoint: focusPoint || null });
    }

    function releaseGestureTracking() {
      pointerStart = null;
      pinchState = null;
      activeTouchPointers.clear();
      if (stage) stage.classList.remove("is-dragging");
    }

    function resetImageZoom() {
      imageZoom = 1;
      imagePan.x = 0;
      imagePan.y = 0;
      releaseGestureTracking();
      applyImageTransform(false);
    }

    function stagePointFromClient(clientX, clientY) {
      var bounds = stage.getBoundingClientRect();
      return {
        x: clientX - bounds.left,
        y: clientY - bounds.top
      };
    }

    function touchPointerCenter() {
      var points = Array.from(activeTouchPointers.values());
      if (points.length < 2) return null;
      return {
        x: (points[0].x + points[1].x) / 2,
        y: (points[0].y + points[1].y) / 2
      };
    }

    function touchPointerDistance() {
      var points = Array.from(activeTouchPointers.values());
      if (points.length < 2) return 0;
      return Math.hypot(points[0].x - points[1].x, points[0].y - points[1].y);
    }

    function shouldToggleControlsForTouch(event) {
      if (!event || event.pointerType === "mouse") return false;
      var target = event.target;
      if (!target || typeof target.closest !== "function") return false;
      if (target.closest("button, a, input, select, textarea")) return false;
      var mediaTarget = target.closest("video, audio");
      if (mediaTarget && dialog.classList.contains("controls-visible")) return false;
      var bounds = stage.getBoundingClientRect();
      if (!bounds.width || !bounds.height) return false;
      var x = event.clientX - bounds.left;
      if (x <= bounds.width * 0.25 || x >= bounds.width * 0.75) return false;
      return true;
    }

    function navigateByStageClick(event) {
      if (!dialog.open || suppressClickNavigation || event.defaultPrevented) return;
      var target = event.target;
      if (!target || typeof target.closest !== "function") return;
      if (target.closest("button, a, input, select, textarea")) return;
      var mediaTarget = target.closest("video, audio");
      if (mediaTarget && dialog.classList.contains("controls-visible")) return;
      if (isZoomedImageStage()) return;
      if (event.pointerType && event.pointerType !== "mouse") {
        var touchBounds = stage.getBoundingClientRect();
        var touchX = event.clientX - touchBounds.left;
        if (touchX > touchBounds.width * 0.25 && touchX < touchBounds.width * 0.75) {
          return;
        }
      }
      var bounds = stage.getBoundingClientRect();
      if (!bounds.width || !bounds.height) return;
      var x = event.clientX - bounds.left;
      if (x <= bounds.width * 0.25) {
        show(current - 1);
      } else if (x >= bounds.width * 0.75) {
        show(current + 1);
      }
    }

    function setInfoPanel(open) {
      dialog.classList.toggle("info-open", open);
      if (infoButton) {
        infoButton.setAttribute("aria-pressed", open ? "true" : "false");
        infoButton.setAttribute("aria-label", open ? "Informationen ausblenden" : "Informationen anzeigen");
      }
      if (open) {
        window.requestAnimationFrame(renderInfoMap);
        window.setTimeout(renderInfoMap, 80);
      }
    }

    function isKeyboardControlTarget(target) {
      if (!target || typeof target.closest !== "function") return false;
      return Boolean(target.closest("button, a, input, select, textarea, video, audio, [contenteditable='true']"));
    }

    function eventIsSpace(event) {
      return event.key === " " || event.key === "Spacebar" || event.code === "Space";
    }

    function toggleMediaElementPlayback(element, hidden) {
      if (!element || hidden || !element.src) return false;
      if (element.paused || element.ended) {
        var playResult = element.play();
        if (playResult && typeof playResult.catch === "function") {
          playResult.catch(function () {});
        }
      } else {
        element.pause();
      }
      return true;
    }

    function toggleCurrentPlayback() {
      if (!dialog.open) return false;
      if (toggleMediaElementPlayback(video, video.hidden)) return true;
      return toggleMediaElementPlayback(audio, !audioStage || audioStage.hidden);
    }

    function setDownload(item) {
      if (!download) return;
      var original = item.original || item.src || "";
      download.hidden = !original;
      download.href = original || "#";
      if (item.title) {
        download.setAttribute("download", item.title);
      }
    }

    function closeInfoPanel() {
      setInfoPanel(false);
    }

    function swapImageWhenLoaded(src, token, item) {
      if (!src) return;
      var loader = new Image();
      loader.decoding = "async";
      loader.onload = function () {
        if (token !== imageLoadToken || items[current] !== item) return;
        image.src = src;
      };
      loader.src = src;
      if (loader.complete) {
        loader.onload();
      }
    }

    function loadPhotoImage(item) {
      var target = bestPhotoDisplaySrc(item);
      var placeholder = item.thumb || target || item.preview || item.largePreview || item.original || item.src;
      var token = imageLoadToken + 1;
      imageLoadToken = token;
      currentImageTarget = target || placeholder || "";
      if (placeholder) {
        image.src = placeholder;
      } else {
        image.removeAttribute("src");
      }
      image.hidden = false;
      applyImageTransform(false);
      if (target && target !== placeholder) {
        swapImageWhenLoaded(target, token, item);
      }
    }

    function upgradeCurrentImage() {
      if (!dialog.open || !items.length || image.hidden) return;
      var item = items[current];
      if (!item || item.type !== "image") return;
      var target = bestPhotoDisplaySrc(item);
      if (!target || target === currentImageTarget) return;
      var token = imageLoadToken + 1;
      imageLoadToken = token;
      currentImageTarget = target;
      swapImageWhenLoaded(target, token, item);
    }

    function resetMedia() {
      imageLoadToken += 1;
      currentImageTarget = "";
      resetImageZoom();
      image.hidden = true;
      image.removeAttribute("src");
      video.hidden = true;
      video.pause();
      try {
        video.currentTime = 0;
      } catch (_) {}
      video.removeAttribute("src");
      video.load();
      if (audioStage) audioStage.hidden = true;
      if (audio) {
        audio.pause();
        try {
          audio.currentTime = 0;
        } catch (_) {}
        audio.removeAttribute("src");
        audio.load();
      }
      applyImageTransform(false);
    }

    function renderCurrentItem(item) {
      if (!item || items[current] !== item) return;
      resetMedia();
      if (item.type === "video") {
        if (item.src || item.original) {
          video.src = item.src || item.original;
          video.hidden = false;
        }
      } else if (item.type === "audio") {
        if (audio && audioStage && (item.src || item.original)) {
          audio.src = item.src || item.original;
          audioStage.hidden = false;
        }
      } else {
        loadPhotoImage(item);
      }
      applyImageTransform(false);
      setText("[data-photo-info-name]", item.title);
      setText("[data-photo-info-date]", item.date);
      setText("[data-photo-info-camera]", item.camera);
      setText("[data-photo-info-lens]", item.lens);
      setText("[data-photo-info-rating]", formatPhotoRating(item.rating));
      setText("[data-photo-info-size]", item.size);
      setText("[data-photo-info-resolution]", item.resolution);
      setText("[data-photo-info-coords]", item.coords);
      setDownload(item);
      setMap(item);
    }

    function show(index) {
      if (!items.length) return;
      current = (index + items.length) % items.length;
      var item = items[current];
      if (title) title.textContent = item.title;
      renderCurrentItem(item);
      ensurePhotoItemDetails(item).then(function (updated) {
        if (items[current] !== item) return;
        if (title) title.textContent = updated.title;
        renderCurrentItem(updated);
        preloadNeighbors();
      });
      if (item.detailsLoaded) {
        preloadNeighbors();
      }
    }

    function renderInfoMap() {
      if (!infoMapPosition || !mapCard || !mapCanvas || !mapTiles || !mapMarker || mapCard.hidden) return;
      if (!photoMap.initialState || !photoMap.renderTiles || !photoMap.screenPoint) return;
      var size = {
        width: Math.max(1, Math.round(mapCanvas.clientWidth || mapCanvas.offsetWidth || 280)),
        height: Math.max(1, Math.round(mapCanvas.clientHeight || mapCanvas.offsetHeight || 210))
      };
      var state = photoMap.initialState([infoMapPosition], size.width, size.height);
      photoMap.renderTiles(mapTiles, state, size, infoMapTileCache);
      var point = photoMap.screenPoint(infoMapPosition, state, size);
      mapMarker.hidden = false;
      mapMarker.style.left = point.x.toFixed(2) + "px";
      mapMarker.style.top = point.y.toFixed(2) + "px";
    }

    function setMap(item) {
      if (!mapCard || !mapCanvas || !mapTiles || !mapMarker || !mapLink) return;
      var lat = Number.parseFloat(item.lat || "");
      var lon = Number.parseFloat(item.lon || "");
      var hasPosition = Number.isFinite(lat) && Number.isFinite(lon);
      mapCard.hidden = !hasPosition;
      if (!hasPosition) {
        infoMapPosition = null;
        mapTiles.replaceChildren();
        mapMarker.hidden = true;
        mapLink.href = "#";
        return;
      }
      infoMapPosition = { lat: lat, lon: lon };
      mapLink.href = photoMap.externalURL ? photoMap.externalURL(lat, lon, 17) : "#";
      renderInfoMap();
    }

    function preloadItem(index) {
      if (!items.length) return;
      var normalized = (index + items.length) % items.length;
      var item = items[normalized];
      if (!item) return;
      ensurePhotoItemDetails(item).then(function () {
        var src = bestPhotoDisplaySrc(item);
        if (item.type !== "image" || !src || preloaded.has(src)) return;
        preloaded.add(src);
        if (preloaded.size > 8) {
          preloaded.delete(preloaded.values().next().value);
        }
        var preload = new Image();
        preload.decoding = "async";
        preload.src = src;
      });
    }

    function preloadNeighbors() {
      if (!preloadAdjacent) return;
      window.setTimeout(function () {
        preloadItem(current + 1);
        if (items.length > 2) {
          preloadItem(current - 1);
        }
      }, 0);
    }

    function open(index) {
      ensureItemsCollected();
      if (!items.length) return;
      show(index);
      setInfoPanel(false);
      setControlsVisible(touchControlsDefaultVisible());
      if (typeof dialog.showModal === "function") {
        dialog.showModal();
      } else {
        dialog.setAttribute("open", "open");
      }
      dialog.focus({ preventScroll: true });
      window.requestAnimationFrame(function () {
        applyImageTransform(false);
      });
    }

    function stopSlideshow() {
      if (slideshow) window.clearInterval(slideshow);
      slideshow = 0;
      slideshowButton.textContent = "Start";
    }

    function closeLightbox() {
      stopSlideshow();
      if (typeof dialog.close === "function") {
        dialog.close();
      } else {
        dialog.removeAttribute("open");
      }
    }

    gallery.addEventListener("click", function (event) {
      if (isPhotoEditMode()) return;
      if (event.target.closest("[data-tag-select], .photo-select-input")) return;
      var itemNode = event.target.closest("[data-photo-item]");
      if (!itemNode) return;
      ensureItemsCollected();
      var index = items.findIndex(function (item) { return item.node === itemNode; });
      if (index >= 0) open(index);
    });

    dialog.querySelector("[data-photo-close]").addEventListener("click", function () {
      closeLightbox();
    });
    dialog.querySelector("[data-photo-prev]").addEventListener("click", function () { show(current - 1); });
    dialog.querySelector("[data-photo-next]").addEventListener("click", function () { show(current + 1); });
    if (infoButton) {
      infoButton.addEventListener("click", function () {
        setInfoPanel(!dialog.classList.contains("info-open"));
      });
    }
    if (infoClose) {
      infoClose.addEventListener("click", function (event) {
        event.preventDefault();
        event.stopPropagation();
        closeInfoPanel();
      });
    }
    if (fullscreenButton) {
      if (requestFullscreen && exitFullscreen) {
        fullscreenButton.addEventListener("click", toggleFullscreen);
      } else {
        fullscreenButton.hidden = true;
      }
    }
    slideshowButton.addEventListener("click", function () {
      if (slideshow) {
        stopSlideshow();
        return;
      }
      slideshow = window.setInterval(function () { show(current + 1); }, slideshowDelay);
      slideshowButton.textContent = "Stop";
    });
    if (zoomInButton) {
      zoomInButton.addEventListener("click", function (event) {
        event.preventDefault();
        if (!isImageStageActive()) return;
        changeImageZoom(1);
      });
    }
    if (zoomOutButton) {
      zoomOutButton.addEventListener("click", function (event) {
        event.preventDefault();
        if (!isImageStageActive()) return;
        changeImageZoom(-1);
      });
    }
    if (zoomResetButton) {
      zoomResetButton.addEventListener("click", function (event) {
        event.preventDefault();
        if (!isImageStageActive()) return;
        resetImageZoom();
      });
    }
    if (image) {
      image.addEventListener("load", function () {
        applyImageTransform(false);
      });
      image.addEventListener("error", function () {
        applyImageTransform(false);
      });
    }
    dialog.addEventListener("close", function () {
      stopSlideshow();
      setInfoPanel(false);
      setControlsVisible(false);
      leaveFullscreen();
      resetMedia();
    });
    function syncViewportMedia() {
      syncFullscreenButton();
      upgradeCurrentImage();
      renderInfoMap();
      applyImageTransform(false);
    }

    document.addEventListener("fullscreenchange", syncViewportMedia);
    document.addEventListener("webkitfullscreenchange", syncViewportMedia);
    window.addEventListener("resize", syncViewportMedia);

    function suppressClickNavigationOnce() {
      suppressClickNavigation = true;
      window.setTimeout(function () {
        suppressClickNavigation = false;
      }, 0);
    }

    function clearTouchPointer(pointerId) {
      activeTouchPointers.delete(pointerId);
      if (activeTouchPointers.size < 2) {
        pinchState = null;
      }
    }

    dialog.addEventListener("pointermove", function (event) {
      syncControlsForPointer(event);
      if (!dialog.open) return;
      if (activeTouchPointers.has(event.pointerId)) {
        activeTouchPointers.set(event.pointerId, { x: event.clientX, y: event.clientY });
      }
      if (pinchState && activeTouchPointers.size >= 2 && isImageStageActive()) {
        var distance = touchPointerDistance();
        if (distance > 0.1 && pinchState.distance > 0.1) {
          var center = touchPointerCenter();
          if (center) {
            setImageZoom(pinchState.zoom * (distance / pinchState.distance), {
              focusPoint: stagePointFromClient(center.x, center.y),
              dragging: true
            });
            setControlsVisible(true);
            suppressClickNavigation = true;
            event.preventDefault();
            return;
          }
        }
      }
      if (!pointerStart || pointerStart.id !== event.pointerId || !pointerStart.panningImage || !isImageStageActive()) return;
      var deltaX = event.clientX - pointerStart.x;
      var deltaY = event.clientY - pointerStart.y;
      if (Math.abs(deltaX) > 2 || Math.abs(deltaY) > 2) {
        pointerStart.moved = true;
      }
      imagePan.x = pointerStart.panX + deltaX;
      imagePan.y = pointerStart.panY + deltaY;
      applyImageTransform(true);
      event.preventDefault();
    });

    dialog.addEventListener("mouseleave", function (event) {
      if (event.pointerType === "mouse") setControlsVisible(false);
    });
    dialog.addEventListener("pointerdown", function (event) {
      if (!dialog.open) return;
      var insideStage = Boolean(stage && stage.contains(event.target));
      var interactiveTarget = event.target.closest("video, audio, button, a, input, select, textarea");
      if (insideStage && !interactiveTarget && event.pointerType !== "mouse") {
        activeTouchPointers.set(event.pointerId, { x: event.clientX, y: event.clientY });
        if (activeTouchPointers.size >= 2 && isImageStageActive()) {
          var startDistance = touchPointerDistance();
          if (startDistance > 0.1) {
            pinchState = {
              distance: startDistance,
              zoom: imageZoom
            };
            pointerStart = null;
            suppressClickNavigation = true;
          }
        }
      }
      if (!event.isPrimary) return;
      pointerStart = {
        id: event.pointerId,
        x: event.clientX,
        y: event.clientY,
        panX: imagePan.x,
        panY: imagePan.y,
        moved: false,
        panningImage: insideStage && !interactiveTarget && isZoomedImageStage()
      };
      if (pointerStart.panningImage && stage && typeof stage.setPointerCapture === "function") {
        try {
          stage.setPointerCapture(event.pointerId);
        } catch (_) {}
      }
    });

    function handlePointerRelease(event) {
      var wasPinching = Boolean(pinchState);
      var hadTouchPointer = activeTouchPointers.has(event.pointerId);
      clearTouchPointer(event.pointerId);
      var pinchJustEnded = hadTouchPointer && wasPinching && !pinchState;
      if (stage && typeof stage.releasePointerCapture === "function") {
        try {
          stage.releasePointerCapture(event.pointerId);
        } catch (_) {}
      }
      if (!pointerStart || pointerStart.id !== event.pointerId) {
        if (pinchJustEnded) {
          suppressClickNavigationOnce();
          applyImageTransform(false);
        }
        return;
      }
      var deltaX = event.clientX - pointerStart.x;
      var deltaY = event.clientY - pointerStart.y;
      var absX = Math.abs(deltaX);
      var absY = Math.abs(deltaY);
      var pointerWasPanning = pointerStart.panningImage;
      var moved = pointerStart.moved || absX > 3 || absY > 3;
      pointerStart = null;
      applyImageTransform(false);

      if (pointerWasPanning || isZoomedImageStage()) {
        if (moved || pinchJustEnded) {
          suppressClickNavigationOnce();
        } else if (shouldToggleControlsForTouch(event)) {
          setControlsVisible(!dialog.classList.contains("controls-visible"));
        }
        return;
      }

      if (pinchJustEnded) {
        suppressClickNavigationOnce();
        return;
      }
      if (absX < 48 || absX < absY * 1.2) {
        if (shouldToggleControlsForTouch(event)) {
          setControlsVisible(!dialog.classList.contains("controls-visible"));
        }
        return;
      }
      suppressClickNavigationOnce();
      show(deltaX < 0 ? current + 1 : current - 1);
    }

    dialog.addEventListener("pointerup", handlePointerRelease);
    dialog.addEventListener("pointercancel", handlePointerRelease);
    if (stage) {
      stage.addEventListener("click", navigateByStageClick);
      stage.addEventListener("wheel", function (event) {
        if (!isImageStageActive() || !event.deltaY) return;
        event.preventDefault();
        changeImageZoom(event.deltaY < 0 ? 1 : -1, stagePointFromClient(event.clientX, event.clientY));
      }, { passive: false });
    }
    syncLightboxZoomUI();
    document.addEventListener("keydown", function (event) {
      if (!dialog.open) return;
      if (eventIsSpace(event) && !event.repeat && !isKeyboardControlTarget(event.target) && toggleCurrentPlayback()) {
        event.preventDefault();
        return;
      }
      if (isImageStageActive()) {
        if (event.key === "+" || event.key === "=") {
          event.preventDefault();
          changeImageZoom(1);
          return;
        }
        if (event.key === "-") {
          event.preventDefault();
          changeImageZoom(-1);
          return;
        }
        if (event.key === "0") {
          event.preventDefault();
          resetImageZoom();
          return;
        }
      }
      if (event.key === "ArrowLeft" && !isZoomedImageStage()) show(current - 1);
      if (event.key === "ArrowRight" && !isZoomedImageStage()) show(current + 1);
      if (event.key && event.key.toLowerCase() === "i") setInfoPanel(!dialog.classList.contains("info-open"));
      if (event.key === "Escape") {
        event.preventDefault();
        closeLightbox();
      }
    });
  }

  function focusPhotoFilterSearchInput(menu) {
    if (!menu) return;
    var input = menu.querySelector("form.photo-filters input[type='search']");
    if (!input || input.disabled) return;
    var applyFocus = function () {
      if (input.disabled) return;
      if (typeof input.focus !== "function") return;
      try {
        input.focus({ preventScroll: true });
      } catch (_) {
        input.focus();
      }
      if (typeof input.select === "function") {
        input.select();
      }
    };
    applyFocus();
    var schedule = window.requestAnimationFrame || function (callback) {
      return window.setTimeout(callback, 16);
    };
    schedule(applyFocus);
    window.setTimeout(applyFocus, 0);
  }

  function initPhotoFilterMenuFocus() {
    document.querySelectorAll("[data-photo-filter-menu]").forEach(function (menu) {
      menu.addEventListener("toggle", function () {
        if (!menu.open) return;
        focusPhotoFilterSearchInput(menu);
      });
    });
  }

  window.BearStack = window.BearStack || {};
  window.BearStack.photos = Object.assign(window.BearStack.photos || {}, {
    applyPhotoItemDetails: applyPhotoItemDetails,
    bestPhotoDisplaySrc: bestPhotoDisplaySrc,
    collectItems: collectItems,
    formatPhotoRating: formatPhotoRating,
    photoItemFromNode: photoItemFromNode
  });

  document.addEventListener("DOMContentLoaded", function () {
    initPhotoMode();
    initPhotoJustifiedLayout();
    if (photoThumbnails.init) photoThumbnails.init();
    initLightbox();
    if (photoMap.init) photoMap.init();
    initPhotoFilterMenuFocus();
  });
})();
