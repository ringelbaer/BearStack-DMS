(function () {
  function photoHelpers() {
    return (window.BearStack && window.BearStack.photos) || {};
  }

  function applyFramePhotoItemDetails(item, data) {
    var helpers = photoHelpers();
    if (typeof helpers.applyPhotoItemDetails === "function") {
      return helpers.applyPhotoItemDetails(item, data);
    }
    return Object.assign(item, data || {});
  }

  function bestFramePhotoDisplaySrc(item) {
    var helpers = photoHelpers();
    if (typeof helpers.bestPhotoDisplaySrc === "function") {
      return helpers.bestPhotoDisplaySrc(item);
    }
    return item.largePreview || item.preview || item.thumb || item.original || item.src || "";
  }

  function initFrame() {
    var frame = document.querySelector("[data-photo-frame]");
    if (!frame) return;
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
    var loading = false;
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

    function loadFrameItems() {
      if (loading || exhausted || typeof window.fetch !== "function") return Promise.resolve();
      loading = true;
      return window.fetch(frameItemsURL(), {
        credentials: "same-origin",
        headers: { Accept: "application/json" }
      }).then(function (response) {
        if (!response.ok) throw new Error("Fotoframe konnte keine Medien laden");
        return response.json();
      }).then(function (payload) {
        (payload.media || []).forEach(function (media) {
          items.push(applyFramePhotoItemDetails({
            node: null,
            detailsLoaded: true,
            detailPromise: null
          }, media));
        });
        exhausted = !payload.has_next;
        page += 1;
        if (count) {
          count.textContent = payload.total ? (items.length + " von " + payload.total + " Medien") : (items.length + " Medien");
        }
      }).catch(function () {
        exhausted = true;
        if (count) count.textContent = "Keine Medien";
      }).finally(function () {
        loading = false;
      });
    }

    function isCurrentFrameItem(item) {
      if (!items.length) return false;
      return items[(index - 1 + items.length) % items.length] === item;
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
      var loader = new Image();
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

    function show() {
      if (!items.length) {
        if (exhausted) return;
        loadFrameItems().then(show);
        return;
      }
      if (!exhausted && index + 5 >= items.length) {
        loadFrameItems();
      }
      var item = items[index % items.length];
      index += 1;
      title.textContent = item.title;
      if (item.type === "video") {
        showFrameVideo(item.src);
      } else {
        var target = bestFramePhotoDisplaySrc(item);
        var fallback = item.preview || item.thumb || item.largePreview || item.original || item.src || "";
        if (target) {
          swapFrameImage(target, fallback, item);
        } else if (fallback) {
          showFrameImage(fallback);
        }
      }
    }
    loadFrameItems().then(show);
    window.setInterval(show, frameDelay);
  }

  window.BearStack = window.BearStack || {};
  window.BearStack.photos = window.BearStack.photos || {};
  window.BearStack.photos.frame = Object.assign(window.BearStack.photos.frame || {}, {
    init: initFrame
  });

  document.addEventListener("DOMContentLoaded", initFrame);
})();
