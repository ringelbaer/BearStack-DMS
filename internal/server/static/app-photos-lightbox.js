(function () {
  var photoMedia = window.BearStack.photos;
  var bestPhotoDisplaySrc = photoMedia.bestPhotoDisplaySrc;
  var collectItems = photoMedia.collectItems;
  var formatPhotoRating = photoMedia.formatPhotoRating;
  var positiveNumber = photoMedia.positiveNumber;

  // The host supplies edit state, batched metadata loading and optional map helpers.
  function initLightbox(options) {
    var isPhotoEditMode = options.isEditMode;
    var ensurePhotoItemDetails = options.ensureItemDetails;
    var photoMap = options.map || {};
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
    var prevButton = dialog.querySelector("[data-photo-prev]");
    var nextButton = dialog.querySelector("[data-photo-next]");
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
      setText("[data-photo-info-date]", item.dateTime || item.date);
      setText("[data-photo-info-camera]", item.camera);
      setText("[data-photo-info-lens]", item.lens);
      setText("[data-photo-info-rating]", formatPhotoRating(item.rating));
      setText("[data-photo-info-size]", item.size);
      setText("[data-photo-info-resolution]", item.resolution);
      setText("[data-photo-info-coords]", item.coords);
      var people = dialog.querySelector("[data-photo-info-people]");
      if (people) {
        people.replaceChildren();
        (item.automaticFaces || []).forEach(function (face) {
          var link = document.createElement("a");
          link.href = "/photos/people/" + encodeURIComponent(face.person_id);
          link.textContent = face.name || "Unbenannt";
          people.appendChild(link); people.appendChild(document.createTextNode(" "));
        });
        if (!people.childNodes.length) people.textContent = "–";
      }
      setDownload(item);
      setMap(item);
    }

    function show(index) {
      if (index < 0 || index >= items.length) return;
      current = index;
      prevButton.disabled = current === 0;
      nextButton.disabled = current === items.length - 1;
      slideshowButton.disabled = nextButton.disabled;
      if (nextButton.disabled) stopSlideshow();
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
      if (index < 0 || index >= items.length) return;
      var item = items[index];
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
        preloadItem(current - 1);
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
    prevButton.addEventListener("click", function () { show(current - 1); });
    nextButton.addEventListener("click", function () { show(current + 1); });
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

  photoMedia.lightbox = Object.assign(photoMedia.lightbox || {}, {
    init: initLightbox
  });
})();
