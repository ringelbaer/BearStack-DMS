(function () {
  "use strict";
  window.BearStackFaceBoxes = {
    render: function (container, faces) {
      function clamp(value, min, max) { return Math.max(min, Math.min(max, value)); }
      var boxes = document.createDocumentFragment();
      faces.forEach(function (face) {
        var x = face.x, y = face.y, width = face.width, height = face.height;
        if (![x, y, width, height].every(Number.isFinite) || width <= 0 || height <= 0) return;
        var left = clamp(x, 0, 1), top = clamp(y, 0, 1);
        var right = clamp(x + width, left, 1), bottom = clamp(y + height, top, 1);
        if (right <= left || bottom <= top) return;
        var box = document.createElement("div");
        box.className = "photo-face-box";
        box.style.left = (left * 100) + "%"; box.style.top = (top * 100) + "%";
        box.style.width = ((right - left) * 100) + "%"; box.style.height = ((bottom - top) * 100) + "%";
        if (face.ignored) box.dataset.ignored = "";
        var label = document.createElement("span");
        label.textContent = (face.name || "Unbenannt") + (face.ignored ? " (ignoriert)" : "");
        box.title = label.textContent;
        box.append(label); boxes.append(box);
      });
      container.replaceChildren(boxes);
    }
  };
}());
