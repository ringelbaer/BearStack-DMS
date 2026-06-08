(function () {
  var photoThumbQueue = [];
  var photoThumbRunning = 0;
  var photoThumbLimit = 2;
  var photoThumbQueueDirty = false;
  var photoThumbPumpFrame = 0;
  var photoThumbStatusWait = new Map();
  var photoThumbStatusTimer = 0;
  var photoThumbStatusInFlight = false;
  var photoThumbInitBatchSize = 160;

  function markThumbnailLoaded(img) {
    var wrap = img.closest(".photo-thumb-wrap, .photo-folder-preview-cell");
    if (wrap) {
      wrap.classList.add("is-loaded");
      wrap.classList.remove("is-generating");
    }
  }

  function markThumbnailError(img) {
    var wrap = img.closest(".photo-thumb-wrap, .photo-folder-preview-cell");
    if (wrap) {
      wrap.classList.add("is-error");
      wrap.classList.remove("is-generating");
    }
  }

  function markPhotoThumbReady(img) {
    if (!img || img.dataset.photoThumbLoaded === "1") return;
    img.dataset.photoThumbLoaded = "1";
    img.dataset.photoThumbReady = "1";
    markThumbnailLoaded(img);
  }

  function markPhotoThumbBroken(img) {
    if (!img || img.dataset.photoThumbLoaded === "1") return;
    img.dataset.photoThumbLoaded = "1";
    markThumbnailError(img);
    markThumbnailLoaded(img);
  }

  function watchNativePhotoThumb(img) {
    if (!img || img.dataset.photoThumbObserved === "1") return;
    if (img.complete) {
      if (img.naturalWidth > 0) {
        markPhotoThumbReady(img);
      } else {
        markPhotoThumbBroken(img);
      }
      return;
    }
    img.dataset.photoThumbObserved = "1";
    img.addEventListener("load", function () {
      delete img.dataset.photoThumbObserved;
      markPhotoThumbReady(img);
    }, { once: true });
    img.addEventListener("error", function () {
      delete img.dataset.photoThumbObserved;
      markPhotoThumbBroken(img);
    }, { once: true });
  }

  function photoThumbWrap(img) {
    return img?.closest(".photo-thumb-wrap, .photo-folder-preview-cell") || null;
  }

  function setPhotoThumbGenerating(img, generating) {
    var wrap = photoThumbWrap(img);
    if (!wrap || !wrap.querySelector(".photo-thumb-loader")) return;
    wrap.classList.toggle("is-generating", generating);
  }

  function photoThumbRequest(src) {
    if (!src) return "";
    try {
      var url = new URL(src, window.location.href);
      if (url.origin !== window.location.origin || url.pathname !== "/photos/thumbnail") {
        return null;
      }
      var path = url.searchParams.get("path") || "";
      var size = Number.parseInt(url.searchParams.get("size") || "420", 10);
      if (!path) return null;
      return {
        key: path + "\n" + (Number.isFinite(size) ? size : 420),
        path: path,
        size: Number.isFinite(size) ? size : 420
      };
    } catch (_) {
      return null;
    }
  }

  function photoThumbVisible(img) {
    if (!img || !document.documentElement.contains(img)) return false;
    var rect = img.getBoundingClientRect();
    return rect.bottom >= -360 && rect.top <= (window.innerHeight || 0) + 360;
  }

  function waitForPhotoThumbReady(img, src, attempt) {
    if (!img || img.dataset.photoThumbLoaded === "1") return;
    var request = photoThumbRequest(src);
    if (!request || typeof window.fetch !== "function") {
      img.dataset.photoThumbReady = "1";
      enqueuePhotoThumb(img, photoThumbPriority(img));
      return;
    }
    var entry = photoThumbStatusWait.get(request.key);
    if (!entry) {
      entry = {
        path: request.path,
        size: request.size,
        images: new Set(),
        attempt: attempt || 0
      };
      photoThumbStatusWait.set(request.key, entry);
    }
    entry.images.add(img);
    schedulePhotoThumbStatusBatch(entry.attempt <= 0 ? 0 : Math.min(8000, 500 + entry.attempt * 500));
  }

  function schedulePhotoThumbStatusBatch(delay) {
    if (photoThumbStatusTimer) return;
    photoThumbStatusTimer = window.setTimeout(function () {
      photoThumbStatusTimer = 0;
      pollPhotoThumbStatusBatch();
    }, Math.max(0, delay || 0));
  }

  function pollPhotoThumbStatusBatch() {
    if (photoThumbStatusInFlight || photoThumbStatusWait.size === 0 || typeof window.fetch !== "function") return;
    var activeEntries = [];
    photoThumbStatusWait.forEach(function (entry, key) {
      entry.images.forEach(function (img) {
        if (!img || img.dataset.photoThumbLoaded === "1" || !document.documentElement.contains(img)) {
          entry.images.delete(img);
        }
      });
      var hasVisibleImage = Array.from(entry.images).some(photoThumbVisible);
      if (!entry.images.size) {
        photoThumbStatusWait.delete(key);
      } else if (hasVisibleImage) {
        activeEntries.push(entry);
      }
    });
    if (!activeEntries.length) {
      if (photoThumbStatusWait.size > 0) schedulePhotoThumbStatusBatch(1000);
      return;
    }
    photoThumbStatusInFlight = true;
    window.fetch("/photos/thumbnail/status", {
      method: "POST",
      headers: { "Accept": "application/json", "Content-Type": "application/json" },
      credentials: "same-origin",
      body: JSON.stringify({
        items: activeEntries.map(function (entry) {
          return { path: entry.path, size: entry.size };
        })
      })
    }).then(function (response) {
      if (!response.ok) throw new Error("thumbnail status failed");
      return response.json();
    }).catch(function () {
      return { items: [] };
    }).then(function (payload) {
      var nextDelay = 0;
      (payload.items || []).forEach(function (status) {
        var key = (status.path || "") + "\n" + (status.size || 420);
        var entry = photoThumbStatusWait.get(key);
        if (!entry) return;
        if (status.ready) {
          entry.images.forEach(function (img) {
            if (!img || img.dataset.photoThumbLoaded === "1" || !document.documentElement.contains(img)) return;
            img.dataset.photoThumbReady = "1";
            enqueuePhotoThumb(img, photoThumbPriority(img));
          });
          photoThumbStatusWait.delete(key);
          return;
        }
        entry.attempt += 1;
        entry.images.forEach(function (img) {
          setPhotoThumbGenerating(img, true);
          if (entry.attempt >= 40) {
            markThumbnailError(img);
            markThumbnailLoaded(img);
          }
        });
        if (entry.attempt >= 40) {
          photoThumbStatusWait.delete(key);
        } else {
          nextDelay = Math.max(nextDelay, Math.min(8000, 500 + entry.attempt * 500));
        }
      });
      if (photoThumbStatusWait.size > 0) {
        schedulePhotoThumbStatusBatch(nextDelay || 1000);
      }
    }).finally(function () {
      photoThumbStatusInFlight = false;
    });
  }

  function photoThumbPriorityFromRect(rect) {
    if (rect.bottom >= 0 && rect.top <= window.innerHeight) {
      return 0;
    }
    return Math.abs(rect.top);
  }

  function photoThumbPriority(img) {
    return photoThumbPriorityFromRect(img.getBoundingClientRect());
  }

  function photoThumbType(img) {
    return img.dataset.photoThumbType || img.closest("[data-photo-item]")?.dataset.photoType || "image";
  }

  function photoThumbTypeRank(img) {
    return photoThumbType(img) === "video" ? 1 : 0;
  }

  function enqueuePhotoThumb(img, priority) {
    if (!img || img.dataset.photoThumbLoaded === "1" || img.dataset.photoThumbQueued === "1" || img.dataset.photoThumbLoading === "1") {
      return;
    }
    if (!img.dataset.photoThumbSrc && !img.getAttribute("src")) {
      return;
    }
    img.dataset.photoThumbQueued = "1";
    photoThumbQueue.push({
      img: img,
      typeRank: photoThumbTypeRank(img),
      priority: Number.isFinite(priority) ? priority : photoThumbPriority(img)
    });
    photoThumbQueueDirty = true;
    schedulePhotoThumbQueue();
  }

  function finishPhotoThumb(img) {
    delete img.dataset.photoThumbQueued;
    delete img.dataset.photoThumbLoading;
    setPhotoThumbGenerating(img, false);
    photoThumbRunning = Math.max(0, photoThumbRunning - 1);
    schedulePhotoThumbQueue();
  }

  function startPhotoThumb(img) {
    var src = img.dataset.photoThumbSrc || img.getAttribute("src") || "";
    if (!src) {
      finishPhotoThumb(img);
      return;
    }
    if ("loading" in img) {
      img.loading = "eager";
    }
    img.dataset.photoThumbLoading = "1";
    img.onload = function () {
      img.onload = null;
      img.onerror = null;
      markPhotoThumbReady(img);
      finishPhotoThumb(img);
    };
    img.onerror = function () {
      var fallback = img.dataset.photoFallbackSrc || "";
      img.onload = null;
      img.onerror = null;
      finishPhotoThumb(img);
      if (fallback && img.src !== fallback) {
        img.removeAttribute("data-photo-fallback-src");
        img.dataset.photoThumbSrc = fallback;
        delete img.dataset.photoThumbReady;
        enqueuePhotoThumb(img);
        return;
      }
      markPhotoThumbBroken(img);
    };
    if (!photoThumbWrap(img)?.querySelector(".photo-thumb-loader")) {
      img.src = src;
      return;
    }
    if (img.dataset.photoThumbReady === "1") {
      setPhotoThumbGenerating(img, false);
      img.src = src;
      return;
    }
    if (img.dataset.photoThumbReady === "0") {
      setPhotoThumbGenerating(img, true);
      finishPhotoThumb(img);
      waitForPhotoThumbReady(img, src, 0);
      return;
    }
    img.src = src;
  }

  function schedulePhotoThumbQueue() {
    if (photoThumbPumpFrame) return;
    var schedule = window.requestAnimationFrame || function (callback) {
      return window.setTimeout(callback, 0);
    };
    photoThumbPumpFrame = schedule(function () {
      photoThumbPumpFrame = 0;
      pumpPhotoThumbQueue();
    });
  }

  function pumpPhotoThumbQueue() {
    if (photoThumbRunning >= photoThumbLimit || photoThumbQueue.length === 0) {
      return;
    }
    if (photoThumbQueueDirty) {
      photoThumbQueue.sort(function (a, b) {
        return (a.typeRank - b.typeRank) || (a.priority - b.priority);
      });
      photoThumbQueueDirty = false;
    }
    while (photoThumbRunning < photoThumbLimit && photoThumbQueue.length > 0) {
      var task = photoThumbQueue.shift();
      if (!task.img || task.img.dataset.photoThumbLoaded === "1" || task.img.dataset.photoThumbLoading === "1") {
        if (task.img) {
          delete task.img.dataset.photoThumbQueued;
        }
        continue;
      }
      photoThumbRunning++;
      startPhotoThumb(task.img);
    }
  }

  function initPhotoThumbnails() {
    var observer = "IntersectionObserver" in window ? new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (!entry.isIntersecting) return;
        observer.unobserve(entry.target);
        enqueuePhotoThumb(entry.target, photoThumbPriorityFromRect(entry.boundingClientRect));
      });
    }, { rootMargin: "360px 0px", threshold: 0.01 }) : null;

    var images = Array.from(document.querySelectorAll("[data-photo-thumb-image]"));
    var index = 0;
    var schedule = window.requestAnimationFrame || function (callback) {
      return window.setTimeout(callback, 0);
    };

    function processChunk() {
      var end = Math.min(images.length, index + photoThumbInitBatchSize);
      while (index < end) {
        var img = images[index];
        index += 1;
        if (img.dataset.photoThumbReady === "1" && img.dataset.photoThumbSrc) {
          if (!img.getAttribute("src")) {
            img.src = img.dataset.photoThumbSrc;
          }
          markPhotoThumbReady(img);
          continue;
        }
        if (img.getAttribute("src")) {
          watchNativePhotoThumb(img);
          continue;
        }
        if (observer && img.dataset.photoThumbSrc) {
          observer.observe(img);
        } else {
          enqueuePhotoThumb(img);
        }
      }
      if (index < images.length) {
        schedule(processChunk);
      }
    }

    processChunk();
  }

  window.BearStack = window.BearStack || {};
  window.BearStack.photos = window.BearStack.photos || {};
  window.BearStack.photos.thumbnails = Object.assign(window.BearStack.photos.thumbnails || {}, {
    init: initPhotoThumbnails
  });
})();
