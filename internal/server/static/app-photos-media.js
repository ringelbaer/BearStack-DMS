(function () {
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
      folderName: node.dataset.photoFolderName || "-",
      date: node.dataset.photoDate || "-",
      dateTime: node.dataset.photoDateTime || "-",
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
    item.folderName = data.folder_name || item.folderName || "-";
    item.date = data.date || item.date || "-";
    item.dateTime = data.date_time || data.dateTime || item.date || item.dateTime || "-";
    item.automaticFaces = data.automatic_faces || [];
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

  window.BearStack = window.BearStack || {};
  window.BearStack.photos = Object.assign(window.BearStack.photos || {}, {
    applyPhotoItemDetails: applyPhotoItemDetails,
    bestPhotoDisplaySrc: bestPhotoDisplaySrc,
    collectItems: collectItems,
    formatPhotoRating: formatPhotoRating,
    photoItemFromNode: photoItemFromNode,
    positiveNumber: positiveNumber
  });

})();
