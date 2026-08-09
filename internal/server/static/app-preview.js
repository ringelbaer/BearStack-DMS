const previewModal = document.querySelector("[data-preview-modal]");
const previewTitle = previewModal?.querySelector("[data-preview-title]");
const previewFrame = document.querySelector("[data-preview-frame]");
const previewImage = document.querySelector("[data-preview-image]");
const previewClose = document.querySelector("[data-preview-close]");
const sidePreview = document.querySelector("[data-side-preview]");
const sidePreviewTitle = document.querySelector("[data-side-preview-title]");
const sidePreviewFrame = document.querySelector("[data-side-preview-frame]");
const sidePreviewImage = document.querySelector("[data-side-preview-image]");
const sidePreviewClose = document.querySelector("[data-side-preview-close]");
const columnsModal = document.querySelector("[data-columns-modal]");
const columnsClose = document.querySelector("[data-columns-close]");
const customPDFPreviewEnabled = document.body?.dataset.customPdfPreview === "true";

function pdfPreviewForFrame(frame) {
  return frame?.parentElement?.querySelector("[data-pdf-preview]") || null;
}

function resetPreviewTargets(frame, image) {
  if (frame) {
    frame.hidden = true;
    frame.removeAttribute("src");
  }
  if (image) {
    image.hidden = true;
    image.removeAttribute("src");
    image.alt = "";
  }
  const pdfPreview = pdfPreviewForFrame(frame);
  if (pdfPreview && window.bearStackPDFPreview) {
    window.bearStackPDFPreview.destroy(pdfPreview);
  }
}

function loadPreviewTarget(frame, image, url, mime, title) {
  resetPreviewTargets(frame, image);
  if (mime.startsWith("image/") && image) {
    image.src = url;
    image.alt = title;
    image.hidden = false;
    return;
  }
  const pdfPreview = pdfPreviewForFrame(frame);
  if (customPDFPreviewEnabled && pdfPreview && window.bearStackPDFPreview) {
    const downloadURL = url.replace(/\/preview(?:\?.*)?$/, "/download");
    window.bearStackPDFPreview.load(pdfPreview, url, title, downloadURL).catch(() => {
      window.bearStackPDFPreview.destroy(pdfPreview);
      if (frame) {
        frame.src = url;
        frame.hidden = false;
      }
      if (typeof window.showAppToast === "function") {
        window.showAppToast("BearStack-Vorschau nicht verfügbar; Browser-Viewer wird verwendet.");
      }
    });
    return;
  }
  if (frame) {
    frame.src = url;
    frame.hidden = false;
  }
}

function isDesktopPreviewViewport() {
  return window.matchMedia("(min-width: 961px)").matches;
}

function shouldUseSidePreview() {
  return Boolean(sidePreview && sidePreviewFrame && sidePreviewImage && isDesktopPreviewViewport());
}

function clearPreviewActiveRow() {
  document.querySelectorAll(".document-preview-active").forEach((row) => {
    row.classList.remove("document-preview-active");
  });
}

function markPreviewActiveRow(button) {
  clearPreviewActiveRow();
  button.closest("tr[data-document-id]")?.classList.add("document-preview-active");
}

function documentIDForPreviewControl(control) {
  return control.closest("tr[data-document-id]")?.dataset.documentId || "";
}

function currentSidePreviewDocumentID() {
  return sidePreview?.dataset.documentId || document.querySelector(".document-preview-active[data-document-id]")?.dataset.documentId || "";
}

function openModalPreview(url, mime, title) {
  if (!previewModal || !previewFrame || !previewImage) return;
  clearPreviewActiveRow();
  if (previewTitle) {
    previewTitle.textContent = title || "Vorschau";
  }
  loadPreviewTarget(previewFrame, previewImage, url, mime, title || "");
  if (typeof previewModal.showModal === "function") {
    previewModal.showModal();
    document.documentElement.classList.add("preview-modal-open");
    document.body.classList.add("preview-modal-open");
  }
}

function openSidePreview(url, mime, title, documentID = "") {
  if (!sidePreview || !sidePreviewFrame || !sidePreviewImage) return;
  sidePreview.dataset.documentId = documentID;
  if (sidePreviewTitle) {
    sidePreviewTitle.textContent = title || "Vorschau";
  }
  loadPreviewTarget(sidePreviewFrame, sidePreviewImage, url, mime, title || "");
  sidePreview.classList.remove("hidden");
  document.querySelector("main.page")?.classList.add("side-preview-open");
}

function closeSidePreview() {
  sidePreview?.classList.add("hidden");
  if (sidePreview) {
    delete sidePreview.dataset.documentId;
  }
  document.querySelector("main.page")?.classList.remove("side-preview-open");
  resetPreviewTargets(sidePreviewFrame, sidePreviewImage);
  clearPreviewActiveRow();
}

function openSidePreviewForListControl(control) {
  if (!shouldUseSidePreview()) return;
  if (sidePreview.classList.contains("hidden")) return;
  const row = control.closest("tr[data-document-id]");
  const documentID = row?.dataset.documentId || "";
  if (documentID && currentSidePreviewDocumentID() === documentID) return;
  const preview = row?.querySelector("[data-preview-url]");
  const url = preview?.dataset.previewUrl;
  if (!url) return;
  openSidePreview(url, preview.dataset.previewMime || "", preview.dataset.previewTitle || "Vorschau", documentID);
  if (preview) {
    markPreviewActiveRow(preview);
  }
}

function initializePreviewButtons(root = document) {
  initializeOnce(root, "[data-preview-url]", (button) => {
    button.addEventListener("click", () => {
      const url = button.dataset.previewUrl;
      const mime = button.dataset.previewMime || "";
      const title = button.dataset.previewTitle || "Vorschau";
      if (!url) return;

      if (shouldUseSidePreview()) {
        openSidePreview(url, mime, title, documentIDForPreviewControl(button));
        markPreviewActiveRow(button);
        return;
      }
      openModalPreview(url, mime, title);
    });
  });
}

function initializeDetailPreview(root = document) {
  initializeOnce(root, "[data-detail-preview]", (target) => {
    const frame = target.querySelector("[data-detail-preview-frame]");
    const url = target.dataset.detailPreviewUrl || "";
    if (!frame || !url) return;
    loadPreviewTarget(
      frame,
      null,
      url,
      target.dataset.detailPreviewMime || "",
      target.dataset.detailPreviewTitle || "Vorschau"
    );
  });
}

function absoluteShareURL(rawURL) {
  if (!rawURL) return "";
  try {
    return new URL(rawURL, window.location.origin).toString();
  } catch {
    return "";
  }
}

async function copyTextToClipboard(value) {
  if (!value) throw new Error("Kein Link verfügbar");
  if (navigator?.clipboard?.writeText) {
    await navigator.clipboard.writeText(value);
    return;
  }
  const fallback = document.createElement("textarea");
  fallback.value = value;
  fallback.setAttribute("readonly", "readonly");
  fallback.style.position = "fixed";
  fallback.style.opacity = "0";
  fallback.style.pointerEvents = "none";
  document.body.append(fallback);
  fallback.focus();
  fallback.select();
  const copied = typeof document.execCommand === "function" ? document.execCommand("copy") : false;
  fallback.remove();
  if (!copied) {
    throw new Error("Zwischenablage nicht verfügbar");
  }
}

function initializeShareButtons(root = document) {
  initializeOnce(root, "[data-share-url]", (button) => {
    button.addEventListener("click", async () => {
      const shareURL = absoluteShareURL(button.dataset.shareUrl || "");
      if (!shareURL) {
        showAppAlert("Sharing-Link konnte nicht erzeugt werden.");
        return;
      }
      button.disabled = true;
      try {
        await copyTextToClipboard(shareURL);
        if (typeof window.showAppToast === "function") {
          window.showAppToast("Sharing-Link kopiert.");
        } else {
          showAppAlert("Sharing-Link kopiert.");
        }
      } catch {
        showAppAlert("Sharing-Link konnte nicht in die Zwischenablage kopiert werden.");
      } finally {
        button.disabled = false;
      }
    });
  });
}

function initializeDocumentThumbnails(root = document) {
  root.querySelectorAll("img.document-thumb").forEach((img) => {
    const holder = img.closest(".thumb-link") || img;
    const markLoaded = () => {
      holder.classList.remove("is-loading");
    };
    const markError = () => {
      holder.classList.remove("is-loading");
      holder.classList.add("is-error");
    };
    if (img.complete && img.naturalWidth !== 0) {
      markLoaded();
    } else {
      holder.classList.add("is-loading");
      img.addEventListener("load", markLoaded, { once: true });
      img.addEventListener("error", markError, { once: true });
    }
  });
}

sidePreviewClose?.addEventListener("click", () => {
  closeSidePreview();
});

window.addEventListener("resize", () => {
  if (!sidePreview || isDesktopPreviewViewport()) return;
  closeSidePreview();
});

if (previewClose && previewModal) {
  previewClose.addEventListener("click", () => {
    previewModal.close();
  });
}

if (previewModal) {
  previewModal.addEventListener("click", (event) => {
    if (event.target === previewModal) {
      previewModal.close();
    }
  });

  previewModal.addEventListener("close", () => {
    document.documentElement.classList.remove("preview-modal-open");
    document.body.classList.remove("preview-modal-open");
    resetPreviewTargets(previewFrame, previewImage);
  });
}

function checkedColumns() {
  return Array.from(document.querySelectorAll("[data-column-toggle]"))
    .filter((item) => item.checked)
    .map((item) => item.value);
}

function applyColumns(columns) {
  const visible = new Set(columns);
  document.querySelectorAll("[data-column]").forEach((cell) => {
    cell.hidden = !visible.has(cell.dataset.column);
  });
  document.querySelectorAll("[data-column-toggle]").forEach((checkbox) => {
    checkbox.checked = visible.has(checkbox.value);
  });
}

function syncColumnTogglesFromTable() {
  const cells = document.querySelectorAll("[data-column]");
  if (cells.length === 0) return;
  const visible = new Set(
    Array.from(cells)
      .filter((cell) => !cell.hidden)
      .map((cell) => cell.dataset.column)
  );
  document.querySelectorAll("[data-column-toggle]").forEach((checkbox) => {
    checkbox.checked = visible.has(checkbox.value);
  });
}

function columnDragAfterElement(list, y) {
  const items = Array.from(list.querySelectorAll("[data-column-sort-item]:not(.dragging)"));
  return items.reduce(
    (closest, item) => {
      const box = item.getBoundingClientRect();
      const offset = y - box.top - box.height / 2;
      if (offset < 0 && offset > closest.offset) {
        return { offset, item };
      }
      return closest;
    },
    { offset: Number.NEGATIVE_INFINITY, item: null }
  ).item;
}

function moveColumnDragItem(list, item, y) {
  const after = columnDragAfterElement(list, y);
  if (after) {
    list.insertBefore(item, after);
  } else {
    list.append(item);
  }
  updateColumnMoveButtons(list);
}

function moveColumnItemByDirection(list, item, direction) {
  if (direction === "up" && item.previousElementSibling) {
    list.insertBefore(item, item.previousElementSibling);
  } else if (direction === "down" && item.nextElementSibling) {
    list.insertBefore(item.nextElementSibling, item);
  }
  updateColumnMoveButtons(list);
}

function updateColumnMoveButtons(list) {
  const items = Array.from(list.querySelectorAll("[data-column-sort-item]"));
  items.forEach((item, index) => {
    item.querySelectorAll("[data-column-move]").forEach((button) => {
      button.disabled =
        (button.dataset.columnMove === "up" && index === 0) ||
        (button.dataset.columnMove === "down" && index === items.length - 1);
    });
  });
}

function startColumnPointerDrag(list, item, event) {
  if (event.isPrimary === false || (event.button !== undefined && event.button !== 0)) return;
  event.preventDefault();
  item.classList.add("dragging");
  list.classList.add("column-sort-list-dragging");
  try {
    item.setPointerCapture(event.pointerId);
  } catch {
    // Some older WebViews do not support pointer capture on reparented nodes.
  }

  const cleanup = () => {
    item.classList.remove("dragging");
    list.classList.remove("column-sort-list-dragging");
    document.removeEventListener("pointermove", onMove);
    document.removeEventListener("pointerup", cleanup);
    document.removeEventListener("pointercancel", cleanup);
  };

  const onMove = (moveEvent) => {
    if (moveEvent.pointerId !== event.pointerId) return;
    moveEvent.preventDefault();
    moveColumnDragItem(list, item, moveEvent.clientY);
  };

  document.addEventListener("pointermove", onMove);
  document.addEventListener("pointerup", cleanup);
  document.addEventListener("pointercancel", cleanup);
}

function initializeColumnSortList(list) {
  if (list.dataset.columnSortInitialized === "true") return;
  list.dataset.columnSortInitialized = "true";

  list.querySelectorAll("[data-column-sort-item]").forEach((item) => {
    item.addEventListener("dragstart", (event) => {
      item.classList.add("dragging");
      event.dataTransfer.effectAllowed = "move";
      event.dataTransfer.setData("text/plain", item.querySelector("[data-column-toggle]")?.value || "");
    });
    item.addEventListener("dragend", () => {
      item.classList.remove("dragging");
    });

    item.querySelector("[data-column-drag-handle]")?.addEventListener("pointerdown", (event) => {
      startColumnPointerDrag(list, item, event);
    });

    item.querySelectorAll("[data-column-move]").forEach((button) => {
      button.addEventListener("click", () => {
        moveColumnItemByDirection(list, item, button.dataset.columnMove || "");
        button.focus();
      });
    });
  });

  list.addEventListener("dragover", (event) => {
    event.preventDefault();
    const dragging = list.querySelector(".dragging");
    if (!dragging) return;
    moveColumnDragItem(list, dragging, event.clientY);
  });

  updateColumnMoveButtons(list);
}

document.querySelectorAll("[data-column-sort-list]").forEach(initializeColumnSortList);

if (document.querySelector("[data-column-toggle]")) {
  applyColumns(checkedColumns());
}

function initializeColumnOpenButtons(root = document) {
  if (!columnsModal) return;
  initializeOnce(root, "[data-columns-open]", (button) => {
    button.addEventListener("click", () => {
      syncColumnTogglesFromTable();
      if (typeof columnsModal.showModal === "function") {
        columnsModal.showModal();
      }
    });
  });
}

function initializeBulkFieldButtons(root = document) {
  initializeOnce(root, "[data-bulk-fields-open]", (button) => {
    button.addEventListener("click", () => {
      const form = button.closest("form");
      const selected = form?.querySelectorAll('input[name="ids"]:checked').length || 0;
      if (selected === 0) {
        showAppAlert("Bitte mindestens ein Dokument auswählen.");
        return;
      }
      const modal = form?.querySelector("[data-bulk-fields-modal]");
      if (modal && typeof modal.showModal === "function") {
        modal.showModal();
      }
    });
  });

  initializeOnce(root, "[data-bulk-fields-close]", (button) => {
    button.addEventListener("click", () => {
      button.closest("[data-bulk-fields-modal]")?.close();
    });
  });
}

if (columnsClose && columnsModal) {
  columnsClose.addEventListener("click", () => columnsModal.close());
}

function initializeDocumentList(root = document) {
  initializeSubmitPrompts(root);
  if (typeof initializeSearchFavoriteYearFields === "function") {
    initializeSearchFavoriteYearFields(root);
  }
  if (typeof initializeDocumentDateCharts === "function") {
    initializeDocumentDateCharts(root);
  }
  if (typeof initializeTagTimelines === "function") {
    initializeTagTimelines(root);
  }
  initializeSelectionControls(root);
  initializeDocumentDateInputs(root);
  initializeCustomFieldSuggestions(root);
  if (typeof initializeTagSelects === "function") {
    initializeTagSelects(root);
  }
  initializeDocumentThumbnails(root);
  initializePreviewButtons(root);
  initializeDetailPreview(root);
  initializeShareButtons(root);
  initializeColumnOpenButtons(root);
  initializeBulkFieldButtons(root);
  if (document.querySelector("[data-column-toggle]")) {
    applyColumns(checkedColumns());
  }
}

initializeDocumentList();

const highlightedDocument = document.querySelector(".document-highlight");
if (highlightedDocument) {
  highlightedDocument.scrollIntoView({ block: "center" });
}

initializeTabs(document);
