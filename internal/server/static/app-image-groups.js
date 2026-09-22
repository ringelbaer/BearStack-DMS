(function () {
  "use strict";
  var dialog = document.querySelector("[data-image-group-dialog]");
  var opener;
  if (dialog) {
    var create = document.querySelector("[data-image-group-create]");
    var form = document.querySelector("[data-photo-bulk-form]");
    function selected() { return Array.from(form.querySelectorAll('input[name="ids"]:checked')).map(function (input) { return input.closest("[data-photo-item]"); }); }
    function eligible(items) { return items.length >= 2 && items.length <= 500 && items.every(function (item) { return item.dataset.photoType === "image" && !Number(item.dataset.imageGroupId); }); }
    function update() {
      var items = selected(), hint = document.querySelector("[data-image-group-hint]");
      create.disabled = !eligible(items);
      if (!hint) return;
      hint.textContent = items.length > 500 ? "Bitte höchstens 500 Bilder auswählen." :
        items.some(function (item) { return Number(item.dataset.imageGroupId); }) ? "Die Auswahl enthält bereits gruppierte Bilder. Öffne deren Gruppensymbol, um die bestehende Gruppe zu ändern." :
        items.some(function (item) { return item.dataset.photoType !== "image"; }) ? "Bildgruppen können nur Bilder enthalten. Bitte Videos und Audios abwählen." :
        "Gruppieren ist für mindestens zwei ungruppierte Bilder möglich.";
    }
    if (create && form) {
      create.hidden = false;
      form.addEventListener("photo-selection-changed", update);
      create.addEventListener("click", function () {
        var items = selected(); if (!eligible(items)) return;
        opener = create;
        var select = dialog.querySelector("[data-image-group-primary]"), inputs = dialog.querySelector("[data-image-group-inputs]");
        select.replaceChildren(); inputs.replaceChildren();
        items.forEach(function (item) {
          var option = document.createElement("option"); option.value = item.dataset.photoPath; option.textContent = item.dataset.photoDisplayPath; select.append(option);
          var input = document.createElement("input"); input.type = "hidden"; input.name = "ids"; input.value = item.dataset.photoPath; inputs.append(input);
        });
        dialog.querySelector("[data-image-group-summary]").textContent = items.length + " ausgewählte Bilder werden zu einer Bildgruppe zusammengefasst.";
        dialog.showModal(); select.focus();
      });
      dialog.querySelector("[data-image-group-cancel]").addEventListener("click", function () { dialog.close(); });
      dialog.addEventListener("close", function () { if (opener) opener.focus(); });
      update();
    }
  }
  document.querySelectorAll("[data-image-group-confirm]").forEach(function (form) {
    var approved = false, pending = false;
    form.addEventListener("submit", async function (event) {
      if (approved) return;
      event.preventDefault(); if (pending) return;
      pending = true;
      try {
        if (await showAppConfirm(form.dataset.imageGroupConfirm, "Bildgruppe ändern")) { approved = true; form.requestSubmit(event.submitter); }
      } finally { pending = false; }
    });
  });
}());
