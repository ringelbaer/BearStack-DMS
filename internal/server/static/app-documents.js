const documentSelectionControllers = new WeakMap();
const documentBatchMenuMedia =
  typeof window.matchMedia === "function" ? window.matchMedia("(max-width: 960px)") : null;
const documentExportDownloadWaiters = new WeakMap();
const exportDownloadCookieName = "bearstack_export_download";
const exportDownloadTokenFieldName = "download_token";
let documentBatchMenuMediaInitialized = false;

function documentSelectionController(form) {
  let controller = documentSelectionControllers.get(form);
  if (!controller) {
    controller = createSelectionController(form);
    documentSelectionControllers.set(form, controller);
  }
  return controller;
}

function selectedDocumentCount(form) {
  return documentSelectionController(form).selectedCount();
}

function documentSubmitAction(form, submitter) {
  return submitter?.getAttribute("formaction") || submitter?.formAction || form.getAttribute("action") || form.action || "";
}

function isDocumentExportSubmit(form, submitter) {
  const action = documentSubmitAction(form, submitter);
  if (!action) return false;
  try {
    return new URL(action, window.location.href).pathname === "/export";
  } catch {
    return action === "/export" || action.endsWith("/export");
  }
}

function createExportDownloadToken() {
  const randomValues = new Uint32Array(2);
  if (window.crypto && typeof window.crypto.getRandomValues === "function") {
    window.crypto.getRandomValues(randomValues);
  } else {
    randomValues[0] = Math.floor(Math.random() * 0xffffffff);
    randomValues[1] = Math.floor(Math.random() * 0xffffffff);
  }
  return `bs-${Date.now().toString(36)}-${randomValues[0].toString(36)}-${randomValues[1].toString(36)}`;
}

function exportDownloadCookieValue() {
  return document.cookie
    .split(";")
    .map((part) => part.trim())
    .find((part) => part.startsWith(`${exportDownloadCookieName}=`))
    ?.slice(exportDownloadCookieName.length + 1) || "";
}

function clearExportDownloadCookie() {
  document.cookie = `${exportDownloadCookieName}=; Max-Age=0; Path=/; SameSite=Lax`;
}

function setExportDownloadToken(form, token) {
  let input = form.querySelector(`input[type="hidden"][name="${exportDownloadTokenFieldName}"]`);
  if (!input) {
    input = document.createElement("input");
    input.type = "hidden";
    input.name = exportDownloadTokenFieldName;
    form.append(input);
  }
  input.value = token;
}

function clearDocumentExportDownloadWait(form) {
  const waiter = documentExportDownloadWaiters.get(form);
  if (!waiter) return;
  window.clearInterval(waiter.interval);
  window.clearTimeout(waiter.timeout);
  documentExportDownloadWaiters.delete(form);
}

function waitForDocumentExportDownload(form) {
  const token = createExportDownloadToken();
  setExportDownloadToken(form, token);
  clearDocumentExportDownloadWait(form);

  const finish = (matched) => {
    clearDocumentExportDownloadWait(form);
    if (matched) {
      clearExportDownloadCookie();
    }
    clearBatchBusy(form);
  };
  const interval = window.setInterval(() => {
    if (exportDownloadCookieValue() === token) {
      finish(true);
    }
  }, 250);
  const timeout = window.setTimeout(() => finish(false), 5 * 60 * 1000);
  documentExportDownloadWaiters.set(form, { interval, timeout });
}

function setDocumentBatchMenuOpen(menu, open) {
  if (open) {
    menu.setAttribute("open", "");
  } else {
    menu.removeAttribute("open");
  }
  const toggle = menu.querySelector("[data-document-batch-menu-toggle]");
  if (toggle) {
    toggle.setAttribute("aria-expanded", open ? "true" : "false");
  }
}

function syncDocumentBatchMenu(menu) {
  const compact = documentBatchMenuMedia && documentBatchMenuMedia.matches;
  if (!compact) {
    setDocumentBatchMenuOpen(menu, false);
    return;
  }
  setDocumentBatchMenuOpen(menu, menu.hasAttribute("open"));
}

function syncDocumentBatchMenus(root = document) {
  root.querySelectorAll("[data-document-batch-menu]").forEach(syncDocumentBatchMenu);
}

function initializeDocumentBatchMenus(root = document) {
  syncDocumentBatchMenus(root);
  initializeOnce(root, "[data-document-batch-menu-toggle]", (button) => {
    button.addEventListener("click", () => {
      const menu = button.closest("[data-document-batch-menu]");
      if (!menu) return;
      setDocumentBatchMenuOpen(menu, !menu.hasAttribute("open"));
    });
  });

  if (documentBatchMenuMediaInitialized || !documentBatchMenuMedia) return;

  const handleChange = () => syncDocumentBatchMenus(document);
  if (typeof documentBatchMenuMedia.addEventListener === "function") {
    documentBatchMenuMedia.addEventListener("change", handleChange);
    documentBatchMenuMediaInitialized = true;
  } else if (typeof documentBatchMenuMedia.addListener === "function") {
    documentBatchMenuMedia.addListener(handleChange);
    documentBatchMenuMediaInitialized = true;
  }
}

function initializeSelectionControls(root = document) {
  initializeDocumentBatchMenus(root);

  initializeOnce(root, "[data-page-size-select]", (select) => {
    select.addEventListener("change", () => {
      const form = select.closest("form");
      if (!form) return;
      if (typeof form.requestSubmit === "function") {
        form.requestSubmit();
      } else {
        form.submit();
      }
    });
  });

  root.querySelectorAll(".table-form").forEach((form) => {
    if (form.dataset.batchSubmitInitialized !== "true") {
      form.dataset.batchSubmitInitialized = "true";
      form.addEventListener("submit", (event) => {
        setBatchBusy(form, "Auswahl wird verarbeitet...");
        if (isDocumentExportSubmit(form, event.submitter)) {
          waitForDocumentExportDownload(form);
        }
      });
    }
    if (form.dataset.selectionInitialized !== "true") {
      form.dataset.selectionInitialized = "true";
      documentSelectionController(form).bind();
    }
    initializeOnce(form, "[data-link-submit]", (button) => {
      button.addEventListener("click", async (event) => {
        const selected = selectedDocumentCount(form);
        if (selected < 2) {
          event.preventDefault();
          showAppAlert("Bitte mindestens zwei Dokumente auswählen.");
          return;
        }
        const message = (button.dataset.linkPrompt || "{count} Dokumente verknüpfen?").replace("{count}", selected);
        event.preventDefault();
        if (await showAppConfirm(message)) {
          form.requestSubmit(button);
        }
      });
    });
    documentSelectionController(form).sync();
  });
}

if (uploadMinimize) {
  uploadMinimize.addEventListener("click", () => {
    uploadStatus.classList.toggle("minimized");
  });
}

function setFormMessage(element, message, isError) {
  if (!element) return;
  element.textContent = message || "";
  element.hidden = !message;
  element.classList.toggle("error", Boolean(isError));
}

const documentDateStatusTimers = new WeakMap();
const documentDateSavePromises = new WeakMap();

function setDocumentDateStatus(editor, message, state, timeout = 0) {
  if (!editor) return;
  const status = editor.querySelector("[data-document-date-status]");
  const existingTimer = documentDateStatusTimers.get(editor);
  if (existingTimer) {
    window.clearTimeout(existingTimer);
    documentDateStatusTimers.delete(editor);
  }
  editor.classList.toggle("saving", state === "saving");
  editor.classList.toggle("error", state === "error");
  editor.classList.toggle("saved", state === "saved");
  if (status) {
    status.textContent = message || "";
  }
  if (timeout > 0) {
    const timer = window.setTimeout(() => {
      editor.classList.remove("saving", "error", "saved");
      if (status) {
        status.textContent = "";
      }
      documentDateStatusTimers.delete(editor);
    }, timeout);
    documentDateStatusTimers.set(editor, timer);
  }
}

function syncDocumentDateInputs(source, value, title) {
  const updateURL = source.dataset.updateUrl || "";
  if (!updateURL) return;
  document.querySelectorAll("[data-document-date-input]").forEach((input) => {
    if (input === source || input.dataset.updateUrl !== updateURL) return;
    input.value = value;
    input.dataset.initialValue = value;
    input.title = title;
  });
}

function initializeDocumentDateInputs(root = document) {
  initializeOnce(root, "[data-document-date-input]", (input) => {
    input.dataset.initialValue = input.dataset.initialValue || input.value || "";

    async function saveDocumentDate() {
      const updateURL = input.dataset.updateUrl;
      const previousValue = input.dataset.initialValue || "";
      const nextValue = input.value || "";
      if (!updateURL || nextValue === previousValue) return null;

      const runningSave = documentDateSavePromises.get(input);
      if (runningSave) return runningSave;

      const editor = input.closest("[data-document-date-editor]");
      const body = new URLSearchParams();
      body.set("document_date", nextValue);

      input.disabled = true;
      setDocumentDateStatus(editor, "Speichern...", "saving");

      const savePromise = (async () => {
        try {
          const response = await fetch(updateURL, {
            method: "POST",
            body,
            credentials: "same-origin",
            headers: { Accept: "application/json" },
          });
          const payload = await response.json().catch(() => ({}));
          if (!response.ok) {
            throw new Error(payload.error || "Dateidatum konnte nicht gespeichert werden");
          }
          input.value = payload.document_date_input || "";
          input.dataset.initialValue = input.value;
          input.title = payload.document_date || "Kein Dateidatum";
          syncDocumentDateInputs(input, input.value, input.title);
          setDocumentDateStatus(editor, payload.notice || "Gespeichert.", "saved", 1800);
        } catch (error) {
          input.value = previousValue;
          setDocumentDateStatus(editor, error.message || "Dateidatum konnte nicht gespeichert werden", "error", 5000);
        } finally {
          input.disabled = false;
          documentDateSavePromises.delete(input);
        }
      })();

      documentDateSavePromises.set(input, savePromise);
      return savePromise;
    }

    input.addEventListener("change", () => {
      if (document.activeElement === input) return;
      saveDocumentDate();
    });

    input.addEventListener("click", () => {
      openSidePreviewForListControl(input);
    });

    input.addEventListener("blur", () => {
      saveDocumentDate();
    });

    input.addEventListener("keydown", (event) => {
      if (event.key !== "Enter") return;
      event.preventDefault();
      input.blur();
    });
  });
}

const customFieldSuggestionTimers = new WeakMap();
const customFieldSuggestionControllers = new WeakMap();

function customFieldDatalist(input) {
  const listID = input.getAttribute("list");
  if (!listID) return null;
  return document.getElementById(listID);
}

function renderCustomFieldSuggestions(input, values) {
  const list = customFieldDatalist(input);
  if (!list) return;
  list.textContent = "";
  (values || []).forEach((value) => {
    const option = document.createElement("option");
    option.value = value;
    list.append(option);
  });
}

async function loadCustomFieldSuggestions(input) {
  const url = input.dataset.customFieldValuesUrl;
  if (!url) return;

  const query = (input.value || "").trim();
  if (input.dataset.customFieldSuggestionQuery === query && customFieldDatalist(input)?.children.length > 0) {
    return;
  }

  const existingController = customFieldSuggestionControllers.get(input);
  if (existingController) {
    existingController.abort();
  }
  const controller = new AbortController();
  customFieldSuggestionControllers.set(input, controller);

  const requestURL = new URL(url, window.location.origin);
  if (query) {
    requestURL.searchParams.set("q", query);
  }

  try {
    const response = await fetch(requestURL, {
      credentials: "same-origin",
      headers: { Accept: "application/json" },
      signal: controller.signal,
    });
    if (!response.ok) return;
    const payload = await response.json().catch(() => ({}));
    if (!Array.isArray(payload.values)) return;
    input.dataset.customFieldSuggestionQuery = query;
    renderCustomFieldSuggestions(input, payload.values);
  } catch (error) {
    if (error.name !== "AbortError") {
      renderCustomFieldSuggestions(input, []);
    }
  } finally {
    if (customFieldSuggestionControllers.get(input) === controller) {
      customFieldSuggestionControllers.delete(input);
    }
  }
}

function scheduleCustomFieldSuggestions(input, delay = 180) {
  const existingTimer = customFieldSuggestionTimers.get(input);
  if (existingTimer) {
    window.clearTimeout(existingTimer);
  }
  const timer = window.setTimeout(() => {
    customFieldSuggestionTimers.delete(input);
    loadCustomFieldSuggestions(input);
  }, delay);
  customFieldSuggestionTimers.set(input, timer);
}

function initializeCustomFieldSuggestions(root = document) {
  initializeOnce(root, "[data-custom-field-values-url]", (input) => {
    input.addEventListener("focus", () => scheduleCustomFieldSuggestions(input, 0));
    input.addEventListener("input", () => scheduleCustomFieldSuggestions(input));
  });
}

initializeCustomFieldSuggestions();

document.querySelectorAll("[data-metadata-form]").forEach((form) => {
  const message = form.querySelector("[data-metadata-message]");
  const submit = form.querySelector('button[type="submit"]');
  form.dataset.initialValue = serializedForm(form);

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    setFormMessage(message, "", false);
    if (submit) {
      submit.disabled = true;
    }

    try {
      const response = await fetch(form.action, {
        method: "POST",
        body: new URLSearchParams(new FormData(form)),
        credentials: "same-origin",
        headers: { Accept: "application/json" },
      });
      const payload = await response.json().catch(() => ({}));
      if (!response.ok) {
        throw new Error(payload.error || "Metadaten konnten nicht gespeichert werden");
      }

      const titleInput = form.querySelector('input[name="title"]');
      if (titleInput && payload.title) {
        titleInput.value = payload.title;
      }
      const documentDateInput = form.querySelector('input[name="document_date"]');
      if (documentDateInput) {
        documentDateInput.value = payload.document_date_input || "";
      }
      const title = document.querySelector("[data-document-title]");
      if (title && payload.title) {
        title.textContent = payload.title;
      }
      const documentDate = document.querySelector("[data-document-date]");
      if (documentDate) {
        documentDate.textContent = payload.document_date || "-";
      }
      const tagSelect = form.querySelector("[data-tag-select]");
      if (tagSelect && Array.isArray(payload.tags)) {
        setTagSelection(tagSelect, payload.tags);
      }
      if (Array.isArray(payload.tags)) {
        const initialProtected = document.body?.dataset.documentDeleteProtected === "true";
        const nextProtected = payload.tags.some((name) => tagOptionMap.get(name)?.deleteProtected);
        if (initialProtected !== nextProtected) {
          window.location.reload();
          return;
        }
      }

      form.dataset.initialValue = serializedForm(form);
      setFormMessage(message, payload.notice || "Metadaten gespeichert.", false);
    } catch (error) {
      setFormMessage(message, error.message || "Metadaten konnten nicht gespeichert werden", true);
    } finally {
      if (submit) {
        submit.disabled = false;
      }
    }
  });
});
