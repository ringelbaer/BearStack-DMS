(function () {
  var photoMedia = window.BearStack.photos;

  function initFrame() {
    var frame = document.querySelector("[data-photo-frame]");
    if (!frame || frame.dataset.photoFrameReady === "true") return;
    frame.dataset.photoFrameReady = "true";
    var image = frame.querySelector("[data-photo-frame-image]");
    var video = frame.querySelector("[data-photo-frame-video]");
    var bar = frame.querySelector("[data-photo-frame-bar]");
    var barClose = frame.querySelector("[data-photo-frame-bar-close]");
    var title = frame.querySelector("[data-photo-frame-title]");
    var count = frame.querySelector("[data-photo-frame-count]");
    var items = [];
    var index = 0;
    var page = 1;
    var pageSize = 200;
    var loading = null;
    var loadController = null;
    var imageLoader = null;
    var currentItem = null;
    var waitingForItems = false;
    var timer = null;
    var suspended = false;
    var shown = 0;
    var total = 0;
    var exhausted = false;
    var frameSeconds = Number.parseInt(frame.dataset.photoFrameSeconds || "8", 10);
    var frameDelay = Math.max(3, Math.min(300, Number.isFinite(frameSeconds) ? frameSeconds : 8)) * 1000;

    if (bar && barClose) {
      barClose.addEventListener("click", function () {
        bar.hidden = true;
      });
    }

    function frameItemsURL() {
      var url = new URL(frame.dataset.photoFrameItemsUrl || "/photos/frame/items", window.location.href);
      var current = new URL(window.location.href);
      current.searchParams.forEach(function (value, key) {
        if (key !== "page" && key !== "page_size") {
          url.searchParams.set(key, value);
        }
      });
      url.searchParams.set("page", String(page));
      url.searchParams.set("page_size", String(pageSize));
      return url.toString();
    }

    function active() {
      return !document.hidden && !suspended;
    }

    function loadFrameItems() {
      if (loading) return loading;
      // At most two pages, including the consumed prefix of the current page.
      if (!active() || exhausted || items.length > pageSize || typeof window.fetch !== "function") return Promise.resolve();
      loadController = new AbortController();
      loading = window.fetch(frameItemsURL(), {
        credentials: "same-origin",
        headers: { Accept: "application/json" },
        signal: loadController.signal
      }).then(function (response) {
        if (!response.ok) throw new Error("Fotoframe konnte keine Medien laden");
        return response.json();
      }).then(function (payload) {
        var media = payload.media || [];
        if (!Array.isArray(media) || media.length > pageSize) throw new Error("Ungültige Fotoframe-Seite");
        media.forEach(function (item) {
          items.push(photoMedia.applyPhotoItemDetails({
            node: null,
            detailsLoaded: true,
            detailPromise: null
          }, item));
        });
        exhausted = !payload.has_next || !media.length;
        total = payload.total || 0;
        page += 1;
        if (!items.length && !currentItem && count) count.textContent = "Keine Medien";
      }).catch(function (error) {
        if (error.name === "AbortError") return;
        exhausted = true;
        if (!items.length && !currentItem && count) count.textContent = "Keine Medien";
      }).finally(function () {
        loading = null;
        loadController = null;
      });
      return loading;
    }

    function isCurrentFrameItem(item) {
      return active() && currentItem === item;
    }

    function revealFrameMedia(media) {
      if (!media) return;
      media.classList.remove("is-visible");
      var schedule = window.requestAnimationFrame || function (callback) {
        return window.setTimeout(callback, 16);
      };
      schedule(function () {
        media.classList.add("is-visible");
      });
    }

    function showFrameImage(src) {
      if (!src) return;
      video.classList.remove("is-visible");
      video.hidden = true;
      video.pause();
      video.removeAttribute("src");
      var unchanged = image.getAttribute("src") === src && !image.hidden;
      image.setAttribute("src", src);
      image.hidden = false;
      if (!unchanged) {
        revealFrameMedia(image);
      }
    }

    function showFrameVideo(src) {
      if (!src) return;
      image.classList.remove("is-visible");
      image.hidden = true;
      image.removeAttribute("src");
      video.pause();
      video.removeAttribute("src");
      video.src = src;
      video.hidden = false;
      revealFrameMedia(video);
      video.play().catch(function () {});
    }

    function swapFrameImage(src, fallback, item) {
      var target = src || fallback || "";
      if (!target) return;
      if (image.getAttribute("src") === target && !image.hidden) return;
      if (imageLoader) imageLoader.removeAttribute("src");
      var loader = new Image();
      imageLoader = loader;
      loader.decoding = "async";
      loader.onload = function () {
        if (!isCurrentFrameItem(item)) return;
        showFrameImage(target);
      };
      loader.onerror = function () {
        if (!isCurrentFrameItem(item) || !fallback || fallback === target) return;
        showFrameImage(fallback);
      };
      loader.src = target;
    }

    function display(item, resuming) {
      title.textContent = item.title;
      if (item.type === "video") {
        if (resuming && video.getAttribute("src") === item.src && !video.hidden) {
          video.play().catch(function () {});
        } else {
          showFrameVideo(item.src);
        }
      } else {
        var target = photoMedia.bestPhotoDisplaySrc(item);
        var fallback = item.preview || item.thumb || item.largePreview || item.original || item.src || "";
        if (target) {
          swapFrameImage(target, fallback, item);
        } else if (fallback) {
          showFrameImage(fallback);
        }
      }
    }

    function show() {
      if (!active() || waitingForItems) return;
      if (exhausted && !items.length && !currentItem) return;
      if (index >= items.length) {
        if (exhausted) {
          items = [];
          index = 0;
          page = 1;
          shown = 0;
          exhausted = false;
        }
        waitingForItems = true;
        loadFrameItems().then(function () {
          waitingForItems = false;
          if (active() && index < items.length) show();
        });
        return;
      }
      currentItem = items[index++];
      shown += 1;
      if (count) count.textContent = total ? (shown + " von " + total + " Medien") : (shown + " Medien");
      display(currentItem);
      if (index >= pageSize) {
        items.splice(0, pageSize);
        index -= pageSize;
      }
      if (!exhausted && index + 5 >= items.length) loadFrameItems();
    }

    function pause() {
      if (timer !== null) window.clearInterval(timer);
      timer = null;
      video.pause();
      if (loadController) loadController.abort();
      if (imageLoader) {
        imageLoader.onload = null;
        imageLoader.onerror = null;
        imageLoader.removeAttribute("src");
        imageLoader = null;
      }
    }

    function resume() {
      if (!active() || timer !== null) return;
      if (currentItem) display(currentItem, true);
      else show();
      timer = window.setInterval(show, frameDelay);
    }

    document.addEventListener("visibilitychange", function () {
      if (active()) resume();
      else pause();
    });
    window.addEventListener("pagehide", function () {
      suspended = true;
      pause();
    });
    window.addEventListener("pageshow", function () {
      suspended = false;
      resume();
    });
    resume();
  }

  window.BearStack = window.BearStack || {};
  window.BearStack.photos = window.BearStack.photos || {};
  window.BearStack.photos.frame = Object.assign(window.BearStack.photos.frame || {}, {
    init: initFrame
  });

  document.addEventListener("DOMContentLoaded", initFrame);
})();
