(function () {
  "use strict";
  var dialog = document.querySelector("[data-person-dialog]");
  var content = document.querySelector("[data-person-folder-content]");
  if (!dialog || !content) return;
  var modal = dialog.querySelector("form"), opener, submitting = false, stale = false;
  var status = document.querySelector("[data-folder-status]");
  var refreshButton = document.querySelector("[data-folder-refresh]");
  var modalStatus = dialog.querySelector("[data-person-dialog-status]");
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
  function prepare() {
    content.querySelectorAll("[data-folder-move]").forEach(function (button) { button.hidden = false; });
    content.querySelectorAll("button").forEach(function (button) { button.disabled = submitting || stale; });
    content.setAttribute("aria-busy", String(submitting));
    modal.querySelectorAll("input, button").forEach(function (control) { control.disabled = submitting; });
    modal.querySelector("[data-person-submit]").disabled = submitting || stale;
    refreshButton.disabled = submitting;
  }
  function message(text) { status.textContent = text; status.hidden = !text; }
  async function request(url, options) {
    var controller = new AbortController();
    var timer = setTimeout(function () { controller.abort(); }, 30000);
    try {
      var response = await fetch(url, Object.assign({ credentials: "same-origin", redirect: "error", cache: "no-store", signal: controller.signal }, options));
      var body = await response.text();
      if (!response.ok) {
        var error = new Error(response.status === 409 ? "Die Ordneransicht wurde inzwischen geändert." : "Anfrage fehlgeschlagen (HTTP " + response.status + ").");
        throw error;
      }
      return body;
    } finally { clearTimeout(timer); }
  }
  async function refresh() {
    var url = new URL(window.location.href);
    url.searchParams.set("format", "fragment");
    var html = await request(url, { headers: { Accept: "text/html" } });
    var next = new DOMParser().parseFromString(html, "text/html").querySelector("[data-person-folder-content]");
    if (!next) throw new Error("Die Ordneransicht konnte nicht geladen werden.");
    content.replaceWith(next); content = next;
    if (Number(next.dataset.revision) > 0) {
      document.querySelector("[data-folder-heading]").textContent = "Ordner: " + (next.dataset.personName || "Unbenannt");
    }
    // The server falls back to page one if the current last page disappeared.
    var current = new URL(window.location.href);
    if (next.dataset.page === "1") current.searchParams.delete("page");
    else current.searchParams.set("page", next.dataset.page);
    history.replaceState(history.state, "", current);
    stale = false; refreshButton.hidden = true;
  }
  async function save(form, action) {
    var data = new URLSearchParams(new FormData(form));
    if (action) data.set("action", action);
    submitting = true; prepare(); message("Ordneraktion wird gespeichert …"); modalStatus.textContent = "";
    var saved = false;
    try {
      var result = JSON.parse(await request(form.getAttribute("action"), {
        method: "POST", headers: { Accept: "application/json" }, body: data
      }));
      if (!result.ok) throw new Error("Speichern konnte nicht bestätigt werden.");
      saved = true;
      await refresh();
      dialog.close();
      message("Ordneraktion gespeichert.");
      status.focus({ preventScroll: true });
    } catch (error) {
      stale = true; refreshButton.hidden = false;
      var text = saved ? "Ordneraktion gespeichert, aber die Ansicht konnte nicht aktualisiert werden." : "Speichern konnte nicht bestätigt werden. " + (error.name === "AbortError" ? "Zeitlimit erreicht." : error.message);
      text += " Bitte die Ansicht aktualisieren, bevor du eine weitere Aktion wählst.";
      message(text); modalStatus.textContent = text;
      if (saved) dialog.close();
    } finally { submitting = false; prepare(); }
  }
  function close() { if (!submitting) dialog.close(); }
  dialog.querySelector("[data-person-dialog-cancel]").addEventListener("click", close);
  dialog.addEventListener("cancel", function (event) { if (submitting) event.preventDefault(); });
  dialog.addEventListener("close", function () {
    modal.dispatchEvent(new CustomEvent("person-picker-close"));
    if (opener && opener.isConnected && !opener.disabled) opener.focus({ preventScroll: true });
    opener = null;
  });
  modal.addEventListener("submit", function (event) {
    event.preventDefault();
    if (!submitting && !stale) save(modal);
  });
  document.addEventListener("click", function (event) {
    var button = event.target.closest("[data-folder-move]");
    if (!button || !content.contains(button) || submitting || stale) return;
    var form = button.closest("form");
    opener = button;
    modal.setAttribute("action", form.getAttribute("action"));
    modal.dataset.renameAction = form.getAttribute("action");
    ["directory", "revision"].forEach(function (name) { modal.elements.namedItem(name).value = form.elements.namedItem(name).value; });
    modal.elements.namedItem("action").value = "move";
    // Always use the server-formatted path, including refreshed/new rows.
    path.textContent = form.closest("[data-person-folder]").querySelector("h2").textContent;
    modalStatus.textContent = "";
    modal.dispatchEvent(new CustomEvent("person-picker-reset", { detail: { name: "" } }));
    dialog.showModal();
  });
  document.addEventListener("submit", async function (event) {
    var form = event.target;
    if (!form.matches("[data-person-folder-form]")) return;
    event.preventDefault();
    if (submitting || stale || !content.contains(form) || !event.submitter) return;
    var action = event.submitter, value = action.value;
    submitting = true; prepare();
    var folder = form.closest("[data-person-folder]").querySelector("h2").textContent;
    var text = action.textContent + " in „" + folder + "“? Die Aktion betrifft nur diese Person im exakten Ordner, ohne Unterordner.";
    if (value === "exclude") text += " Auch neue Zuordnungen zu dieser Person werden dort gesperrt.";
    try {
      if (await showAppConfirm(text, "Ordneraktion bestätigen")) {
        // Restore controls before reading FormData; disabled inputs are omitted.
        submitting = false; prepare();
        await save(form, value);
      }
    } finally { submitting = false; prepare(); }
  });
  refreshButton.addEventListener("click", async function () {
    if (submitting) return;
    submitting = true; prepare();
    try {
      await refresh(); dialog.close(); message("Ordneransicht aktualisiert. Bitte wähle die gewünschte Aktion erneut.");
    } catch (error) { message("Die Ansicht konnte nicht aktualisiert werden. Bitte erneut versuchen."); }
    finally { submitting = false; prepare(); }
  });
  prepare();
}());
