(function () {
  "use strict";
  window.BearStackGroupStrip = { bind: function (options) {
    var root = document.querySelector("[data-group-strip]");
    var rail = root.querySelector("[data-group-strip-rail]");
    var status = root.querySelector("[data-group-strip-status]");
    var previous = root.querySelector("[data-group-strip-previous]");
    var next = root.querySelector("[data-group-strip-next]");
    var currentButton = root.querySelector("[data-group-strip-current]");
    var retry = root.querySelector("[data-group-strip-retry]");
    var current = null, threshold, hasPrevious = false, hasNext = false, loading = false;
    var generation = 0, controller, failedDirection, lastLeft = 0, adjusting = false;
    var observer = window.IntersectionObserver ? new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (!entry.isIntersecting) return;
        var img = entry.target;
        img.src = img.dataset.src; observer.unobserve(img);
      });
    }, { root: rail, rootMargin: "0px 200px" }) : null;
    root.hidden = false;
    document.body.classList.add("has-group-strip");

    function items() { return Array.from(rail.children); }
    function selected() { return items().find(function (item) { return current && item.dataset.path === current.path; }); }
    function busy() {
      var blocked = options.isBusy();
      root.setAttribute("aria-busy", String(blocked || loading));
      items().forEach(function (item) { item.setAttribute("aria-disabled", String(blocked)); });
      previous.disabled = blocked || loading || (!hasPrevious && rail.scrollLeft <= 1);
      next.disabled = blocked || loading || (!hasNext && rail.scrollLeft + rail.clientWidth >= rail.scrollWidth - 1);
      currentButton.disabled = blocked || !current;
      retry.disabled = blocked || loading;
    }
    function adjust(fn) {
      adjusting = true; fn(); lastLeft = rail.scrollLeft;
      requestAnimationFrame(function () { adjusting = false; busy(); });
    }
    function mark() {
      items().forEach(function (item) {
        if (current && item.dataset.path === current.path) item.setAttribute("aria-current", "true");
        else item.removeAttribute("aria-current");
      });
    }
    function describe(link, photo) {
      link.title = photo.display_path + " – " + photo.remaining + " unbearbeitete Gesichter";
      link.setAttribute("aria-label", "Gruppenbild öffnen: " + link.title);
    }
    function card(photo) {
      var link = document.createElement("a");
      link.className = "group-strip-item"; link.dataset.path = photo.path;
      link.href = "/photos/people/groups?min=" + options.minimum() + "&path=" + encodeURIComponent(photo.path);
      describe(link, photo);
      var img = document.createElement("img"); img.width = 102; img.height = 62; img.alt = ""; img.decoding = "async";
      img.dataset.src = "/photos/thumbnail?size=160&path=" + encodeURIComponent(photo.path);
      img.addEventListener("error", function () { img.hidden = true; });
      if (observer) observer.observe(img); else { img.loading = "lazy"; img.src = img.dataset.src; }
      var caption = document.createElement("span"); caption.textContent = photo.display_path;
      link.append(img, caption);
      return link;
    }
    function remove(item) { if (observer) observer.unobserve(item.querySelector("img")); item.remove(); }
    async function load(direction) {
      if (loading) return;
      var serial = ++generation;
      controller = new AbortController(); loading = true; failedDirection = direction; busy();
      status.textContent = "Bilder werden geladen …"; retry.hidden = true;
      var address = new URL("/photos/people/groups", location.origin);
      address.searchParams.set("format", "strip"); address.searchParams.set("min", String(options.minimum()));
      if (direction === "before" && rail.firstElementChild) address.searchParams.set("before", rail.firstElementChild.dataset.path);
      else if (direction === "after" && rail.lastElementChild) address.searchParams.set("after", rail.lastElementChild.dataset.path);
      else if (current) address.searchParams.set("path", current.path);
      try {
        var response = await fetch(address, { signal: controller.signal, credentials: "same-origin", redirect: "error", cache: "no-store", headers: { Accept: "application/json" } });
        if (!response.ok) throw new Error("Bilderleiste konnte nicht geladen werden.");
        var data = await response.json();
        if (!Array.isArray(data.photos) || data.photos.length > 33 || data.photos.some(function (p) { return typeof p.path !== "string" || !p.path || typeof p.display_path !== "string"; })) throw new Error("Ungültige Bilderleiste.");
        if (serial !== generation) return;
        var anchor = direction === "before" ? rail.firstElementChild : null;
        var anchorLeft = anchor ? anchor.getBoundingClientRect().left : 0;
        var existing = new Map(items().map(function (item) { return [item.dataset.path, item]; }));
        var additions = data.photos.filter(function (p) { return !direction || !existing.has(p.path); }).map(function (p) { var item = existing.get(p.path) || card(p); describe(item, p); return item; });
        adjust(function () {
          if (!direction) {
            items().forEach(function (item) { if (!additions.includes(item)) remove(item); });
            rail.replaceChildren.apply(rail, additions);
            hasPrevious = data.has_previous; hasNext = data.has_next;
          } else if (direction === "before") {
            rail.prepend.apply(rail, additions); hasPrevious = data.has_previous;
          } else { rail.append.apply(rail, additions); hasNext = data.has_next; }
          // Retain at most three pages, and compensate for removed/prepended width.
          var retained = direction === "after" ? rail.children[Math.max(0, rail.children.length - 96)] : null;
          var retainedLeft = retained ? retained.getBoundingClientRect().left : 0;
          while (rail.children.length > 96) {
            if (direction === "before") { remove(rail.lastElementChild); hasNext = true; }
            else { remove(rail.firstElementChild); hasPrevious = true; }
          }
          if (anchor && anchor.isConnected) rail.scrollLeft += anchor.getBoundingClientRect().left - anchorLeft;
          if (retained && retained.isConnected) rail.scrollLeft += retained.getBoundingClientRect().left - retainedLeft;
          mark();
          if (!direction) { var active = selected(); rail.scrollLeft = active ? active.offsetLeft - rail.offsetLeft - rail.clientWidth / 2 + active.offsetWidth / 2 : 0; }
        });
        status.textContent = rail.children.length ? "Foto anklicken zum Springen" : "Keine passenden Gruppenbilder";
      } catch (error) {
        if (serial === generation && error.name !== "AbortError") { status.textContent = error.message; retry.hidden = false; }
      } finally { if (serial === generation) { loading = false; busy(); } }
    }
    function reset() {
      generation++; if (controller) controller.abort(); loading = false;
      return load("");
    }
    function sync(photo) {
      var same = current && photo && current.path === photo.path;
      current = photo;
      if (threshold !== options.minimum() || !rail.children.length || (photo && !selected()) || same) {
        threshold = options.minimum(); reset();
      } else {
        mark(); var active = selected();
        if (active) adjust(function () { active.scrollIntoView({ block: "nearest", inline: "nearest" }); });
        busy();
      }
    }
    rail.addEventListener("click", function (event) {
      var item = event.target.closest("[data-path]");
      if (!item || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey || event.button) return;
      event.preventDefault();
      if (options.isBusy()) return;
      options.select(item.dataset.path).catch(function () {});
    });
    rail.addEventListener("scroll", function () {
      var direction = rail.scrollLeft < lastLeft ? "before" : "after"; lastLeft = rail.scrollLeft; busy();
      if (adjusting || loading || !retry.hidden) return;
      if (direction === "before" && hasPrevious && rail.scrollLeft < 160) load("before");
      if (direction === "after" && hasNext && rail.scrollLeft + rail.clientWidth > rail.scrollWidth - 160) load("after");
    }, { passive: true });
    async function move(direction) {
      if (options.isBusy()) return;
      if (direction < 0 && hasPrevious && rail.scrollLeft < rail.clientWidth) await load("before");
      else if (direction > 0 && hasNext && rail.scrollLeft + rail.clientWidth * 2 > rail.scrollWidth) await load("after");
      rail.scrollBy({ left: direction * rail.clientWidth * .8, behavior: "auto" });
    }
    rail.addEventListener("wheel", function (event) {
      if (event.ctrlKey || Math.abs(event.deltaY) <= Math.abs(event.deltaX)) return;
      event.preventDefault();
      var delta = event.deltaY * (event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? rail.clientWidth : 1);
      rail.scrollLeft += delta;
      if (!loading && retry.hidden) {
        if (delta > 0 && hasNext && rail.scrollLeft + rail.clientWidth >= rail.scrollWidth - 160) load("after");
        else if (delta < 0 && hasPrevious && rail.scrollLeft < 160) load("before");
      }
    }, { passive: false });
    previous.addEventListener("click", function () { move(-1); });
    next.addEventListener("click", function () { move(1); });
    currentButton.addEventListener("click", function () { if (!options.isBusy()) reset(); });
    retry.addEventListener("click", function () { load(failedDirection); });
    if (window.ResizeObserver) new ResizeObserver(busy).observe(rail);
    return { sync: sync, busy: busy };
  } };
}());
