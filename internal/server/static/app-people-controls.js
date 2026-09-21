(function () {
  "use strict";
  if (window.BearStackPeopleControls) return;
  // The same interaction for groups, active faces and ignored faces; bounded to one page.
  function bindSelection(options) {
    var grid = options.grid, mode = false;
    var button = options.button, all = options.all, clear = options.clear;
    function inputs() { return Array.from(grid.querySelectorAll(options.input)); }
    function update() {
      var items = inputs(), count = items.filter(function (input) { return input.checked; }).length;
      var blocked = options.blocked();
      grid.dataset.selectionMode = String(mode);
      if (button) { button.hidden = false; button.disabled = blocked; button.setAttribute("aria-pressed", String(mode)); }
      if (all) { all.hidden = !mode && !count; all.disabled = blocked || !items.length; all.textContent = count && count === items.length ? "Auswahl aufheben" : "Alle auf dieser Seite"; }
      if (clear) clear.disabled = blocked;
      items.forEach(function (input) {
        input.disabled = blocked;
        var card = input.closest(options.card), target = card.querySelector(options.open) || card;
        if (mode) {
          var currentLabel = target.getAttribute("aria-label") || "";
          if (!target.hasAttribute("data-selection-label") || currentLabel !== target.dataset.selectionActionLabel) target.dataset.selectionLabel = currentLabel;
          target.dataset.selectionActionLabel = "Auswählen: " + (card.dataset.personName || card.dataset.displayPath || "Unbenannt");
          target.setAttribute("aria-label", target.dataset.selectionActionLabel);
          if (target.hasAttribute("href")) { target.dataset.selectionHref = target.getAttribute("href"); target.removeAttribute("href"); }
          if (target.tagName !== "BUTTON") { target.setAttribute("role", "button"); target.tabIndex = 0; }
          target.setAttribute("aria-pressed", String(input.checked));
        } else {
          if (target.hasAttribute("data-selection-label")) {
            if (target.dataset.selectionLabel) target.setAttribute("aria-label", target.dataset.selectionLabel);
            else target.removeAttribute("aria-label");
            delete target.dataset.selectionLabel; delete target.dataset.selectionActionLabel;
          }
          target.removeAttribute("aria-pressed");
          if (target.dataset.selectionHref) { target.setAttribute("href", target.dataset.selectionHref); delete target.dataset.selectionHref; }
          if (target.tagName !== "BUTTON") { target.removeAttribute("role"); target.removeAttribute("tabindex"); }
        }
      });
    }
    function select(value) {
      if (options.blocked()) return;
      inputs().forEach(function (input) { input.checked = value; });
      options.changed(); update();
    }
    if (button) button.addEventListener("click", function () { if (!options.blocked()) { mode = !mode; update(); } });
    if (all) all.addEventListener("click", function () { select(!inputs().every(function (input) { return input.checked; })); });
    if (clear) clear.addEventListener("click", function () { select(false); if (button) button.focus({ preventScroll: true }); });
    grid.addEventListener("click", function (event) {
      if (!mode) return;
      var card = event.target.closest(options.card);
      if (!card || !grid.contains(card)) return;
      if (event.target.closest("label") || event.target.matches(options.input)) return;
      event.preventDefault(); event.stopImmediatePropagation();
      if (options.blocked()) return;
      var input = card.querySelector(options.input); input.checked = !input.checked;
      options.changed(); update();
    }, true);
    grid.addEventListener("keydown", function (event) {
      if (!mode || options.blocked() || !["Enter", " "].includes(event.key) || event.target.tagName === "BUTTON" || event.target.tagName === "INPUT") return;
      if (!event.target.matches('[role="button"]')) return;
      event.preventDefault(); if (!event.repeat) event.target.click();
    });
    grid.addEventListener("change", function (event) { if (event.target.matches(options.input)) { options.changed(); update(); } });
    update(); return { update: update };
  }
  window.BearStackPeopleControls = { bindSelection: bindSelection };
  var bars = Array.from(document.querySelectorAll(".people-selection-bar"));
  function reserveSelectionSpace() {
    var height = bars.reduce(function (value, bar) { return Math.max(value, bar.hidden ? 0 : bar.getBoundingClientRect().height); }, 0);
    document.documentElement.style.setProperty("--people-selection-height", height + "px");
  }
  if (window.ResizeObserver) {
    var observer = new ResizeObserver(reserveSelectionSpace);
    bars.forEach(function (bar) { observer.observe(bar); });
  }
  reserveSelectionSpace();
  document.querySelectorAll(".people-menu, .people-display-menu, .group-photo-help, .person-detail-more").forEach(function (menu) {
    function fitMenu() {
      if (!menu.open) return;
      var panel = menu.querySelector(":scope > .people-menu-panel, :scope > .people-display-options, :scope > .group-photo-help-content, :scope > .person-detail-menu");
      if (!panel) return;
      panel.style.transform = "";
      var box = panel.getBoundingClientRect(), edge = document.documentElement.clientWidth - 16;
      var shift = box.left < 16 ? 16 - box.left : box.right > edge ? edge - box.right : 0;
      panel.style.transform = "translateX(" + shift + "px)";
    }
    menu.addEventListener("toggle", fitMenu);
    window.addEventListener("resize", fitMenu);
    menu.addEventListener("keydown", function (event) {
      if (event.key === "Escape" && !event.defaultPrevented) { menu.open = false; menu.querySelector("summary").focus(); event.preventDefault(); event.stopPropagation(); }
    });
    document.addEventListener("click", function (event) { if (menu.open && !menu.contains(event.target) && !event.target.closest("dialog")) menu.open = false; });
  });
}());
