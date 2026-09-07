(function () {
  var photoMedia = window.BearStack.photos;
  var applyPhotoItemDetails = photoMedia.applyPhotoItemDetails;
  var positiveNumber = photoMedia.positiveNumber;
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

  function initPhotoScrollRestoration() {
    var module = photoModule();
    if (!module || module.classList.contains("photo-map-module")) return;
    var storageKey = "bearstackPhotoNavigation:v1";
    var historyKey = "bearstackPhotoScroll";
    var currentURL = photoURL(window.location.href);
    var positions = [];
    var pendingURL = "";
    var departure = null;
    var interacted = false;

    function photoURL(value) {
      try {
        var url = new URL(value, window.location.href);
        if (url.origin !== window.location.origin || url.pathname !== "/photos" || url.hash || url.searchParams.get("view") === "map") return null;
        url.searchParams.sort();
        return url;
      } catch (_) {
        return null;
      }
    }

    if (!currentURL) return;
    var currentPath = currentURL.searchParams.get("path") || "";
    var currentAddress = currentURL.pathname + currentURL.search;
    var historyEntry = window.history.state && window.history.state[historyKey];
    if (!historyEntry || historyEntry.url !== currentAddress || typeof historyEntry.id !== "string") {
      historyEntry = { url: currentAddress, id: Date.now().toString(36) + Math.random().toString(36).slice(2) };
      try {
        var state = Object.assign({}, window.history.state || {});
        state[historyKey] = historyEntry;
        window.history.replaceState(state, "");
      } catch (_) {}
    }

    function validPosition(position) {
      return position && typeof position.id === "string" && typeof position.url === "string" && photoURL(position.url) &&
        Number.isFinite(position.x) && Number.isFinite(position.y) && position.x >= 0 && position.y >= 0;
    }

    function readNavigation() {
      try {
        var stored = JSON.parse(window.sessionStorage.getItem(storageKey) || "null");
        if (stored && Array.isArray(stored.positions)) {
          return {
            positions: stored.positions.slice(-50).filter(validPosition),
            pendingURL: typeof stored.pendingURL === "string" ? stored.pendingURL : ""
          };
        }
      } catch (_) {}
      return null;
    }

    var stored = readNavigation();
    if (stored) {
      positions = stored.positions;
      pendingURL = stored.pendingURL;
    }

    function persist(nextURL) {
      try {
        window.sessionStorage.setItem(storageKey, JSON.stringify({ positions: positions, pendingURL: nextURL || "" }));
      } catch (_) {}
    }

    function remember(position) {
      // A page revived from the browser cache may hold an older copy than the
      // child pages visited since it was suspended.
      var latest = readNavigation();
      if (latest) positions = latest.positions;
      positions = positions.filter(function (saved) { return saved.id !== historyEntry.id; });
      positions.push(position);
      positions = positions.slice(-50);
    }

    function capture(link) {
      return {
        id: historyEntry.id,
        url: currentAddress,
        x: window.scrollX,
        y: window.scrollY,
        anchor: link ? link.getAttribute("href") : "",
        offset: link ? link.getBoundingClientRect().top : 0
      };
    }

    // Restore the visited parent URL as well, retaining its sort, filters and
    // page. The bounded tab history avoids work proportional to the photo tree.
    document.querySelectorAll('.folder-breadcrumb[aria-label="Fotopfad"] a').forEach(function (link) {
      var destination = photoURL(link.href);
      if (!destination) return;
      var path = destination.searchParams.get("path") || "";
      if (path === currentPath) return;
      for (var i = positions.length - 1; i >= 0; i -= 1) {
        var savedURL = photoURL(positions[i].url);
        if ((savedURL.searchParams.get("path") || "") === path) {
          link.href = positions[i].url;
          break;
        }
      }
    });

    document.addEventListener("click", function (event) {
      if (event.defaultPrevented || event.button !== 0 || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return;
      var link = event.target.closest("a[href]");
      if (!link || link.hasAttribute("download") || (link.target && link.target !== "_self")) return;
      var breadcrumb = link.closest('.folder-breadcrumb[aria-label="Fotopfad"]');
      var folder = link.matches(".photo-folder-link");
      var destination = photoURL(link.href);
      if (!destination || (!folder && !breadcrumb)) return;
      departure = capture(folder ? link : null);
      remember(departure);
      pendingURL = breadcrumb ? destination.pathname + destination.search : "";
      persist(pendingURL);
    });

    window.addEventListener("pagehide", function () {
      remember(departure || capture(null));
      persist(pendingURL);
    });

    function restore(position) {
      if (!validPosition(position) || position.url !== currentAddress) return;
      function apply() {
        if (interacted) return;
        var y = position.y;
        if (typeof position.anchor === "string" && position.anchor && Number.isFinite(position.offset)) {
          var link = module.querySelector('.photo-folder-link[href="' + CSS.escape(position.anchor) + '"]');
          if (link) y = window.scrollY + link.getBoundingClientRect().top - position.offset;
        }
        window.scrollTo({ left: position.x, top: Math.max(0, y), behavior: "instant" });
      }
      // Wait for the gallery's initial layout and the browser's own history
      // restoration. Thumbnail boxes already reserve their final dimensions.
      window.requestAnimationFrame(function () {
        apply();
        window.requestAnimationFrame(apply);
      });
    }

    var initialPosition = positions.find(function (saved) { return saved.id === historyEntry.id; });
    if (pendingURL === currentAddress) {
      initialPosition = positions.slice().reverse().find(function (saved) { return saved.url === currentAddress; }) || initialPosition;
    }
    pendingURL = "";
    persist("");
    restore(initialPosition);
    window.addEventListener("pageshow", function (event) {
      departure = null;
      pendingURL = "";
      if (event.persisted) {
        interacted = false;
        var latest = readNavigation();
        restore(latest && latest.positions.find(function (saved) { return saved.id === historyEntry.id; }));
      } else {
        restore(initialPosition);
      }
    });
    ["wheel", "touchstart", "pointerdown", "keydown"].forEach(function (name) {
      window.addEventListener(name, function () { interacted = true; }, { passive: true });
    });
  }

  function isPhotoEditMode() {
    var module = photoModule();
    return module && module.dataset.photoMode === "edit";
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

  document.addEventListener("DOMContentLoaded", function () {
    initPhotoMode();
    initPhotoJustifiedLayout();
    if (photoThumbnails.init) photoThumbnails.init();
    photoMedia.lightbox.init({
      isEditMode: isPhotoEditMode,
      ensureItemDetails: ensurePhotoItemDetails,
      map: photoMap
    });
    if (photoMap.init) photoMap.init();
    initPhotoFilterMenuFocus();
    initPhotoScrollRestoration();
  });
})();
