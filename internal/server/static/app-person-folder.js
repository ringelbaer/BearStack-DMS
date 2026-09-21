(function () {
  "use strict";
  var dialog = document.querySelector("[data-person-dialog]");
  if (!dialog) return;
  var modal = dialog.querySelector("form"), opener, submitting = false;
  dialog.classList.add("person-folder-dialog");
  modal.dataset.personFolderSelection = "";
  dialog.querySelector("#person-dialog-title").textContent = "Gesichter im Ordner zuordnen";
  dialog.querySelector("#overview-person-hint").textContent = "Alle aktiven Gesichter dieser Person im angezeigten Ordner werden zugeordnet, ohne Unterordner. Wähle eine vorhandene Person oder gib einen neuen Namen ein.";
  dialog.querySelectorAll("[data-person-face-match], [data-person-dialog-ignore], [data-person-dialog-unname]").forEach(function (button) { button.hidden = true; });
  var path = document.createElement("p");
  path.dataset.folderDialogPath = "";
  dialog.querySelector(".person-dialog-fields").prepend(path);
  ["directory", "revision", "action"].forEach(function (name) {
    var input = document.createElement("input"); input.type = "hidden"; input.name = name; modal.append(input);
  });
  function close() { if (!submitting) dialog.close(); }
  dialog.querySelector("[data-person-dialog-cancel]").addEventListener("click", close);
  dialog.addEventListener("cancel", function (event) { if (submitting) event.preventDefault(); });
  dialog.addEventListener("close", function () {
    modal.dispatchEvent(new CustomEvent("person-picker-close"));
    if (opener && opener.isConnected) opener.focus({ preventScroll: true });
  });
  modal.addEventListener("submit", function (event) {
    if (submitting) { event.preventDefault(); return; }
    submitting = true;
  });
  document.querySelectorAll("[data-person-folder-form]").forEach(function (form) {
    var button = form.querySelector("[data-folder-move]");
    if (button) {
      button.hidden = false;
      button.addEventListener("click", function () {
        if (submitting) return;
        opener = button;
        modal.setAttribute("action", form.getAttribute("action"));
        modal.dataset.renameAction = form.getAttribute("action");
        ["directory", "revision"].forEach(function (name) { modal.elements.namedItem(name).value = form.elements.namedItem(name).value; });
        modal.elements.namedItem("action").value = "move";
        // The heading is the server-formatted display path, never a raw directory.
        path.textContent = form.closest("[data-person-folder]").querySelector("h2").textContent;
        modal.dispatchEvent(new CustomEvent("person-picker-reset", { detail: { name: "" } }));
        dialog.showModal();
      });
    }
    var confirmed = false;
    form.addEventListener("submit", async function (event) {
      if (confirmed) { confirmed = false; return; }
      event.preventDefault();
      if (submitting) return;
      var action = event.submitter;
      if (!action) return;
      submitting = true;
      var folder = form.closest("[data-person-folder]").querySelector("h2").textContent;
      var message = action.textContent + " in „" + folder + "“? Die Aktion betrifft nur diese Person im exakten Ordner, ohne Unterordner.";
      if (action.value === "exclude") message += " Auch neue Zuordnungen zu dieser Person werden dort gesperrt.";
      if (await showAppConfirm(message, "Ordneraktion bestätigen")) { confirmed = true; form.requestSubmit(action); }
      else submitting = false;
    });
  });
  window.addEventListener("pageshow", function () { submitting = false; });
}());
