(function () {
  var photoMapTileSize = 256;
  var photoMapMinZoom = 2;
  var photoMapMaxZoom = 18;
  var photoMapFitMaxZoom = 15;
  var photoMapMaxLat = 85.05112878;

  function clampPhotoMap(value, min, max) {
    return Math.max(min, Math.min(max, value));
  }

  function wrapPhotoMapX(value, zoom) {
    var size = photoMapWorldSize(zoom);
    return ((value % size) + size) % size;
  }

  function photoMapWorldSize(zoom) {
    return photoMapTileSize * Math.pow(2, zoom);
  }

  function projectPhotoMap(lat, lon, zoom) {
    var size = photoMapWorldSize(zoom);
    var safeLat = clampPhotoMap(lat, -photoMapMaxLat, photoMapMaxLat);
    var sin = Math.sin(safeLat * Math.PI / 180);
    return {
      x: (lon + 180) / 360 * size,
      y: (0.5 - Math.log((1 + sin) / (1 - sin)) / (4 * Math.PI)) * size
    };
  }

  function unprojectPhotoMap(x, y, zoom) {
    var size = photoMapWorldSize(zoom);
    var lon = x / size * 360 - 180;
    var latRadians = Math.atan(Math.sinh(Math.PI * (1 - 2 * y / size)));
    return {
      lat: latRadians * 180 / Math.PI,
      lon: lon
    };
  }

  function photoMapInitialState(positions, width, height) {
    var minLat = Math.min.apply(null, positions.map(function (position) { return position.lat; }));
    var maxLat = Math.max.apply(null, positions.map(function (position) { return position.lat; }));
    var minLon = Math.min.apply(null, positions.map(function (position) { return position.lon; }));
    var maxLon = Math.max.apply(null, positions.map(function (position) { return position.lon; }));
    var centerLat = (minLat + maxLat) / 2;
    var centerLon = (minLon + maxLon) / 2;
    if (positions.length === 1 || (Math.abs(maxLat - minLat) < 0.00001 && Math.abs(maxLon - minLon) < 0.00001)) {
      return { lat: centerLat, lon: centerLon, zoom: 15 };
    }

    var padding = Math.min(90, Math.max(36, Math.min(width, height) * 0.14));
    var availableWidth = Math.max(1, width - padding * 2);
    var availableHeight = Math.max(1, height - padding * 2);
    var zoom = photoMapMinZoom;
    for (var candidate = photoMapFitMaxZoom; candidate >= photoMapMinZoom; candidate -= 1) {
      var southWest = projectPhotoMap(minLat, minLon, candidate);
      var northEast = projectPhotoMap(maxLat, maxLon, candidate);
      var spanX = Math.abs(northEast.x - southWest.x);
      var spanY = Math.abs(southWest.y - northEast.y);
      if (spanX <= availableWidth && spanY <= availableHeight) {
        zoom = candidate;
        break;
      }
    }

    var sw = projectPhotoMap(minLat, minLon, zoom);
    var ne = projectPhotoMap(maxLat, maxLon, zoom);
    var center = unprojectPhotoMap((sw.x + ne.x) / 2, (sw.y + ne.y) / 2, zoom);
    return {
      lat: center.lat,
      lon: center.lon,
      zoom: zoom
    };
  }

  function photoMapExternalURL(lat, lon, zoom) {
    var safeZoom = clampPhotoMap(zoom || 17, photoMapMinZoom, photoMapMaxZoom);
    var latValue = lat.toFixed(6);
    var lonValue = lon.toFixed(6);
    return "https://www.openstreetmap.org/?mlat=" + encodeURIComponent(latValue) + "&mlon=" + encodeURIComponent(lonValue) + "#map=" + safeZoom + "/" + encodeURIComponent(latValue) + "/" + encodeURIComponent(lonValue);
  }

  function photoMapScreenPoint(position, state, size) {
    return photoMapProjector(state, size)(position);
  }

  function photoMapProjector(state, size) {
    var center = projectPhotoMap(state.lat, state.lon, state.zoom);
    var worldSize = photoMapWorldSize(state.zoom);
    return function (position) {
      var world = projectPhotoMap(position.lat, position.lon, state.zoom);
      var deltaX = world.x - center.x;
      if (deltaX > worldSize / 2) deltaX -= worldSize;
      if (deltaX < -worldSize / 2) deltaX += worldSize;
      return {
        x: deltaX + size.width / 2,
        y: world.y - center.y + size.height / 2
      };
    };
  }

  function renderPhotoMapTiles(tileLayer, state, size, tileCache) {
    if (!tileLayer || !state || !size || !size.width || !size.height) return;
    var center = projectPhotoMap(state.lat, state.lon, state.zoom);
    var topLeftX = center.x - size.width / 2;
    var topLeftY = center.y - size.height / 2;
    var tileCount = Math.pow(2, state.zoom);
    var startX = Math.floor(topLeftX / photoMapTileSize);
    var endX = Math.floor((topLeftX + size.width) / photoMapTileSize);
    var startY = Math.floor(topLeftY / photoMapTileSize);
    var endY = Math.floor((topLeftY + size.height) / photoMapTileSize);
    var cache = tileCache || new Map();
    var visible = new Set();

    for (var tileY = startY; tileY <= endY; tileY += 1) {
      if (tileY < 0 || tileY >= tileCount) continue;
      for (var tileX = startX; tileX <= endX; tileX += 1) {
        var wrappedX = ((tileX % tileCount) + tileCount) % tileCount;
        var key = state.zoom + "/" + wrappedX + "/" + tileY;
        var tile = cache.get(key);
        if (!tile) {
          tile = document.createElement("img");
          tile.className = "photo-map-tile";
          tile.alt = "";
          tile.decoding = "async";
          tile.draggable = false;
          tile.referrerPolicy = "origin";
          tile.src = "https://tile.openstreetmap.org/" + key + ".png";
        }
        // Insertion order is the LRU order; visible tiles survive eviction.
        cache.delete(key);
        cache.set(key, tile);
        visible.add(tile);
        tile.style.transform = "translate(" + Math.round(tileX * photoMapTileSize - topLeftX) + "px, " + Math.round(tileY * photoMapTileSize - topLeftY) + "px)";
        if (tile.parentNode !== tileLayer) tileLayer.appendChild(tile);
      }
    }

    Array.from(tileLayer.children).forEach(function (tile) {
      if (!visible.has(tile)) tile.remove();
    });
    var limit = Math.max(128, visible.size);
    for (var entry of cache) {
      if (cache.size <= limit) break;
      if (!visible.has(entry[1])) cache.delete(entry[0]);
    }
  }

  window.BearStack = window.BearStack || {};
  window.BearStack.photos = window.BearStack.photos || {};
  window.BearStack.photos.map = Object.assign(window.BearStack.photos.map || {}, {
    externalURL: photoMapExternalURL,
    minZoom: photoMapMinZoom,
    maxZoom: photoMapMaxZoom,
    clamp: clampPhotoMap,
    wrapX: wrapPhotoMapX,
    worldSize: photoMapWorldSize,
    project: projectPhotoMap,
    unproject: unprojectPhotoMap,
    projector: photoMapProjector,
    initialState: photoMapInitialState,
    renderTiles: renderPhotoMapTiles,
    screenPoint: photoMapScreenPoint
  });
})();
