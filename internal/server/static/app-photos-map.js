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
    var center = projectPhotoMap(state.lat, state.lon, state.zoom);
    var world = projectPhotoMap(position.lat, position.lon, state.zoom);
    var worldSize = photoMapWorldSize(state.zoom);
    var deltaX = world.x - center.x;
    if (deltaX > worldSize / 2) deltaX -= worldSize;
    if (deltaX < -worldSize / 2) deltaX += worldSize;
    return {
      x: deltaX + size.width / 2,
      y: world.y - center.y + size.height / 2
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
    var fragment = document.createDocumentFragment();

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
          cache.set(key, tile);
        }
        tile.style.transform = "translate(" + Math.round(tileX * photoMapTileSize - topLeftX) + "px, " + Math.round(tileY * photoMapTileSize - topLeftY) + "px)";
        fragment.appendChild(tile);
      }
    }

    tileLayer.replaceChildren(fragment);
  }

  function parsePhotoMapPoints(value) {
    return (value || "").trim().split(/\s+/).map(function (entry) {
      var parts = entry.split(",");
      return {
        lat: parseFloat(parts[0] || ""),
        lon: parseFloat(parts[1] || "")
      };
    }).filter(function (point) {
      return Number.isFinite(point.lat) && Number.isFinite(point.lon);
    });
  }

  function initMap() {
    var panel = document.querySelector("[data-photo-map]");
    if (!panel) return;
    var canvas = panel.querySelector("[data-photo-map-canvas]");
    var tileLayer = panel.querySelector("[data-photo-map-tiles]");
    var routeLayer = panel.querySelector("[data-photo-map-route]");
    var zoomIn = panel.querySelector("[data-photo-map-zoom-in]");
    var zoomOut = panel.querySelector("[data-photo-map-zoom-out]");
    var markers = Array.from(panel.querySelectorAll("[data-photo-map-marker]"));
    var routePointNodes = Array.from(panel.querySelectorAll("[data-photo-route-point]"));
    var layerToggles = Array.from(panel.querySelectorAll("[data-photo-layer-toggle]"));
    var gpxTrackNodes = Array.from(panel.querySelectorAll("[data-photo-map-gpx-track]"));
    var gpxLabelNodes = Array.from(panel.querySelectorAll("[data-photo-map-gpx-label]"));
    var gpxLabels = new Map(gpxLabelNodes.map(function (label) {
      return [label.dataset.photoMapLayer || "", label];
    }));
    var tileCache = new Map();
    var renderQueued = false;
    var layerVisibility = {};
    layerToggles.forEach(function (toggle) {
      var layer = toggle.dataset.photoMapLayer || "";
      layerVisibility[layer] = toggle.checked;
    });
    function layerVisible(layer) {
      return layerVisibility[layer] !== false;
    }
    var positions = markers.map(function (marker) {
      return {
        node: marker,
        marker: marker,
        lat: parseFloat(marker.dataset.lat || ""),
        lon: parseFloat(marker.dataset.lon || "")
      };
    }).filter(function (position) {
      return Number.isFinite(position.lat) && Number.isFinite(position.lon);
    });
    var routePositions = routePointNodes.map(function (point) {
      return {
        node: point,
        lat: parseFloat(point.dataset.lat || ""),
        lon: parseFloat(point.dataset.lon || "")
      };
    }).filter(function (position) {
      return Number.isFinite(position.lat) && Number.isFinite(position.lon);
    });
    var gpxTracks = gpxTrackNodes.map(function (track) {
      var layer = track.dataset.photoMapLayer || "";
      return {
        node: track,
        label: gpxLabels.get(layer) || null,
        layer: layer,
        color: track.dataset.color || "",
        labelText: track.dataset.label || "",
        points: parsePhotoMapPoints(track.dataset.points || "")
      };
    }).filter(function (track) {
      return track.layer && track.points.length > 0;
    });
    var gpxPositions = [];
    gpxTracks.forEach(function (track) {
      track.points.forEach(function (point) {
        gpxPositions.push(point);
      });
    });
    var allPositions = positions.concat(routePositions, gpxPositions);
    if (!canvas || !allPositions.length) {
      if (canvas) canvas.classList.add("empty");
      renderPhotoRoute(routeLayer, [], null);
      gpxTracks.forEach(function (track) {
        renderPhotoMapTrack(track, false, null, null);
      });
      return;
    }
    canvas.classList.remove("empty");

    var state = photoMapInitialState(allPositions, canvas.clientWidth || 640, canvas.clientHeight || 320);

    function canvasSize() {
      return {
        width: Math.max(1, canvas.clientWidth || canvas.offsetWidth || 640),
        height: Math.max(1, canvas.clientHeight || canvas.offsetHeight || 320)
      };
    }

    function centerWorld() {
      return projectPhotoMap(state.lat, state.lon, state.zoom);
    }

    function setStateFromWorld(world, zoom) {
      var size = photoMapWorldSize(zoom);
      var center = unprojectPhotoMap(
        wrapPhotoMapX(world.x, zoom),
        clampPhotoMap(world.y, 0, size),
        zoom
      );
      state = {
        lat: center.lat,
        lon: center.lon,
        zoom: zoom
      };
      updateControls();
      scheduleRender();
    }

    function screenPoint(position) {
      return photoMapScreenPoint(position, state, canvasSize());
    }

    function renderTiles() {
      renderPhotoMapTiles(tileLayer, state, canvasSize(), tileCache);
    }

    function renderMarkers() {
      var showPhotos = layerVisible("photos");
      positions.forEach(function (position) {
        position.node.hidden = !showPhotos;
        if (!showPhotos) return;
        var point = screenPoint(position);
        position.node.style.left = point.x.toFixed(2) + "px";
        position.node.style.top = point.y.toFixed(2) + "px";
      });
      var showRoute = layerVisible("photo-route");
      routePositions.forEach(function (position) {
        position.node.hidden = !showRoute;
        if (!showRoute) return;
        var point = screenPoint(position);
        position.node.style.left = point.x.toFixed(2) + "px";
        position.node.style.top = point.y.toFixed(2) + "px";
      });
      var size = canvasSize();
      renderPhotoRoute(routeLayer, showRoute ? routePositions : [], screenPoint, size);
      gpxTracks.forEach(function (track) {
        renderPhotoMapTrack(track, layerVisible(track.layer), screenPoint, size);
      });
    }

    function updateControls() {
      if (zoomIn) zoomIn.disabled = state.zoom >= photoMapMaxZoom;
      if (zoomOut) zoomOut.disabled = state.zoom <= photoMapMinZoom;
    }

    function renderMap() {
      renderTiles();
      renderMarkers();
    }

    function scheduleRender() {
      if (renderQueued) return;
      renderQueued = true;
      function flushRender() {
        if (!renderQueued) return;
        renderQueued = false;
        renderMap();
      }
      window.requestAnimationFrame(flushRender);
      window.setTimeout(flushRender, 80);
    }

    function zoomBy(delta, clientX, clientY) {
      var nextZoom = clampPhotoMap(state.zoom + delta, photoMapMinZoom, photoMapMaxZoom);
      if (nextZoom === state.zoom) return;
      var rect = canvas.getBoundingClientRect();
      var size = canvasSize();
      var anchorX = Number.isFinite(clientX) ? clientX - rect.left : size.width / 2;
      var anchorY = Number.isFinite(clientY) ? clientY - rect.top : size.height / 2;
      anchorX = clampPhotoMap(anchorX, 0, size.width);
      anchorY = clampPhotoMap(anchorY, 0, size.height);
      var oldCenter = centerWorld();
      var anchor = unprojectPhotoMap(
        oldCenter.x + anchorX - size.width / 2,
        oldCenter.y + anchorY - size.height / 2,
        state.zoom
      );
      var nextAnchor = projectPhotoMap(anchor.lat, anchor.lon, nextZoom);
      setStateFromWorld({
        x: nextAnchor.x - anchorX + size.width / 2,
        y: nextAnchor.y - anchorY + size.height / 2
      }, nextZoom);
    }

    if (zoomIn) {
      zoomIn.addEventListener("click", function (event) {
        event.stopPropagation();
        zoomBy(1);
      });
    }
    if (zoomOut) {
      zoomOut.addEventListener("click", function (event) {
        event.stopPropagation();
        zoomBy(-1);
      });
    }
    layerToggles.forEach(function (toggle) {
      toggle.addEventListener("change", function () {
        layerVisibility[toggle.dataset.photoMapLayer || ""] = toggle.checked;
        scheduleRender();
      });
    });

    var drag = null;
    canvas.addEventListener("pointerdown", function (event) {
      if (event.button !== undefined && event.button !== 0) return;
      if (event.target.closest("[data-photo-map-marker], [data-photo-route-point], .photo-map-controls, .photo-map-layer-panel, .photo-map-attribution")) return;
      var center = centerWorld();
      drag = {
        pointerId: event.pointerId,
        startX: event.clientX,
        startY: event.clientY,
        centerX: center.x,
        centerY: center.y
      };
      canvas.classList.add("is-dragging");
      canvas.setPointerCapture(event.pointerId);
      event.preventDefault();
    });

    canvas.addEventListener("pointermove", function (event) {
      if (!drag || drag.pointerId !== event.pointerId) return;
      setStateFromWorld({
        x: drag.centerX - (event.clientX - drag.startX),
        y: drag.centerY - (event.clientY - drag.startY)
      }, state.zoom);
      event.preventDefault();
    });

    function endDrag(event) {
      if (!drag || drag.pointerId !== event.pointerId) return;
      drag = null;
      canvas.classList.remove("is-dragging");
    }

    canvas.addEventListener("pointerup", endDrag);
    canvas.addEventListener("pointercancel", endDrag);

    canvas.addEventListener("wheel", function (event) {
      event.preventDefault();
      zoomBy(event.deltaY < 0 ? 1 : -1, event.clientX, event.clientY);
    }, { passive: false });

    canvas.addEventListener("dblclick", function (event) {
      if (event.target.closest("[data-photo-map-marker], [data-photo-route-point], .photo-map-controls, .photo-map-layer-panel, .photo-map-attribution")) return;
      event.preventDefault();
      zoomBy(1, event.clientX, event.clientY);
    });

    canvas.addEventListener("keydown", function (event) {
      if (event.target !== canvas) return;
      var pan = 80;
      var center = centerWorld();
      if (event.key === "+" || event.key === "=") {
        event.preventDefault();
        zoomBy(1);
        return;
      }
      if (event.key === "-") {
        event.preventDefault();
        zoomBy(-1);
        return;
      }
      if (event.key === "ArrowLeft") {
        center.x -= pan;
      } else if (event.key === "ArrowRight") {
        center.x += pan;
      } else if (event.key === "ArrowUp") {
        center.y -= pan;
      } else if (event.key === "ArrowDown") {
        center.y += pan;
      } else {
        return;
      }
      event.preventDefault();
      setStateFromWorld(center, state.zoom);
    });

    window.addEventListener("resize", scheduleRender);
    updateControls();
    renderMap();
  }

  function renderPhotoRoute(routeLayer, routePositions, project, size) {
    renderPhotoMapPolyline(routeLayer, routePositions, project, size, "photo-map-route-line", "", "");
  }

  function renderPhotoMapTrack(track, visible, project, size) {
    if (!track) return;
    var points = visible ? track.points : [];
    renderPhotoMapPolyline(track.node, points, project, size, "photo-map-gpx-line", track.color, track.labelText);
    if (!track.label) return;
    if (!visible || !project || !track.points.length) {
      track.label.hidden = true;
      return;
    }
    var anchor = project(track.points[Math.floor(track.points.length / 2)]);
    track.label.hidden = false;
    track.label.style.left = anchor.x.toFixed(2) + "px";
    track.label.style.top = anchor.y.toFixed(2) + "px";
  }

  function renderPhotoMapPolyline(layer, positions, project, size, className, color, label) {
    if (!layer) return;
    while (layer.firstChild) {
      layer.removeChild(layer.firstChild);
    }
    if (!project || positions.length < 2 || !size || !size.width || !size.height) {
      layer.setAttribute("hidden", "");
      return;
    }
    layer.removeAttribute("hidden");
    layer.setAttribute("viewBox", "0 0 " + size.width + " " + size.height);
    var polyline = document.createElementNS("http://www.w3.org/2000/svg", "polyline");
    polyline.setAttribute("class", className);
    if (color) {
      polyline.setAttribute("stroke", color);
    }
    if (label) {
      var title = document.createElementNS("http://www.w3.org/2000/svg", "title");
      title.textContent = label;
      polyline.appendChild(title);
    }
    polyline.setAttribute("points", positions.map(function (position) {
      var point = project(position);
      return point.x.toFixed(2) + "," + point.y.toFixed(2);
    }).join(" "));
    layer.appendChild(polyline);
  }

  window.BearStack = window.BearStack || {};
  window.BearStack.photos = window.BearStack.photos || {};
  window.BearStack.photos.map = Object.assign(window.BearStack.photos.map || {}, {
    externalURL: photoMapExternalURL,
    init: initMap,
    initialState: photoMapInitialState,
    renderTiles: renderPhotoMapTiles,
    screenPoint: photoMapScreenPoint
  });
})();
