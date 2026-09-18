(function () {
  "use strict";
  document.querySelectorAll("[data-person-folder-form]").forEach(function (form) {
    var element = form.querySelector("[data-person-picker]");
    var picker = element && window.BearStackPersonPicker.bind(element, { allowCreate: true });
    form.addEventListener("submit", function (event) {
      if (event.submitter && event.submitter.value === "move" && picker && !picker.selection().name) {
        event.preventDefault(); element.querySelector("input").focus(); return;
      }
      if (form.dataset.submitting) { event.preventDefault(); return; }
      form.dataset.submitting = "true";
    });
  });
}());
