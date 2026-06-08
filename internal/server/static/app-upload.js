function uploadFiles(files) {
  if (!files || files.length === 0) return;

  const formData = new FormData();
  Array.from(files).forEach((file) => formData.append("files", file));

  const request = new XMLHttpRequest();
  request.open("POST", "/upload");
  request.setRequestHeader("Accept", "application/json");
  request.setRequestHeader("X-Requested-With", "XMLHttpRequest");
  showUploadStatus(`${files.length} Datei(en) werden hochgeladen`);
  setUploadProgress(0);

  request.upload.addEventListener("progress", (event) => {
    if (event.lengthComputable) {
      setUploadProgress((event.loaded / event.total) * 100);
    }
  });

  request.addEventListener("load", () => {
    setUploadProgress(100);
    let payload = null;
    try {
      payload = JSON.parse(request.responseText);
    } catch {
      addUploadLine("Upload abgeschlossen, Antwort konnte nicht gelesen werden");
      return;
    }

    const uploaded = payload.uploaded || [];
    const duplicates = payload.duplicates || [];
    const errors = payload.errors || [];
    uploadMessage.textContent = `${uploaded.length} hochgeladen, ${duplicates.length} Duplikat(e), ${errors.length} Fehler`;
    uploaded.forEach((item) => addUploadLine(`Hochgeladen: ${item.filename}`));
    duplicates.forEach((item) => addUploadLine(`Duplikat übersprungen: ${item.filename}`));
    errors.forEach((item) => addUploadLine(`Fehler: ${item.filename || "Datei"} - ${item.error}`));

    if (uploaded.length > 0 || duplicates.length > 0) {
      window.setTimeout(async () => {
        try {
          if (await refreshDocumentList()) return;
        } catch {
          // Fall back to a full reload when the list refresh cannot be completed.
        }
        window.location.reload();
      }, 900);
    }
  });

  request.addEventListener("error", () => {
    uploadMessage.textContent = "Upload fehlgeschlagen";
    addUploadLine("Netzwerkfehler beim Hochladen");
  });

  request.send(formData);
}

document.querySelectorAll("[data-upload-form]").forEach((form) => {
  const input = form.querySelector('input[type="file"]');

  input?.addEventListener("change", () => {
    if (!input.files || input.files.length === 0) return;
    uploadFiles(input.files);
    input.value = "";
  });

  form.addEventListener("submit", (event) => {
    if (!input || !input.files || input.files.length === 0) return;
    event.preventDefault();
    uploadFiles(input.files);
    input.value = "";
  });
});

let dragDepth = 0;

window.addEventListener("dragenter", (event) => {
  if (!event.dataTransfer || !Array.from(event.dataTransfer.types).includes("Files")) return;
  dragDepth += 1;
  document.body.classList.add("dragging");
});

window.addEventListener("dragover", (event) => {
  if (!event.dataTransfer || !Array.from(event.dataTransfer.types).includes("Files")) return;
  event.preventDefault();
});

window.addEventListener("dragleave", () => {
  dragDepth = Math.max(0, dragDepth - 1);
  if (dragDepth === 0) {
    document.body.classList.remove("dragging");
  }
});

window.addEventListener("drop", (event) => {
  if (!event.dataTransfer || event.dataTransfer.files.length === 0) return;
  event.preventDefault();
  dragDepth = 0;
  document.body.classList.remove("dragging");
  uploadFiles(event.dataTransfer.files);
});

window.BearStack = window.BearStack || {};
window.BearStack.upload = Object.assign(window.BearStack.upload || {}, {
  uploadFiles,
});
