(function () {
  "use strict";
  if (window.BearStackPeopleControls) return;
  // The same interaction for groups, active faces and ignored faces; bounded to one page.
  function bindSelection(options) {
    var grid = options.grid, mode = false, anchor = null;
    var button = options.button, all = options.all, clear = options.clear;
    function inputs() { return Array.from(grid.querySelectorAll(options.input)); }
    function update() {
      var items = inputs(), count = items.filter(function (input) { return input.checked; }).length;
      var blocked = options.blocked();
      if (anchor && (!grid.contains(anchor) || !anchor.checked)) anchor = null;
      grid.dataset.selectionMode = String(mode);
      if (button) { button.hidden = false; button.disabled = blocked; button.setAttribute("aria-pressed", String(mode)); }
      if (all) { all.hidden = !mode && !count; all.disabled = blocked || !items.length; all.textContent = count && count === items.length ? "Auswahl aufheben" : all.dataset.selectionLabel; }
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
      anchor = null;
      inputs().forEach(function (input) { input.checked = value; });
      options.changed(); update();
    }
    // DOM order is the visible row-by-row grid order; never scan off-page data
    // or measure card geometry. Only an explicitly selected item is an anchor.
    function markRange(input) {
      if (!anchor || !anchor.checked) return false;
      var items = inputs(), start = items.indexOf(anchor), end = items.indexOf(input);
      if (start < 0 || end < 0) return false;
      for (var i = Math.min(start, end); i <= Math.max(start, end); i++) {
        if (!items[i].disabled) items[i].checked = true;
      }
      return true;
    }
    function remember(input) {
      if (input.checked) anchor = input;
      else if (anchor === input) anchor = null;
    }
    function activate(input, shift) {
      if (options.blocked() || input.disabled) return;
      if (!shift || !markRange(input)) input.checked = !input.checked;
      remember(input);
      options.changed(); update();
    }
    if (button) button.addEventListener("click", function () {
      if (!options.blocked()) { mode = !mode; anchor = null; update(); }
    });
    if (all) all.addEventListener("click", function () { select(!inputs().every(function (input) { return input.checked; })); });
    if (clear) clear.addEventListener("click", function () { select(false); if (button) button.focus({ preventScroll: true }); });
    grid.addEventListener("click", function (event) {
      if (!mode) return;
      var card = event.target.closest(options.card);
      if (!card || !grid.contains(card)) return;
      if (event.target.matches(options.input)) {
        // Native checkboxes toggle before click and emit change afterwards.
        // Do not cancel click: the browser would roll back the checked state.
        if (options.blocked()) { event.preventDefault(); return; }
        if (event.shiftKey) markRange(event.target);
        remember(event.target);
        return;
      }
      event.preventDefault(); event.stopImmediatePropagation();
      var input = card.querySelector(options.input);
      activate(input, event.shiftKey);
      if (event.target.closest("label") && !input.disabled) input.focus({ preventScroll: true });
    }, true);
    grid.addEventListener("keydown", function (event) {
      if (!mode || options.blocked() || !["Enter", " "].includes(event.key)) return;
      var card = event.target.closest(options.card);
      if (!card || !grid.contains(card)) return;
      var input = card.querySelector(options.input), target = card.querySelector(options.open) || card;
      if (event.target !== input && event.target !== target) return;
      event.preventDefault(); event.stopImmediatePropagation();
      if (!event.repeat) activate(input, event.shiftKey);
    });
    grid.addEventListener("change", function (event) {
      if (event.target.matches(options.input)) {
        if (mode && !options.blocked()) remember(event.target);
        options.changed(); update();
      }
    });
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
  // Delegate menu events so replaced folder rows work without retaining old DOM.
  var menuSelector = ".people-menu, .people-display-menu, .group-photo-help, .person-detail-more";
  function fitMenu(menu) {
    if (!menu.open) return;
    var panel = menu.querySelector(":scope > .people-menu-panel, :scope > .people-display-options, :scope > .group-photo-help-content, :scope > .person-detail-menu");
    if (!panel) return;
    panel.style.transform = "";
    var box = panel.getBoundingClientRect(), edge = document.documentElement.clientWidth - 16;
    var shift = box.left < 16 ? 16 - box.left : box.right > edge ? edge - box.right : 0;
    panel.style.transform = "translateX(" + shift + "px)";
  }
  document.addEventListener("toggle", function (event) {
    if (event.target.matches(menuSelector)) fitMenu(event.target);
  }, true);
  window.addEventListener("resize", function () {
    document.querySelectorAll(menuSelector).forEach(fitMenu);
  });
  document.addEventListener("keydown", function (event) {
    var menu = event.target.closest(menuSelector);
    if (menu && menu.open && event.key === "Escape" && !event.defaultPrevented) {
      menu.open = false; menu.querySelector("summary").focus(); event.preventDefault(); event.stopPropagation();
    }
  }, true);
  document.addEventListener("click", function (event) {
    if (event.target.closest("dialog")) return;
    document.querySelectorAll(menuSelector).forEach(function (menu) {
      if (menu.open && !menu.contains(event.target)) menu.open = false;
    });
  });
}());
