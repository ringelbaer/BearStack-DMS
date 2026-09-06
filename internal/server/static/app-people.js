(function () {
  "use strict";
  document.querySelectorAll("[data-person-picker]").forEach(function (form) {
    var input = form.querySelector("[data-person-search]");
    var select = form.querySelector("[data-person-options]");
    var timer, controller;
    function load() {
      if (controller) controller.abort();
      controller = new AbortController();
      fetch("/photos/people?format=json&q=" + encodeURIComponent(input.value), {signal: controller.signal, credentials: "same-origin"})
        .then(function (response) { if (!response.ok) throw new Error("Personen konnten nicht geladen werden"); return response.json(); })
        .then(function (data) {
          while (select.options.length > 1) select.remove(1);
          (data.people || []).forEach(function (person) {
            var option = document.createElement("option"); option.value = person.id;
            option.textContent = (person.name || "Unbenannt") + " (#" + person.id + ", " + person.count + (person.count === 1 ? " Foto)" : " Fotos)");
            select.appendChild(option);
          });
          input.setCustomValidity("");
        }).catch(function (error) { if (error.name !== "AbortError") input.setCustomValidity(error.message); });
    }
    input.addEventListener("input", function () { clearTimeout(timer); timer = setTimeout(load, 250); });
    load();
  });
}());
