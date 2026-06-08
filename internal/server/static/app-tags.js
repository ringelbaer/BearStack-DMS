function splitTagValues(value) {
  return (value || "")
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

function normalizedTagName(value) {
  return splitTagValues(value)[0] || "";
}

function tagExists(name) {
  return tagOptionMap.has(name);
}

function updateTagCreateButton() {
  if (!tagCreate) return;
  const name = normalizedTagName(tagSelectSearch?.value || "");
  tagCreate.disabled = !name || tagExists(name);
}

function currentTagSelection(picker) {
  const inputContainer = picker.querySelector("[data-tag-select-inputs]");
  const inputs = inputContainer ? inputContainer.querySelectorAll('input[type="hidden"]') : [];
  if (inputs.length > 0) {
    return Array.from(inputs).map((input) => input.value).filter(Boolean);
  }
  return splitTagValues(picker.dataset.selectedTags || "");
}

function orderedTagNames(values) {
  const selected = new Set(values);
  const ordered = Array.from(selected)
    .filter((name) => tagOptionOrder.has(name))
    .sort((a, b) => tagOptionOrder.get(a) - tagOptionOrder.get(b));
  const known = new Set(ordered);
  const unknown = values.filter((name) => !known.has(name)).sort((a, b) => displayTagName(a).localeCompare(displayTagName(b), "de", { sensitivity: "base" }));
  return ordered.concat(unknown);
}

function normalizeTagDisplayMode(mode) {
  const value = String(mode || "").trim().toLowerCase();
  if (value === "strtoupper" || value === "ucfirst") return value;
  return "strtolower";
}

function formatTagDisplayName(name) {
  const raw = String(name || "").trim();
  const mode = normalizeTagDisplayMode(document.body?.dataset?.tagDisplayMode);
  if (mode === "strtoupper") {
    return raw.toLocaleUpperCase();
  }
  if (mode === "ucfirst") {
    const lower = raw.toLocaleLowerCase();
    const chars = Array.from(lower);
    if (!chars.length) return "";
    const upperFirst = Array.from(chars[0].toLocaleUpperCase())[0] || chars[0];
    chars[0] = upperFirst;
    return chars.join("");
  }
  return raw.toLocaleLowerCase();
}

function displayTagName(name, option = null) {
  return option?.displayName || tagOptionMap.get(name)?.displayName || formatTagDisplayName(name);
}

function createTagPill(name) {
  const option = tagOptionMap.get(name);
  const pill = document.createElement("span");
  pill.className = "tag";
  pill.textContent = displayTagName(name, option);
  if (option) {
    pill.style.cssText = option.style;
    pill.title = option.description;
  }
  return pill;
}

function createHiddenTagIndicator(names) {
  const label = names.map((name) => displayTagName(name)).join(", ");
  const indicator = document.createElement("span");
  indicator.className = "tag tag-hidden-indicator";
  indicator.textContent = "...";
  indicator.title = label;
  indicator.setAttribute("aria-label", label);
  return indicator;
}

function tagOptionFromTag(tag) {
  if (!tag || !tag.name) return null;
  return {
    name: tag.name,
    displayName: tag.display_name || tag.displayName || tag.name,
    description: tag.description || "",
    listHidden: Boolean(tag.list_hidden),
    deleteProtected: Boolean(tag.delete_protected),
    style: tag.style || "",
  };
}

function updateTagOption(existing, option) {
  existing.displayName = option.displayName || existing.displayName;
  existing.description = option.description || existing.description;
  existing.listHidden = option.listHidden;
  existing.deleteProtected = option.deleteProtected;
  existing.style = option.style || existing.style;
}

function sortTagOptions() {
  tagOptions.sort((a, b) => displayTagName(a.name, a).localeCompare(displayTagName(b.name, b), "de", { sensitivity: "base" }));
  rebuildTagOptionOrder();
}

function addTagOption(tag) {
  const option = tagOptionFromTag(tag);
  if (!option) return null;
  const existing = tagOptionMap.get(option.name);
  if (existing) {
    updateTagOption(existing, option);
    tagSelectOptionNodes.delete(option.name);
    return existing;
  }
  tagOptions.push(option);
  tagOptionMap.set(option.name, option);
  sortTagOptions();
  tagSelectOptionNodes.delete(option.name);
  return option;
}

function addTagOptions(tags) {
  let added = false;
  (tags || []).forEach((tag) => {
    const option = tagOptionFromTag(tag);
    if (!option) return;
    const existing = tagOptionMap.get(option.name);
    if (existing) {
      updateTagOption(existing, option);
      tagSelectOptionNodes.delete(option.name);
      return;
    }
    tagOptions.push(option);
    tagOptionMap.set(option.name, option);
    tagSelectOptionNodes.delete(option.name);
    added = true;
  });
  if (added) {
    sortTagOptions();
  }
}

function tagDisplayGroups(picker, selected) {
  const hideListTags = picker.dataset.hideListTags === "true";
  const displayed = hideListTags
    ? selected.filter((name) => !tagOptionMap.get(name)?.listHidden)
    : selected;
  const hidden = hideListTags
    ? selected.filter((name) => tagOptionMap.get(name)?.listHidden)
    : [];
  return { displayed, hidden };
}

function updateTagTriggerTitle(picker, selected) {
  const trigger = picker.querySelector("[data-tag-select-trigger]");
  if (!trigger) return;
  const { displayed, hidden } = tagDisplayGroups(picker, selected);
  const titleItems = displayed.concat(hidden).map((name) => displayTagName(name));
  trigger.title = titleItems.length > 0 ? titleItems.join(", ") : picker.dataset.emptyLabel || "Keine Tags";
}

function renderTagSummary(picker, values) {
  const summary = picker.querySelector("[data-tag-select-summary]");
  if (!summary) return;
  const selected = orderedTagNames(values);
  const { displayed, hidden } = tagDisplayGroups(picker, selected);
  summary.textContent = "";
  if (displayed.length === 0 && hidden.length === 0) {
    const empty = document.createElement("span");
    empty.className = "muted";
    empty.textContent = picker.dataset.emptyLabel || "Keine Tags";
    summary.append(empty);
  } else {
    displayed.forEach((name) => summary.append(createTagPill(name)));
    if (hidden.length > 0) {
      summary.append(createHiddenTagIndicator(hidden));
    }
  }
  picker.dataset.selectedTags = selected.join(", ");
  updateTagTriggerTitle(picker, selected);
}

function syncTagInputs(picker, values) {
  const inputContainer = picker.querySelector("[data-tag-select-inputs]");
  const inputName = picker.dataset.inputName || "tags";
  if (!inputContainer) return;
  inputContainer.textContent = "";
  values.forEach((name) => {
    const input = document.createElement("input");
    input.type = "hidden";
    input.name = inputName;
    input.value = name;
    inputContainer.append(input);
  });
}

function setTagSelection(picker, values) {
  const selected = orderedTagNames(values);
  syncTagInputs(picker, selected);
  renderTagSummary(picker, selected);
  const form = picker.closest("form");
  if (form && form.dataset.initialValue) {
    updateDirtyForm(form);
  }
}

function setTagSelectError(message) {
  if (!tagSelectError) return;
  tagSelectError.textContent = message || "";
  tagSelectError.hidden = !message;
}

const tagSelectOptionNodes = new Map();
let tagOptionOrder = new Map();
let tagSelectRenderTimer = 0;
let tagOptionsLoaded = tagOptions.length > 0;
let tagOptionsLoading = null;

function rebuildTagOptionOrder() {
  tagOptionOrder = new Map(tagOptions.map((option, index) => [option.name, index]));
}

rebuildTagOptionOrder();

function tagOptionsURL() {
  return tagSelectModal?.dataset.tagOptionsUrl || document.querySelector("[data-tag-options]")?.dataset.tagOptionsUrl || "";
}

function ensureTagOptionsLoaded() {
  const url = tagOptionsURL();
  if (tagOptionsLoaded || !url) {
    tagOptionsLoaded = true;
    return Promise.resolve();
  }
  if (tagOptionsLoading) return tagOptionsLoading;
  tagOptionsLoading = fetch(url, {
    credentials: "same-origin",
    headers: { Accept: "application/json" },
  }).then((response) => {
    if (!response.ok) {
      throw new Error("Tags konnten nicht geladen werden");
    }
    return response.json();
  }).then((payload) => {
    addTagOptions(payload.tags || []);
    tagOptionsLoaded = true;
  }).catch((error) => {
    setTagSelectError(error.message || "Tags konnten nicht geladen werden");
  }).finally(() => {
    tagOptionsLoading = null;
  });
  return tagOptionsLoading;
}

function tagSelectOptionNode(option) {
  let node = tagSelectOptionNodes.get(option.name);
  if (node) return node;

  const label = document.createElement("label");
  label.className = "tag-select-option";
  label.title = option.description;

  const checkbox = document.createElement("input");
  checkbox.type = "checkbox";
  checkbox.value = option.name;
  checkbox.addEventListener("change", () => {
    if (checkbox.checked) {
      draftTagSelection.add(option.name);
    } else {
      draftTagSelection.delete(option.name);
    }
  });

  label.append(checkbox);
  label.append(createTagPill(option.name));
  node = { label, checkbox };
  tagSelectOptionNodes.set(option.name, node);
  return node;
}

function renderTagSelectList() {
  if (!tagSelectList) return;
  const query = (tagSelectSearch?.value || "").trim().toLowerCase();
  const fragment = document.createDocumentFragment();
  let count = 0;
  tagOptions.forEach((option) => {
    if (query && !option.name.toLowerCase().includes(query) && !displayTagName(option.name, option).toLowerCase().includes(query)) return;
    count += 1;

    const { label, checkbox } = tagSelectOptionNode(option);
    checkbox.checked = draftTagSelection.has(option.name);
    fragment.append(label);
  });
  tagSelectList.replaceChildren(fragment);
  if (tagSelectEmpty) {
    tagSelectEmpty.hidden = count > 0;
  }
  updateTagCreateButton();
}

function scheduleTagSelectListRender(delay = 80) {
  if (tagSelectRenderTimer) {
    window.clearTimeout(tagSelectRenderTimer);
  }
  tagSelectRenderTimer = window.setTimeout(() => {
    tagSelectRenderTimer = 0;
    renderTagSelectList();
  }, delay);
}

function focusTagSelectSearch() {
  if (!tagSelectSearch) return;
  var applyFocus = function () {
    if (!tagSelectSearch || tagSelectSearch.disabled) return;
    if (typeof tagSelectSearch.focus === "function") {
      try {
        tagSelectSearch.focus({ preventScroll: true });
      } catch (_) {
        tagSelectSearch.focus();
      }
    }
  };
  applyFocus();
  var schedule = window.requestAnimationFrame || function (callback) {
    return window.setTimeout(callback, 16);
  };
  schedule(applyFocus);
  window.setTimeout(applyFocus, 0);
}

async function openTagSelect(picker) {
  if (!tagSelectModal) return;
  if (typeof openSidePreviewForListControl === "function") {
    openSidePreviewForListControl(picker);
  }
  activeTagSelect = picker;
  draftTagSelection = picker.dataset.bulkTags === "true" ? new Set() : new Set(currentTagSelection(picker));
  setTagSelectError("");
  if (tagSelectSearch) {
    tagSelectSearch.value = "";
  }
  if (typeof tagSelectModal.showModal === "function" && !tagSelectModal.open) {
    tagSelectModal.showModal();
  }
  focusTagSelectSearch();
  await ensureTagOptionsLoaded();
  renderTagSelectList();
  focusTagSelectSearch();
}

async function createTagFromSearch() {
  const rawName = tagSelectSearch?.value || "";
  const name = normalizedTagName(rawName);
  if (!name || tagExists(name)) return;

  if (activeTagSelect?.dataset.createTagMode === "local") {
    const option = addTagOption({ name });
    if (!option) return;
    draftTagSelection.add(option.name);
    if (tagSelectSearch) {
      tagSelectSearch.value = option.name;
    }
    renderTagSelectList();
    return;
  }

  const body = new URLSearchParams();
  body.set("name", rawName);
  tagCreate.disabled = true;
  setTagSelectError("");
  try {
    const response = await fetch("/tags", {
      method: "POST",
      body,
      credentials: "same-origin",
      headers: { Accept: "application/json" },
    });
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) {
      throw new Error(payload.error || "Tag konnte nicht angelegt werden");
    }
    const option = addTagOption(payload);
    if (!option) {
      throw new Error("Tag konnte nicht gelesen werden");
    }
    draftTagSelection.add(option.name);
    if (tagSelectSearch) {
      tagSelectSearch.value = option.name;
    }
    renderTagSelectList();
  } catch (error) {
    setTagSelectError(error.message || "Tag konnte nicht angelegt werden");
    updateTagCreateButton();
  }
}

async function applyTagSelect() {
  if (!activeTagSelect) return;
  const selected = orderedTagNames(Array.from(draftTagSelection));
  const updateUrl = activeTagSelect.dataset.updateUrl;

  if (!updateUrl) {
    const picker = activeTagSelect;
    const form = picker.closest("form");
    setTagSelection(picker, selected);
    tagSelectModal?.close();
    if (picker.dataset.submitOnApply === "true" && form) {
      if (typeof form.requestSubmit === "function") {
        form.requestSubmit();
      } else {
        form.submit();
      }
    }
    return;
  }

  const body = new URLSearchParams();
  selected.forEach((name) => body.append("tags", name));
  const bulkForm = activeTagSelect.dataset.bulkTags === "true" ? activeTagSelect.closest("form") : null;
  if (activeTagSelect.dataset.bulkTags === "true") {
    bulkForm?.querySelectorAll('input[name="ids"]:checked').forEach((input) => body.append("ids", input.value));
  }
  tagSelectApply.disabled = true;
  setTagSelectError("");
  if (bulkForm) {
    setBatchBusy(bulkForm, "Tags werden fuer die Auswahl gespeichert...");
  }
  try {
    const response = await fetch(updateUrl, {
      method: "POST",
      body,
      credentials: "same-origin",
      headers: { Accept: "application/json" },
    });
    if (!response.ok) {
      throw new Error("Tags konnten nicht gespeichert werden");
    }
    const payload = await response.json();
    if (activeTagSelect.dataset.bulkTags === "true") {
      if (bulkForm?.matches("[data-photo-bulk-form]")) {
        try {
          window.sessionStorage.setItem("bearstackPhotoModeAfterBulk", "edit");
        } catch (_) {}
      }
      tagSelectModal?.close();
      window.location.reload();
      return;
    }
    setTagSelection(activeTagSelect, payload.tags || selected);
    tagSelectModal?.close();
  } catch (error) {
    if (bulkForm) {
      clearBatchBusy(bulkForm);
    }
    setTagSelectError(error.message || "Tags konnten nicht gespeichert werden");
  } finally {
    tagSelectApply.disabled = false;
  }
}

function initializeTagSelects(root = document) {
  root.querySelectorAll("[data-tag-select]").forEach((picker) => {
    initializeTagSelectSummary(picker);
    const trigger = picker.querySelector("[data-tag-select-trigger]");
    if (!trigger) return;
    initializeOnce(picker, "[data-tag-select-trigger]", (button) => {
      button.addEventListener("click", () => openTagSelect(picker));
    });
  });

  initializeOnce(root, "[data-bulk-tags-open]", (button) => {
    button.addEventListener("click", () => {
      const form = button.closest("form");
      const selected = form?.querySelectorAll('input[name="ids"]:checked').length || 0;
      if (selected === 0) {
        showAppAlert(button.dataset.emptySelectionMessage || "Bitte mindestens ein Dokument auswählen.");
        return;
      }
      const action = button.dataset.bulkTagsOpen || "add";
      const pickers = Array.from(form?.querySelectorAll('[data-bulk-tags="true"]') || []);
      const picker = pickers.find((item) => item.dataset.bulkTagsAction === action) || pickers[0];
      if (picker) {
        openTagSelect(picker);
      }
    });
  });
}

function initializeTagSelectSummary(picker) {
  const summary = picker.querySelector("[data-tag-select-summary]");
  if (!summary) return;
  const selected = orderedTagNames(currentTagSelection(picker));
  const alreadyRendered = summary.childElementCount > 0 || summary.textContent.trim() !== "";
  if (alreadyRendered) {
    picker.dataset.selectedTags = selected.join(", ");
    updateTagTriggerTitle(picker, selected);
    return;
  }
  renderTagSummary(picker, selected);
}

tagSelectSearch?.addEventListener("input", () => {
  updateTagCreateButton();
  scheduleTagSelectListRender();
});
tagCreate?.addEventListener("click", createTagFromSearch);
tagSelectClear?.addEventListener("click", () => {
  draftTagSelection.clear();
  renderTagSelectList();
});
tagSelectApply?.addEventListener("click", applyTagSelect);
tagSelectClose?.addEventListener("click", () => tagSelectModal?.close());
tagSelectModal?.addEventListener("close", () => {
  activeTagSelect = null;
  setTagSelectError("");
});

function updateSearchFavoriteYearField(select) {
  const form = select.closest("form");
  const field = form?.querySelector("[data-search-favorite-year-field]");
  if (!field) return;
  const active = select.value === "year";
  field.hidden = !active;
  field.querySelectorAll("input, select, textarea").forEach((input) => {
    input.disabled = !active;
  });
}

function initializeSearchFavoriteYearFields(root = document) {
  initializeOnce(root, "[data-search-favorite-date-mode]", (select) => {
    updateSearchFavoriteYearField(select);
    select.addEventListener("change", () => updateSearchFavoriteYearField(select));
  });
}

document.addEventListener("DOMContentLoaded", () => {
  if (!document.querySelector("[data-photo-module]")) {
    initializeTagSelects(document);
  }
  initializeSearchFavoriteYearFields(document);
});

window.initializeTagSelects = initializeTagSelects;
window.BearStack = window.BearStack || {};
window.BearStack.tags = Object.assign(window.BearStack.tags || {}, {
  addTagOption,
  currentTagSelection,
  displayTagName,
  initializeTagSelects,
  normalizedTagName,
  orderedTagNames,
  renderTagSummary,
  setTagSelection,
  splitTagValues,
});
