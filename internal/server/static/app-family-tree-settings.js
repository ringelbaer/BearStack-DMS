(function () {
  "use strict";
  const form = document.querySelector("[data-family-tree-settings]");
  if (!form) return;
  const list = form.querySelector("[data-tree-roots]"), status = form.querySelector("[data-tree-settings-status]");
  function updated() { status.textContent = list.children.length ? list.children.length + " Ausgangspersonen ausgewählt." : "Keine Ausgangsperson ausgewählt. Nach dem Speichern ist die Stammbaumansicht deaktiviert."; }
  const picker = window.BearStackPersonPicker.bind(form.querySelector("[data-person-picker]"), {
    onChoose(selection) {
      if (!selection.assigned) return;
      if (Array.from(list.children).some(row => row.dataset.rootId === selection.target)) { status.textContent = "Diese Person ist bereits ausgewählt."; picker.reset(); return; }
      if (list.children.length >= 200) { status.textContent = "Höchstens 200 Ausgangspersonen möglich."; return; }
      const row = document.createElement("li"), input = document.createElement("input"), name = document.createElement("span"), button = document.createElement("button");
      row.dataset.rootId = selection.target; input.type = "hidden"; input.name = "person_id"; input.value = selection.target;
      name.textContent = selection.targetName; button.type = "button"; button.className = "secondary-button"; button.dataset.treeRemove = "";
      button.textContent = "Entfernen"; button.setAttribute("aria-label", selection.targetName + " entfernen");
      row.append(input, name, button); list.append(row); picker.reset(); updated();
    }
  });
  list.addEventListener("click", event => { const button = event.target.closest("[data-tree-remove]"); if (button) { button.closest("li").remove(); updated(); } });
  form.querySelector("[data-tree-clear]").addEventListener("click", () => { list.replaceChildren(); picker.reset(); updated(); });
  form.querySelector("[data-person-search]").addEventListener("keydown", event => { if (event.key === "Enter") event.preventDefault(); });
  updated();
}());
