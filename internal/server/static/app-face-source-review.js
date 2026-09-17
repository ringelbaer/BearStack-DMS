(function () {
  "use strict";
  var root = document.querySelector("[data-face-source-review]");
  if (!root) return;
  var stage = root.querySelector("[data-review-stage]");
  var box = root.querySelector("[data-review-box]");
  var form = root.querySelector("[data-review-form]");
  function paint() {
    box.style.left = Number(form.elements.x.value) * 100 + "%";
    box.style.top = Number(form.elements.y.value) * 100 + "%";
    box.style.width = Number(form.elements.width.value) * 100 + "%";
    box.style.height = Number(form.elements.height.value) * 100 + "%";
  }
  function point(event) {
    var bounds = stage.getBoundingClientRect();
    return { x: Math.max(0, Math.min(1, (event.clientX - bounds.left) / bounds.width)),
      y: Math.max(0, Math.min(1, (event.clientY - bounds.top) / bounds.height)) };
  }
  var start = null;
  stage.addEventListener("pointerdown", function (event) {
    if (event.button !== 0) return;
    start = point(event); stage.setPointerCapture(event.pointerId); event.preventDefault();
  });
  stage.addEventListener("pointermove", function (event) {
    if (!start) return;
    var end = point(event);
    form.elements.x.value = Math.min(start.x, end.x);
    form.elements.y.value = Math.min(start.y, end.y);
    form.elements.width.value = Math.abs(end.x - start.x);
    form.elements.height.value = Math.abs(end.y - start.y);
    paint();
  });
  stage.addEventListener("pointerup", function () { start = null; });
  stage.addEventListener("pointercancel", function () { start = null; });
  form.addEventListener("input", paint); paint();
}());
