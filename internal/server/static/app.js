const uploadStatus = document.querySelector("[data-upload-status]");
const uploadProgress = document.querySelector("[data-upload-progress]");
const uploadMessage = document.querySelector("[data-upload-message]");
const uploadList = document.querySelector("[data-upload-list]");
const uploadMinimize = document.querySelector("[data-upload-minimize]");
const tagSelectModal = document.querySelector("[data-tag-select-modal]");
const tagSelectList = document.querySelector("[data-tag-select-list]");
const tagSelectSearch = document.querySelector("[data-tag-select-search]");
const tagSelectClose = document.querySelector("[data-tag-select-close]");
const tagSelectClear = document.querySelector("[data-tag-select-clear]");
const tagSelectApply = document.querySelector("[data-tag-select-apply]");
const tagCreate = document.querySelector("[data-tag-create]");
const tagSelectEmpty = document.querySelector("[data-tag-select-empty]");
const tagSelectError = document.querySelector("[data-tag-select-error]");
const ocrStatus = document.querySelector("[data-ocr-status]");
const tagOptions = Array.from(document.querySelectorAll("[data-tag-option]"))
  .map((item) => ({
    name: item.dataset.name || "",
    displayName: item.dataset.displayName || "",
    description: item.dataset.description || "",
    listHidden: item.dataset.listHidden === "true",
    deleteProtected: item.dataset.deleteProtected === "true",
    style: item.getAttribute("style") || "",
  }))
  .filter((item) => item.name)
  .sort((a, b) => (a.displayName || a.name).localeCompare(b.displayName || b.name, "de", { sensitivity: "base" }));
const tagOptionMap = new Map(tagOptions.map((item) => [item.name, item]));
const initializedElements = new WeakMap();
let activeTagSelect = null;
let draftTagSelection = new Set();

function setBatchBusy(form, message = "Batch-Aktion wird verarbeitet...") {
  if (!form) return null;
  form.classList.add("batch-busy");
  form.setAttribute("aria-busy", "true");
  form.querySelectorAll("button").forEach((control) => {
    if (control.disabled) return;
    control.dataset.batchDisabled = "true";
    control.disabled = true;
  });

  let loader = form.querySelector("[data-batch-loader]");
  if (!loader) {
    loader = document.createElement("div");
    loader.className = "batch-loader";
    loader.dataset.batchLoader = "true";
    loader.setAttribute("role", "status");
    loader.setAttribute("aria-live", "polite");

    const spinner = document.createElement("span");
    spinner.className = "batch-loader-spinner";
    spinner.setAttribute("aria-hidden", "true");

    const text = document.createElement("span");
    text.dataset.batchLoaderText = "true";

    loader.append(spinner);
    loader.append(text);
    form.append(loader);
  }
  const text = loader.querySelector("[data-batch-loader-text]");
  if (text) {
    text.textContent = message;
  }
  return loader;
}

function clearBatchBusy(form) {
  if (!form) return;
  form.classList.remove("batch-busy");
  form.removeAttribute("aria-busy");
  form.querySelectorAll("[data-batch-disabled]").forEach((control) => {
    control.disabled = false;
    delete control.dataset.batchDisabled;
  });
  form.querySelector("[data-batch-loader]")?.remove();
}

function initializeOnce(root, selector, callback) {
  root.querySelectorAll(selector).forEach((element) => {
    let initialized = initializedElements.get(element);
    if (!initialized) {
      initialized = new Set();
      initializedElements.set(element, initialized);
    }
    if (initialized.has(selector)) return;
    initialized.add(selector);
    callback(element);
  });
}

function serializedForm(form) {
  return Array.from(new FormData(form).entries()).map(([key, value]) => `${key}=${value}`).join("&");
}

function updateDirtyForm(form) {
  const submit = form.querySelector("[data-dirty-submit]");
  if (!submit) return;
  submit.classList.toggle("hidden", serializedForm(form) === form.dataset.initialValue);
}

document.querySelectorAll("[data-dirty-form]").forEach((form) => {
  const submit = form.querySelector("[data-dirty-submit]");
  if (!submit) return;
  form.dataset.initialValue = serializedForm(form);

  form.addEventListener("input", () => updateDirtyForm(form));
  form.addEventListener("change", () => updateDirtyForm(form));
});

document.querySelectorAll("[data-rule-form]").forEach((form) => {
  const list = form.querySelector("[data-rule-list]");
  const template = form.querySelector("[data-rule-template]");
  const add = form.querySelector("[data-rule-add]");
  if (!list || !template || !add) return;

  add.addEventListener("click", () => {
    const row = template.content.firstElementChild.cloneNode(true);
    list.append(row);
    updateDirtyForm(form);
  });

  form.addEventListener("click", (event) => {
    const remove = event.target.closest("[data-rule-remove]");
    if (!remove) return;
    remove.closest("[data-rule-row]")?.remove();
    updateDirtyForm(form);
  });
});

function createSelectionController(root, options = {}) {
  const itemSelector = options.itemSelector || 'input[name="ids"]';
  const selectAllSelector = options.selectAllSelector || "[data-select-all]";
  const actionsSelector = options.actionsSelector || "[data-selection-actions]";
  const requiresMultipleSelector = options.requiresMultipleSelector || "[data-requires-multiple]";
  const countSelector = options.countSelector || "";
  const countText = options.countText || ((selected) => `${selected} ausgewählt`);
  const onItemChange = options.onItemChange || (() => {});
  const stopItemClickPropagation = options.stopItemClickPropagation === true;
  let anchor = null;

  function closestWithin(target, selector) {
    if (!target || typeof target.closest !== "function") return null;
    const element = target.closest(selector);
    return element && root.contains(element) ? element : null;
  }

  function items() {
    return Array.from(root.querySelectorAll(itemSelector));
  }

  function selectedItems() {
    return items().filter((item) => item.checked);
  }

  function selectedCount() {
    return selectedItems().length;
  }

  function setItemChecked(item, checked, { update = false } = {}) {
    item.checked = checked;
    onItemChange(item, checked);
    if (update) {
      sync();
    }
  }

  function sync() {
    const currentItems = items();
    const selected = currentItems.filter((item) => item.checked).length;

    root.querySelectorAll(actionsSelector).forEach((actions) => {
      actions.classList.toggle("hidden", selected === 0);
    });
    root.querySelectorAll(requiresMultipleSelector).forEach((control) => {
      control.disabled = selected < 2;
    });
    root.querySelectorAll(selectAllSelector).forEach((selectAll) => {
      selectAll.indeterminate = selected > 0 && selected < currentItems.length;
      selectAll.checked = currentItems.length > 0 && selected === currentItems.length;
    });
    if (countSelector) {
      root.querySelectorAll(countSelector).forEach((count) => {
        count.textContent = countText(selected, currentItems.length);
      });
    }

    return selected;
  }

  function setAll(checked, { update = true } = {}) {
    items().forEach((item) => setItemChecked(item, checked));
    if (update) {
      sync();
    }
  }

  function applyRange(item, checked = item.checked) {
    const currentItems = items();
    const start = currentItems.indexOf(anchor);
    const end = currentItems.indexOf(item);
    if (start < 0 || end < 0 || anchor === item) return false;

    const from = Math.min(start, end);
    const to = Math.max(start, end);
    for (let index = from; index <= to; index++) {
      setItemChecked(currentItems[index], checked);
    }
    return true;
  }

  function handleClick(event) {
    const item = closestWithin(event.target, itemSelector);
    if (!item) return;
    if (stopItemClickPropagation) {
      event.stopPropagation();
    }
    if (event.shiftKey && applyRange(item)) {
      sync();
      return;
    }
    anchor = item;
    onItemChange(item, item.checked);
  }

  function handleChange(event) {
    const selectAll = closestWithin(event.target, selectAllSelector);
    if (selectAll) {
      setAll(selectAll.checked);
      return;
    }

    const item = closestWithin(event.target, itemSelector);
    if (!item) return;
    onItemChange(item, item.checked);
    sync();
  }

  function bind() {
    root.addEventListener("click", handleClick);
    root.addEventListener("change", handleChange);
    sync();
  }

  return {
    applyRange,
    bind,
    items,
    selectedCount,
    selectedItems,
    setAll,
    setAnchor(item) {
      anchor = item;
    },
    setItemChecked,
    sync,
  };
}

window.createSelectionController = createSelectionController;
window.BearStack = window.BearStack || {};
window.BearStack.core = Object.assign(window.BearStack.core || {}, {
  clearBatchBusy,
  createSelectionController,
  setBatchBusy,
});

function createAppDialogController() {
  const dialog = document.querySelector("[data-app-dialog]");
  const title = document.querySelector("[data-app-dialog-title]");
  const message = document.querySelector("[data-app-dialog-message]");
  const cancel = document.querySelector("[data-app-dialog-cancel]");
  const confirm = document.querySelector("[data-app-dialog-confirm]");
  const form = dialog?.querySelector("form");
  const inputWrap = document.querySelector("[data-app-dialog-input-wrap]");
  const inputLabel = document.querySelector("[data-app-dialog-input-label]");
  const input = document.querySelector("[data-app-dialog-input]");
  let resolveCurrent = null;
  let mode = "dialog";

  function resetInput() {
    mode = "dialog";
    if (!inputWrap || !input) return;
    inputWrap.classList.add("hidden");
    input.value = "";
    input.required = false;
    input.type = "text";
  }

  function resolveDialog(value) {
    if (!resolveCurrent) return;
    const resolve = resolveCurrent;
    resolveCurrent = null;
    resolve(value);
  }

  function show({ title: nextTitle = "Hinweis", message: nextMessage = "", confirmLabel = "OK", cancelLabel = "", cancelable = false, input: nextInput = null } = {}) {
    if (!dialog || !title || !message || !confirm || !cancel) {
      return Promise.resolve(!cancelable);
    }
    if (dialog.open && resolveCurrent) {
      resolveDialog(false);
    }
    title.textContent = nextTitle;
    message.textContent = nextMessage;
    confirm.textContent = confirmLabel;
    cancel.textContent = cancelLabel;
    cancel.hidden = !cancelable;
    resetInput();
    if (nextInput && inputWrap && input) {
      mode = "prompt";
      inputWrap.classList.remove("hidden");
      if (inputLabel) {
        inputLabel.textContent = nextInput.label || "Eingabe";
      }
      input.type = nextInput.type || "text";
      input.required = nextInput.required !== false;
      input.autocomplete = nextInput.autocomplete || "";
    }
    dialog.returnValue = "";
    dialog.showModal();
    if (mode === "prompt" && input) {
      input.focus();
    } else {
      confirm.focus();
    }

    return new Promise((resolve) => {
      resolveCurrent = resolve;
    });
  }

  cancel?.addEventListener("click", () => {
    resolveDialog(mode === "prompt" ? null : false);
    dialog?.close("cancel");
  });

  form?.addEventListener("submit", (event) => {
    if (mode !== "prompt" || !input || event.submitter?.value !== "confirm") return;
    if (input.required && !input.value) {
      event.preventDefault();
      input.focus();
    }
  });

  dialog?.addEventListener("close", () => {
    const confirmed = dialog.returnValue === "confirm";
    if (mode === "prompt") {
      resolveDialog(confirmed && input ? input.value : null);
    } else {
      resolveDialog(confirmed);
    }
    resetInput();
  });

  return {
    alert(message, title = "Hinweis") {
      return show({ title, message, confirmLabel: "OK" });
    },
    confirm(message, title = "Bitte bestätigen") {
      return show({ title, message, confirmLabel: "Bestätigen", cancelLabel: "Abbrechen", cancelable: true });
    },
    prompt(message, { title = "Bitte bestätigen", label = "Eingabe", type = "text", confirmLabel = "Bestätigen", cancelLabel = "Abbrechen", autocomplete = "" } = {}) {
      if (!dialog || !input) {
        return Promise.resolve(null);
      }
      return show({
        title,
        message,
        confirmLabel,
        cancelLabel,
        cancelable: true,
        input: { label, type, autocomplete, required: true },
      });
    },
  };
}

const appDialogController = createAppDialogController();

function showAppAlert(message, title = "Hinweis") {
  return appDialogController.alert(message, title);
}

function showAppConfirm(message, title = "Bitte bestätigen") {
  return appDialogController.confirm(message, title);
}

function showAppPrompt(message, options = {}) {
  return appDialogController.prompt(message, options);
}

let appToastElement = null;
let appToastTimer = 0;

function appToast() {
  if (appToastElement) return appToastElement;
  appToastElement = document.createElement("div");
  appToastElement.className = "app-toast";
  appToastElement.setAttribute("role", "status");
  appToastElement.setAttribute("aria-live", "polite");
  appToastElement.hidden = true;
  document.body.append(appToastElement);
  return appToastElement;
}

function showAppToast(message, duration = 2200) {
  const toast = appToast();
  if (!toast) return;
  if (appToastTimer) {
    window.clearTimeout(appToastTimer);
    appToastTimer = 0;
  }
  toast.textContent = message || "";
  toast.hidden = false;
  toast.classList.add("visible");
  appToastTimer = window.setTimeout(() => {
    toast.classList.remove("visible");
    toast.hidden = true;
    appToastTimer = 0;
  }, Math.max(800, duration));
}
window.showAppToast = showAppToast;

let contextHelpPopover = null;
let contextHelpActiveTrigger = null;
let contextHelpPinnedTrigger = null;

function contextHelpElement() {
  if (contextHelpPopover) return contextHelpPopover;
  contextHelpPopover = document.createElement("div");
  contextHelpPopover.className = "context-help-popover";
  contextHelpPopover.hidden = true;
  contextHelpPopover.setAttribute("role", "dialog");
  contextHelpPopover.setAttribute("aria-modal", "false");
  contextHelpPopover.innerHTML = '<strong data-context-help-popover-title></strong><p data-context-help-popover-text></p>';
  document.body.append(contextHelpPopover);
  return contextHelpPopover;
}

function positionContextHelp(trigger, popover) {
  const rect = trigger.getBoundingClientRect();
  popover.style.left = "0";
  popover.style.top = "0";
  popover.hidden = false;
  popover.style.visibility = "hidden";

  const width = popover.offsetWidth;
  const height = popover.offsetHeight;
  const inset = 14;
  const gap = 8;
  let left = rect.left + rect.width / 2 - width / 2;
  left = Math.max(inset, Math.min(left, window.innerWidth - width - inset));

  let top = rect.bottom + gap;
  if (top + height > window.innerHeight - inset) {
    top = rect.top - height - gap;
  }
  top = Math.max(inset, top);

  popover.style.left = `${left}px`;
  popover.style.top = `${top}px`;
  popover.style.visibility = "";
}

function hideContextHelp(trigger = null) {
  if (trigger && contextHelpPinnedTrigger && contextHelpPinnedTrigger !== trigger) return;
  const popover = contextHelpElement();
  popover.hidden = true;
  document.querySelectorAll("[data-context-help][aria-expanded='true']").forEach((item) => {
    item.setAttribute("aria-expanded", "false");
  });
  contextHelpActiveTrigger = null;
  contextHelpPinnedTrigger = null;
}

function showContextHelp(trigger, pinned = false) {
  if (!pinned && contextHelpPinnedTrigger && contextHelpPinnedTrigger !== trigger) return;
  const popover = contextHelpElement();
  popover.querySelector("[data-context-help-popover-title]").textContent = trigger.dataset.contextHelpTitle || "Hinweis";
  popover.querySelector("[data-context-help-popover-text]").textContent = trigger.dataset.contextHelp || "";
  if (contextHelpActiveTrigger && contextHelpActiveTrigger !== trigger) {
    contextHelpActiveTrigger.setAttribute("aria-expanded", "false");
  }
  contextHelpActiveTrigger = trigger;
  if (pinned) {
    contextHelpPinnedTrigger = trigger;
  }
  trigger.setAttribute("aria-expanded", "true");
  positionContextHelp(trigger, popover);
}

function initializeContextHelp(root = document) {
  initializeOnce(root, "[data-context-help]", (trigger) => {
    trigger.addEventListener("mouseenter", () => showContextHelp(trigger, false));
    trigger.addEventListener("mouseleave", () => {
      if (contextHelpPinnedTrigger !== trigger) hideContextHelp(trigger);
    });
    trigger.addEventListener("focus", () => showContextHelp(trigger, false));
    trigger.addEventListener("blur", () => {
      if (contextHelpPinnedTrigger !== trigger) hideContextHelp(trigger);
    });
    trigger.addEventListener("click", (event) => {
      event.preventDefault();
      event.stopPropagation();
      if (contextHelpPinnedTrigger === trigger) {
        hideContextHelp();
        return;
      }
      showContextHelp(trigger, true);
    });
    trigger.addEventListener("keydown", (event) => {
      if (event.key === "Escape") {
        event.preventDefault();
        hideContextHelp();
        return;
      }
      if (event.key !== "Enter" && event.key !== " ") return;
      event.preventDefault();
      if (contextHelpPinnedTrigger === trigger) {
        hideContextHelp();
      } else {
        showContextHelp(trigger, true);
      }
    });
  });
}

function showUploadStatus(message) {
  if (!uploadStatus) return;
  uploadStatus.classList.remove("hidden");
  uploadStatus.classList.remove("minimized");
  uploadMessage.textContent = message || "";
}

function setUploadProgress(value) {
  if (!uploadProgress) return;
  uploadProgress.value = Math.max(0, Math.min(100, value));
}

function addUploadLine(text) {
  if (!uploadList) return;
  const li = document.createElement("li");
  li.textContent = text;
  uploadList.prepend(li);
}

async function refreshDocumentList() {
  const container = document.querySelector("[data-document-list]");
  if (!container) return false;
  const activePreviewDocumentID = document.querySelector(".document-preview-active[data-document-id]")?.dataset.documentId || "";

  const response = await fetch(window.location.href, {
    credentials: "same-origin",
    headers: {
      "X-Requested-With": "XMLHttpRequest",
      "X-BearStack-Partial": "document-list",
    },
  });
  if (!response.ok) return false;

  const html = await response.text();
  const doc = new DOMParser().parseFromString(html, "text/html");
  const updated = doc.querySelector("[data-document-list]");
  if (!updated) return false;

  container.innerHTML = updated.innerHTML;
  initializeDocumentList(container);
  if (activePreviewDocumentID) {
    container
      .querySelector(`[data-document-id="${CSS.escape(activePreviewDocumentID)}"]`)
      ?.classList.add("document-preview-active");
  }
  return true;
}

function closeOpenMenusOutside(target) {
  if (!(target instanceof Node)) return;

  document.querySelectorAll(".system-menu[open], .document-menu[open], .filter-year-menu[open], .document-batch-menu[open], .photo-filter-menu[open], .photo-sort-menu[open], .photo-actions-menu[open]").forEach((menu) => {
    if (!menu.contains(target)) {
      menu.open = false;
      menu.removeAttribute("open");
    }
  });
}

document.addEventListener("click", (event) => {
  closeOpenMenusOutside(event.target);
  if (
    contextHelpPinnedTrigger &&
    !contextHelpPinnedTrigger.contains(event.target) &&
    !contextHelpElement().contains(event.target)
  ) {
    hideContextHelp();
  }
});

document.addEventListener("keydown", (event) => {
  if (event.key === "Escape" && (contextHelpActiveTrigger || contextHelpPinnedTrigger)) {
    hideContextHelp();
  }
});

window.addEventListener("resize", () => {
  if (contextHelpActiveTrigger && !contextHelpElement().hidden) {
    positionContextHelp(contextHelpActiveTrigger, contextHelpElement());
  }
});

function initializeTabs(root = document) {
  initializeOnce(root, "[data-tabs]", (tabs) => {
    const buttons = Array.from(tabs.querySelectorAll("[data-tab-target]"));
    const panels = Array.from(tabs.querySelectorAll("[data-tab-panel]"));

    function activateTab(target, selectedButton) {
      if (!target) return;
      buttons.forEach((button) => {
        const active = selectedButton ? button === selectedButton : button.dataset.tabTarget === target;
        button.classList.toggle("active", active);
        button.setAttribute("aria-selected", active ? "true" : "false");
      });
      panels.forEach((panel) => {
        const active = panel.dataset.tabPanel === target;
        panel.classList.toggle("active", active);
        panel.hidden = !active;
      });
    }

    buttons.forEach((button) => {
      button.addEventListener("click", () => activateTab(button.dataset.tabTarget || "", button));
    });

    const activeButton =
      buttons.find((button) => button.classList.contains("active")) ||
      buttons.find((button) => button.getAttribute("aria-selected") === "true") ||
      buttons[0];
    if (activeButton) {
      activateTab(activeButton.dataset.tabTarget || "", activeButton);
    }
  });
}

function initializeSubmitPrompts(root = document) {
  initializeOnce(root, "[data-confirm], [data-password-prompt]", (form) => {
    form.addEventListener("submit", async (event) => {
      if (form.dataset.confirmed === "true") {
        delete form.dataset.confirmed;
        return;
      }
      event.preventDefault();
      if (form.dataset.passwordPrompt) {
        const password = await showAppPrompt(form.dataset.passwordPrompt, {
          title: form.dataset.passwordPromptTitle || "Passwort bestätigen",
          label: form.dataset.passwordPromptLabel || "Passwort",
          type: "password",
          confirmLabel: form.dataset.passwordPromptConfirm || "Bestätigen",
          autocomplete: "current-password",
        });
        if (password === null) return;
        let input = form.querySelector('input[type="hidden"][name="password"][data-prompt-value]');
        if (!input) {
          input = document.createElement("input");
          input.type = "hidden";
          input.name = "password";
          input.dataset.promptValue = "true";
          form.append(input);
        }
        input.value = password;
        form.dataset.confirmed = "true";
        form.requestSubmit(event.submitter || undefined);
        return;
      }
      if (await showAppConfirm(form.dataset.confirm)) {
        form.dataset.confirmed = "true";
        form.requestSubmit(event.submitter || undefined);
      }
    });
  });
}

initializeTabs();
initializeContextHelp();
initializeSubmitPrompts();
