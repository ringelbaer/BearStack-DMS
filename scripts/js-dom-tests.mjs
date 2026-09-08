#!/usr/bin/env node
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import vm from "node:vm";
import { fileURLToPath } from "node:url";

const repoDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const staticDir = path.join(repoDir, "internal/server/static");

function dataName(name) {
  return name
    .slice(5)
    .replace(/-([a-z])/g, (_, char) => char.toUpperCase());
}

class TestClassList {
  constructor(element) {
    this.element = element;
    this.values = new Set();
  }

  sync() {
    this.element.className = Array.from(this.values).join(" ");
    if (this.element.className) {
      this.element.attributes.set("class", this.element.className);
    } else {
      this.element.attributes.delete("class");
    }
  }

  add(...names) {
    names.filter(Boolean).forEach((name) => this.values.add(name));
    this.sync();
  }

  remove(...names) {
    names.forEach((name) => this.values.delete(name));
    this.sync();
  }

  toggle(name, force) {
    const enabled = force === undefined ? !this.values.has(name) : Boolean(force);
    if (enabled) {
      this.values.add(name);
    } else {
      this.values.delete(name);
    }
    this.sync();
    return enabled;
  }

  contains(name) {
    return this.values.has(name);
  }
}

class TestStyle {
  constructor() {
    this.cssText = "";
  }

  setProperty(name, value) {
    this[name] = value;
  }
}

class TestEventTarget {
  constructor() {
    this.listeners = new Map();
  }

  addEventListener(type, callback) {
    if (!this.listeners.has(type)) this.listeners.set(type, []);
    this.listeners.get(type).push(callback);
  }

  dispatchEvent(event) {
    event.target = event.target || this;
    event.currentTarget = this;
    event.preventDefault = event.preventDefault || (() => {
      event.defaultPrevented = true;
    });
    event.stopPropagation = event.stopPropagation || (() => {});
    for (const callback of this.listeners.get(event.type) || []) {
      callback(event);
    }
    return !event.defaultPrevented;
  }
}

class TestElement extends TestEventTarget {
  constructor(tagName = "div") {
    super();
    this.tagName = tagName.toUpperCase();
    this.children = [];
    this.parentElement = null;
    this.attributes = new Map();
    this.dataset = {};
    this.classList = new TestClassList(this);
    this.style = new TestStyle();
    this.className = "";
    this.hidden = false;
    this.disabled = false;
    this.checked = false;
    this.indeterminate = false;
    this.value = "";
    this.name = "";
    this.type = "";
    this.title = "";
    this.open = false;
    this.clientWidth = 0;
    this.clientHeight = 0;
    this.offsetWidth = 0;
    this.offsetHeight = 0;
    this._textContent = "";
    this.cookieStore = new Map();
  }

  set textContent(value) {
    this._textContent = String(value ?? "");
    this.children = [];
  }

  get textContent() {
    if (this.children.length === 0) return this._textContent;
    return this.children.map((child) => child.textContent).join("");
  }

  get firstElementChild() {
    return this.children[0] || null;
  }

  setAttribute(name, value) {
    value = String(value);
    this.attributes.set(name, value);
    if (name.startsWith("data-")) {
      this.dataset[dataName(name)] = value;
    } else if (name === "class") {
      this.className = value;
      this.classList.values = new Set(value.split(/\s+/).filter(Boolean));
    } else if (name === "type") {
      this.type = value;
    } else if (name === "name") {
      this.name = value;
    } else if (name === "value") {
      this.value = value;
    } else if (name === "title") {
      this.title = value;
    } else if (name === "hidden") {
      this.hidden = true;
    }
  }

  getAttribute(name) {
    if (name.startsWith("data-")) {
      return this.dataset[dataName(name)] ?? null;
    }
    if (name === "class") return this.className || null;
    if (name === "type") return this.type || null;
    if (name === "name") return this.name || null;
    if (name === "value") return this.value || null;
    if (name === "title") return this.title || null;
    return this.attributes.get(name) ?? null;
  }

  hasAttribute(name) {
    return this.getAttribute(name) !== null;
  }

  removeAttribute(name) {
    this.attributes.delete(name);
    if (name.startsWith("data-")) {
      delete this.dataset[dataName(name)];
    } else if (name === "hidden") {
      this.hidden = false;
    }
  }

  append(...nodes) {
    for (const node of nodes.flat()) {
      if (!node) continue;
      if (node instanceof TestFragment) {
        this.append(...node.children);
        node.children = [];
        continue;
      }
      node.parentElement = this;
      this.children.push(node);
    }
  }

  insertBefore(node, reference) {
    node.remove();
    const index = reference ? this.children.indexOf(reference) : this.children.length;
    this.children.splice(index, 0, node);
    node.parentElement = this;
    return node;
  }

  prepend(...nodes) {
    for (const node of nodes.reverse()) {
      if (!node) continue;
      node.parentElement = this;
      this.children.unshift(node);
    }
  }

  replaceChildren(...nodes) {
    this.children.forEach((child) => {
      child.parentElement = null;
    });
    this.children = [];
    this.append(...nodes);
  }

  remove() {
    if (!this.parentElement) return;
    this.parentElement.children = this.parentElement.children.filter((child) => child !== this);
    this.parentElement = null;
  }

  contains(node) {
    if (node === this) return true;
    return this.children.some((child) => child.contains(node));
  }

  querySelector(selector) {
    return this.querySelectorAll(selector)[0] || null;
  }

  querySelectorAll(selector) {
    const results = [];
    const visit = (node) => {
      for (const child of node.children) {
        if (child.matches(selector)) results.push(child);
        visit(child);
      }
    };
    visit(this);
    return results;
  }

  closest(selector) {
    let node = this;
    while (node) {
      if (node.matches(selector)) return node;
      node = node.parentElement;
    }
    return null;
  }

  matches(selector) {
    return selector.split(",").some((part) => matchesSimpleSelector(this, part.trim()));
  }

  focus() {}

  requestSubmit(submitter = null) {
    this.dispatchEvent({ type: "submit", submitter });
  }

  submit() {}

  showModal() {
    this.open = true;
  }

  close() {
    this.open = false;
    this.dispatchEvent({ type: "close" });
  }

  getBoundingClientRect() {
    return {
      bottom: this.clientHeight,
      height: this.clientHeight,
      left: 0,
      right: this.clientWidth,
      top: 0,
      width: this.clientWidth,
    };
  }

  cloneNode(deep = false) {
    const clone = new TestElement(this.tagName.toLowerCase());
    for (const [name, value] of this.attributes.entries()) {
      clone.setAttribute(name, value);
    }
    clone.hidden = this.hidden;
    clone.disabled = this.disabled;
    clone.checked = this.checked;
    clone.indeterminate = this.indeterminate;
    clone.value = this.value;
    clone.name = this.name;
    clone.type = this.type;
    clone.title = this.title;
    clone.open = this.open;
    clone.clientWidth = this.clientWidth;
    clone.clientHeight = this.clientHeight;
    clone.offsetWidth = this.offsetWidth;
    clone.offsetHeight = this.offsetHeight;
    clone.style.cssText = this.style.cssText;
    clone._textContent = this._textContent;
    if (deep) {
      clone.append(...this.children.map((child) => child.cloneNode(true)));
    }
    return clone;
  }
}

class TestFragment extends TestElement {
  constructor() {
    super("#fragment");
  }
}

class TestDocument extends TestElement {
  constructor() {
    super("#document");
    this.documentElement = new TestElement("html");
    this.body = new TestElement("body");
    this.documentElement.append(this.body);
    this.append(this.documentElement);
  }

  get cookie() {
    return Array.from(this.cookieStore.entries())
      .map(([name, value]) => `${name}=${value}`)
      .join("; ");
  }

  set cookie(value) {
    const parts = String(value || "").split(";").map((part) => part.trim());
    const pair = parts.shift() || "";
    const separator = pair.indexOf("=");
    if (separator < 1) return;
    const name = pair.slice(0, separator);
    const cookieValue = pair.slice(separator + 1);
    if (parts.some((part) => part.toLowerCase() === "max-age=0")) {
      this.cookieStore.delete(name);
      return;
    }
    this.cookieStore.set(name, cookieValue);
  }

  createElement(tagName) {
    return new TestElement(tagName);
  }

  createDocumentFragment() {
    return new TestFragment();
  }

  getElementById(id) {
    return this.querySelectorAll("*").find((element) => element.getAttribute("id") === id) || null;
  }
}

function matchesSimpleSelector(element, selector) {
  if (!selector || selector === "*") return true;
  const tagMatch = selector.match(/^[a-zA-Z][\w-]*/);
  if (tagMatch && element.tagName.toLowerCase() !== tagMatch[0].toLowerCase()) return false;

  for (const classMatch of selector.matchAll(/\.([\w-]+)/g)) {
    if (!element.classList.contains(classMatch[1])) return false;
  }

  for (const attrMatch of selector.matchAll(/\[([^\]=]+)(?:=(["']?)([^\]"']*)\2)?\]/g)) {
    const attr = attrMatch[1];
    const expected = attrMatch[3];
    let actual = element.getAttribute(attr);
    if (actual === null && attr.startsWith("data-")) {
      actual = element.dataset[dataName(attr)] ?? null;
    }
    if (actual === null && attr === "checked") actual = element.checked ? "checked" : null;
    if (expected === undefined) {
      if (actual === null) return false;
    } else if (actual !== expected) {
      return false;
    }
  }
  return true;
}

function el(tagName, attrs = {}, children = []) {
  const element = new TestElement(tagName);
  for (const [name, value] of Object.entries(attrs)) {
    if (name === "class") {
      element.setAttribute("class", value);
    } else if (name === "text") {
      element.textContent = value;
    } else if (name === "checked") {
      element.checked = Boolean(value);
    } else if (name === "disabled") {
      element.disabled = Boolean(value);
    } else {
      element.setAttribute(name, value);
    }
  }
  element.append(...children);
  return element;
}

class TestFormData {
  constructor(form) {
    this.entriesList = [];
    if (form instanceof TestElement) {
      for (const input of form.querySelectorAll("input, select, textarea")) {
        if (!input.name || input.disabled) continue;
        if ((input.type === "checkbox" || input.type === "radio") && !input.checked) continue;
        this.append(input.name, input.value);
      }
    }
  }

  append(name, value) {
    this.entriesList.push([name, value]);
  }

  entries() {
    return this.entriesList[Symbol.iterator]();
  }

  [Symbol.iterator]() {
    return this.entries();
  }
}

class FakeXMLHttpRequest extends TestEventTarget {
  static instances = [];

  constructor() {
    super();
    this.upload = new TestEventTarget();
    this.headers = {};
    this.responseText = "";
    FakeXMLHttpRequest.instances.push(this);
  }

  open(method, url) {
    this.method = method;
    this.url = url;
  }

  setRequestHeader(name, value) {
    this.headers[name] = value;
  }

  send(body) {
    this.body = body;
  }
}

function createContext(document = new TestDocument()) {
  FakeXMLHttpRequest.instances = [];
  const intervals = new Map();
  let nextIntervalID = 1;
  const context = {
    AbortController,
    Array,
    CSS: { escape: (value) => String(value).replace(/"/g, '\\"') },
    DOMParser: class {
      parseFromString() {
        return new TestDocument();
      }
    },
    FormData: TestFormData,
    Image: class extends TestElement {
      constructor() {
        super("img");
        this.complete = true;
      }
    },
    Map,
    Node: TestElement,
    Number,
    Object,
    Promise,
    Set,
    URL,
    URLSearchParams,
    WeakMap,
    XMLHttpRequest: FakeXMLHttpRequest,
    clearInterval,
    clearTimeout,
    console,
    document,
    location: {
      href: "http://example.test/documents",
      origin: "http://example.test",
      reloadCalled: false,
      reload() {
        this.reloadCalled = true;
      },
    },
    setInterval: (callback) => {
      const id = nextIntervalID++;
      intervals.set(id, callback);
      return id;
    },
    setTimeout: () => 0,
    navigator: {
      clipboard: {
        writes: [],
        async writeText(value) {
          this.writes.push(String(value));
        },
      },
    },
  };
  context.window = context;
  context.self = context;
  context.globalThis = context;
  context.addEventListener = () => {};
  context.__runIntervals = () => {
    for (const callback of Array.from(intervals.values())) {
      callback();
    }
  };
  context.clearInterval = (id) => {
    intervals.delete(id);
  };
  context.requestAnimationFrame = (callback) => {
    callback();
    return 0;
  };
  context.cancelAnimationFrame = () => {};
  context.getComputedStyle = () => ({
    getPropertyValue: () => "",
  });
  context.matchMedia = () => ({ matches: false });
  context.sessionStorage = {
    values: new Map(),
    getItem(key) {
      return this.values.get(key) || null;
    },
    setItem(key, value) {
      this.values.set(key, String(value));
    },
    removeItem(key) {
      this.values.delete(key);
    },
  };
  return vm.createContext(context);
}

function runScripts(context, files) {
  for (const file of files) {
    const fullPath = path.join(staticDir, file);
    vm.runInContext(fs.readFileSync(fullPath, "utf8"), context, { filename: fullPath });
  }
}

function loadCore(document = new TestDocument()) {
  const context = createContext(document);
  runScripts(context, ["app.js"]);
  return context;
}

function testTagPickerUsesBackendDisplayValues() {
  const context = loadCore();
  runScripts(context, ["app-tags.js"]);
  const picker = el("div", { "data-hide-list-tags": "true", "data-empty-label": "Keine Tags" }, [
    el("div", { "data-tag-select-summary": "" }),
    el("div", { "data-tag-select-inputs": "" }),
    el("button", { "data-tag-select-trigger": "" }),
  ]);

  context.window.BearStack.tags.addTagOption({
    name: "secret",
    display_name: "Geheim",
    list_hidden: true,
  });
  context.window.BearStack.tags.addTagOption({
    name: "steuer",
    display_name: "Steuer",
  });
  context.window.BearStack.tags.setTagSelection(picker, ["secret", "steuer"]);

  const summary = picker.querySelector("[data-tag-select-summary]");
  assert.deepEqual(summary.children.map((child) => child.textContent), ["Steuer", "..."]);
  assert.equal(summary.children[1].title, "Geheim");
  assert.equal(picker.querySelector("[data-tag-select-trigger]").title, "Steuer, Geheim");
  assert.deepEqual(
    picker.querySelectorAll('input[type="hidden"]').map((input) => [input.name, input.value]),
    [["tags", "secret"], ["tags", "steuer"]],
  );
  assert.equal(context.window.BearStack.tags.normalizedTagName("  Neu, Ignoriert "), "Neu");
}

function testTagPickerFallsBackToConfiguredDisplayMode() {
  const document = new TestDocument();
  document.body.setAttribute("data-tag-display-mode", "strtoupper");
  const context = loadCore(document);
  runScripts(context, ["app-tags.js"]);
  const picker = el("div", { "data-empty-label": "Keine Tags" }, [
    el("div", { "data-tag-select-summary": "" }),
    el("div", { "data-tag-select-inputs": "" }),
    el("button", { "data-tag-select-trigger": "" }),
  ]);

  context.window.BearStack.tags.setTagSelection(picker, ["steuer"]);

  const summary = picker.querySelector("[data-tag-select-summary]");
  assert.deepEqual(summary.children.map((child) => child.textContent), ["STEUER"]);
  assert.equal(picker.querySelector("[data-tag-select-trigger]").title, "STEUER");
}

function testUploadLifecycleUsesXHRBoundary() {
  const document = new TestDocument();
  const uploadStatus = el("section", { "data-upload-status": "", class: "hidden" });
  const uploadProgress = el("progress", { "data-upload-progress": "" });
  const uploadMessage = el("div", { "data-upload-message": "" });
  const uploadList = el("ul", { "data-upload-list": "" });
  document.body.append(uploadStatus, uploadProgress, uploadMessage, uploadList);

  const context = loadCore(document);
  runScripts(context, ["app-upload.js"]);
  context.window.BearStack.upload.uploadFiles([{ name: "rechnung.pdf" }]);

  const request = FakeXMLHttpRequest.instances[0];
  assert.equal(request.method, "POST");
  assert.equal(request.url, "/upload");
  assert.equal(request.headers.Accept, "application/json");
  assert.equal(request.headers["X-Requested-With"], "XMLHttpRequest");
  assert.equal(uploadStatus.classList.contains("hidden"), false);
  assert.equal(uploadMessage.textContent, "1 Datei(en) werden hochgeladen");
  assert.equal(uploadProgress.value, 0);

  request.upload.dispatchEvent({ type: "progress", lengthComputable: true, loaded: 5, total: 10 });
  assert.equal(uploadProgress.value, 50);

  request.responseText = JSON.stringify({
    uploaded: [{ filename: "rechnung.pdf" }],
    duplicates: [{ filename: "alt.pdf" }],
    errors: [{ filename: "kaputt.pdf", error: "defekt" }],
  });
  request.dispatchEvent({ type: "load" });
  assert.equal(uploadProgress.value, 100);
  assert.equal(uploadMessage.textContent, "1 hochgeladen, 1 Duplikat(e), 1 Fehler");
  assert.deepEqual(
    uploadList.children.map((child) => child.textContent),
    ["Fehler: kaputt.pdf - defekt", "Duplikat übersprungen: alt.pdf", "Hochgeladen: rechnung.pdf"],
  );
}

function testRuleFormsUseCoreScript() {
  const document = new TestDocument();
  const form = el("form", { "data-rule-form": "", "data-dirty-form": "" });
  const list = el("div", { "data-rule-list": "" });
  const template = el("template", { "data-rule-template": "" });
  const row = el("article", { "data-rule-row": "" }, [
    el("input", { type: "hidden", name: "rule_id", value: "" }),
    el("input", { type: "text", name: "rule_label", value: "" }),
    el("button", { type: "button", "data-rule-remove": "" }),
  ]);
  template.content = new TestFragment();
  template.content.append(row);
  const add = el("button", { type: "button", "data-rule-add": "" });
  const submit = el("button", { type: "submit", class: "dirty-submit hidden", "data-dirty-submit": "" });
  form.append(list, template, add, submit);
  document.body.append(form);

  loadCore(document);

  add.dispatchEvent({ type: "click" });
  assert.equal(list.querySelectorAll("[data-rule-row]").length, 1);
  assert.equal(submit.classList.contains("hidden"), false);

  const remove = list.querySelector("[data-rule-remove]");
  form.dispatchEvent({ type: "click", target: remove });
  assert.equal(list.querySelectorAll("[data-rule-row]").length, 0);
  assert.equal(submit.classList.contains("hidden"), true);
}

function testBulkSelectionControllerUpdatesActionsAndRanges() {
  const context = loadCore();
  const form = el("form", {}, [
    el("input", { type: "checkbox", name: "ids", value: "1" }),
    el("input", { type: "checkbox", name: "ids", value: "2" }),
    el("input", { type: "checkbox", name: "ids", value: "3" }),
    el("input", { type: "checkbox", "data-select-all": "" }),
    el("button", { "data-requires-multiple": "" }),
    el("span", { "data-selection-actions": "", class: "hidden" }),
    el("span", { "data-count": "" }),
  ]);
  const [first, second, third] = form.querySelectorAll('input[name="ids"]');
  const selectAll = form.querySelector("[data-select-all]");
  const multiAction = form.querySelector("[data-requires-multiple]");
  const actions = form.querySelector("[data-selection-actions]");
  const count = form.querySelector("[data-count]");
  const controller = context.window.BearStack.core.createSelectionController(form, {
    countSelector: "[data-count]",
  });

  controller.bind();
  assert.equal(actions.classList.contains("hidden"), true);
  assert.equal(multiAction.disabled, true);

  controller.setItemChecked(first, true, { update: true });
  assert.equal(actions.classList.contains("hidden"), false);
  assert.equal(multiAction.disabled, true);
  assert.equal(selectAll.indeterminate, true);
  assert.equal(count.textContent, "1 ausgewählt");

  controller.setAll(true);
  assert.equal(multiAction.disabled, false);
  assert.equal(selectAll.checked, true);
  assert.equal(count.textContent, "3 ausgewählt");

  controller.setAnchor(first);
  controller.applyRange(third, false);
  controller.sync();
  assert.deepEqual([first.checked, second.checked, third.checked], [false, false, false]);
}

function testDocumentBatchMenuUsesDesktopAndCompactStates() {
  const document = new TestDocument();
  const menu = el("div", { "data-document-batch-menu": "", class: "document-batch-menu" }, [
    el("button", { type: "button", "data-document-batch-menu-toggle": "", "aria-expanded": "false" }),
    el("div", { class: "document-batch-menu-list" }),
  ]);
  document.body.append(el("form", { class: "table-form" }, [menu]));

  const context = loadCore(document);
  const mediaQueries = new Map();
  context.matchMedia = (query) => {
    if (!mediaQueries.has(query)) {
      const mediaQuery = {
        matches: query === "(min-width: 961px)",
        listeners: [],
        addEventListener(type, listener) {
          if (type === "change") this.listeners.push(listener);
        },
      };
      mediaQueries.set(query, mediaQuery);
    }
    return mediaQueries.get(query);
  };

  runScripts(context, ["app-documents.js", "app-preview.js"]);
  assert.equal(menu.hasAttribute("open"), false);

  const compactQuery = mediaQueries.get("(max-width: 960px)");
  compactQuery.matches = true;
  compactQuery.listeners.forEach((listener) => listener({ matches: true }));
  assert.equal(menu.hasAttribute("open"), false);

  const toggle = menu.querySelector("[data-document-batch-menu-toggle]");
  toggle.dispatchEvent({ type: "click" });
  assert.equal(menu.hasAttribute("open"), true);
  assert.equal(toggle.getAttribute("aria-expanded"), "true");

  compactQuery.matches = false;
  compactQuery.listeners.forEach((listener) => listener({ matches: false }));
  assert.equal(menu.hasAttribute("open"), false);
  assert.equal(toggle.getAttribute("aria-expanded"), "false");
}

function testDocumentExportDownloadCookieClearsBatchBusy() {
  const document = new TestDocument();
  const form = el("form", { class: "table-form", method: "get", action: "/export" }, [
    el("input", { type: "checkbox", name: "ids", value: "1", checked: true }),
    el("button", { type: "submit", text: "Exportieren" }),
  ]);
  document.body.append(form);

  const context = loadCore(document);
  runScripts(context, ["app-documents.js", "app-preview.js"]);

  const submit = form.querySelector('button[type="submit"]');
  form.dispatchEvent({ type: "submit", submitter: submit });

  assert.equal(form.classList.contains("batch-busy"), true);
  assert.equal(submit.disabled, true);
  const token = form.querySelector('input[name="download_token"]').value;
  assert.match(token, /^bs-/);

  document.cookie = `bearstack_export_download=${token}; Path=/`;
  context.__runIntervals();

  assert.equal(form.classList.contains("batch-busy"), false);
  assert.equal(submit.disabled, false);
  assert.equal(form.querySelector("[data-batch-loader]"), null);
  assert.equal(document.cookie.includes("bearstack_export_download"), false);
}

function testDocumentThumbnailLoaderStopsAfterLoad() {
  const document = new TestDocument();
  const loading = el("img", { class: "document-thumb" });
  loading.complete = false;
  loading.naturalWidth = 0;
  const loaded = el("img", { class: "document-thumb" });
  loaded.complete = true;
  loaded.naturalWidth = 96;
  document.body.append(loading, loaded);

  const context = loadCore(document);
  runScripts(context, ["app-documents.js", "app-preview.js"]);

  assert.equal(loading.classList.contains("is-loading"), true);
  assert.equal(loaded.classList.contains("is-loading"), false);

  loading.complete = true;
  loading.naturalWidth = 96;
  loading.dispatchEvent({ type: "load" });
  assert.equal(loading.classList.contains("is-loading"), false);
}

function testDocumentThumbnailLoaderUsesThumbLinkOverlay() {
  const document = new TestDocument();
  const img = el("img", { class: "document-thumb" });
  img.complete = false;
  img.naturalWidth = 0;
  const button = el("button", { class: "thumb-link" }, [img]);
  document.body.append(button);

  const context = loadCore(document);
  runScripts(context, ["app-documents.js", "app-preview.js"]);

  assert.equal(button.classList.contains("is-loading"), true);
  assert.equal(img.classList.contains("is-loading"), false);
  img.complete = true;
  img.naturalWidth = 96;
  img.dispatchEvent({ type: "load" });
  assert.equal(button.classList.contains("is-loading"), false);
}

async function testShareButtonCopiesReadOnlyLinkAndShowsToast() {
  const document = new TestDocument();
  const button = el("button", { "data-share-url": "/documents/7/view" });
  document.body.append(button);

  const context = loadCore(document);
  runScripts(context, ["app-documents.js", "app-preview.js"]);
  let toastMessage = "";
  context.window.showAppToast = (message) => {
    toastMessage = String(message || "");
  };

  button.dispatchEvent({ type: "click" });
  await new Promise((resolve) => setImmediate(resolve));

  assert.deepEqual(context.navigator.clipboard.writes, ["http://example.test/documents/7/view"]);
  assert.equal(toastMessage, "Sharing-Link kopiert.");
}

function pdfViewerTestRoot() {
  return el("section", { "data-pdf-preview": "", hidden: true }, [
    el("button", { "data-pdf-previous": "" }),
    el("button", { "data-pdf-next": "" }),
    el("button", { "data-pdf-zoom-out": "" }),
    el("button", { "data-pdf-zoom-in": "" }),
    el("button", { "data-pdf-fit-width": "" }),
    el("button", { "data-pdf-fit-page": "" }),
    el("input", { "data-pdf-page": "", value: "1" }),
    el("span", { "data-pdf-pages": "" }),
    el("span", { "data-pdf-zoom": "" }),
    el("a", { "data-pdf-native": "" }),
    el("a", { "data-pdf-download": "" }),
    el("div", { "data-pdf-status": "" }),
    el("div", { "data-pdf-scroll": "" }, [
      el("div", { "data-pdf-page-container": "" }, [
        el("canvas", { "data-pdf-canvas": "" }),
        el("div", { "data-pdf-text-layer": "" }),
        el("div", { "data-pdf-annotation-layer": "" }),
      ]),
    ]),
  ]);
}

async function testPDFPreviewIsSelectedLazilyOnlyWhenEnabled() {
  const document = new TestDocument();
  document.body.setAttribute("data-custom-pdf-preview", "true");
  const frame = el("iframe", { "data-preview-frame": "", hidden: true });
  const image = el("img", { "data-preview-image": "", hidden: true });
  const pdfRoot = pdfViewerTestRoot();
  const modal = el("dialog", { "data-preview-modal": "" }, [
    el("span", { "data-preview-title": "" }),
    el("button", { "data-preview-close": "" }),
    el("div", {}, [frame, image, pdfRoot]),
  ]);
  const button = el("button", {
    "data-preview-url": "/documents/42/preview",
    "data-preview-mime": "application/pdf",
    "data-preview-title": "Test.pdf",
  });
  document.body.append(modal, button);
  const context = loadCore(document);
  runScripts(context, ["app-documents.js", "app-preview.js"]);
  const calls = [];
  context.window.bearStackPDFPreview = {
    destroy(root) { calls.push(["destroy", root]); },
    load(root, url, title, download) {
      calls.push(["load", root, url, title, download]);
      return Promise.resolve();
    },
  };
  button.dispatchEvent({ type: "click" });
  assert.equal(frame.hidden, true);
  assert.deepEqual(calls.at(-1).slice(0, 5), ["load", pdfRoot, "/documents/42/preview", "Test.pdf", "/documents/42/download"]);

  context.window.bearStackPDFPreview.load = () => Promise.reject(new Error("unsupported PDF"));
  button.dispatchEvent({ type: "click" });
  await new Promise((resolve) => setImmediate(resolve));
  assert.equal(frame.hidden, false);
  assert.equal(frame.src, "/documents/42/preview");
}

function testDocumentDetailUsesCustomPDFPreview() {
  const document = new TestDocument();
  document.body.setAttribute("data-custom-pdf-preview", "true");
  const frame = el("iframe", { "data-detail-preview-frame": "", hidden: true });
  const pdfRoot = pdfViewerTestRoot();
  const target = el("div", {
    "data-detail-preview": "",
    "data-detail-preview-url": "/documents/1234/preview",
    "data-detail-preview-mime": "application/pdf",
    "data-detail-preview-title": "Detail.pdf",
  }, [frame, pdfRoot]);
  document.body.append(target);

  const context = loadCore(document);
  const calls = [];
  context.window.bearStackPDFPreview = {
    destroy(root) { calls.push(["destroy", root]); },
    load(root, url, title, download) {
      calls.push(["load", root, url, title, download]);
      return Promise.resolve();
    },
  };
  runScripts(context, ["app-documents.js", "app-preview.js"]);

  assert.equal(frame.hidden, true);
  assert.deepEqual(calls.at(-1), [
    "load",
    pdfRoot,
    "/documents/1234/preview",
    "Detail.pdf",
    "/documents/1234/download",
  ]);

  target.dispatchEvent({ type: "click", target: pdfRoot.querySelector("[data-pdf-fit-page]") });
  assert.equal(calls.filter(([action]) => action === "load").length, 1);
}

async function testPDFPreviewControlsAndDestroyCancelWork() {
  const document = new TestDocument();
  const root = pdfViewerTestRoot();
  document.body.append(root);
  const context = createContext(document);
  runScripts(context, ["app-pdf-preview.js"]);
  const controller = new context.window.BearStackPDFPreviewController(root);
  controller.pdf = { numPages: 3 };
  let renders = 0;
  controller.renderPage = () => { renders += 1; return Promise.resolve(); };
  root.querySelector("[data-pdf-next]").dispatchEvent({ type: "click" });
  assert.equal(controller.pageNumber, 2);
  root.querySelector("[data-pdf-zoom-in]").dispatchEvent({ type: "click" });
  assert.equal(controller.scaleMode, "manual");
  assert.ok(controller.scale > 1);
  root.querySelector("[data-pdf-fit-page]").dispatchEvent({ type: "click" });
  assert.equal(controller.scaleMode, "page-fit");
  assert.equal(renders, 3);

  let renderCancelled = false;
  let textCancelled = false;
  let loadingDestroyed = false;
  controller.renderTask = { cancel() { renderCancelled = true; } };
  controller.textTask = { cancel() { textCancelled = true; } };
  controller.loadingTask = { async destroy() { loadingDestroyed = true; } };
  await controller.destroy();
  assert.equal(renderCancelled, true);
  assert.equal(textCancelled, true);
  assert.equal(loadingDestroyed, true);
  assert.equal(root.hidden, true);
}

function testPhotoMediaHelpersWorkWithoutGallery() {
  const document = new TestDocument();
  const context = createContext(document);
  context.innerWidth = 1000;
  context.innerHeight = 700;
  context.devicePixelRatio = 1;
  runScripts(context, ["app-photos-media.js", "app-photos-frame.js"]);

  const root = el("div", {}, [
    el("article", {
      "data-photo-item": "",
      "data-photo-path": "album/photo.jpg",
      "data-photo-thumb": "/photos/thumbnail?size=320",
      "data-photo-preview": "/photos/thumbnail?size=960",
      "data-photo-large-preview": "/photos/thumbnail?size=1600",
      "data-photo-src": "/photos/media?path=album%2Fphoto.jpg",
      "data-photo-title": "Albumfoto",
      "data-photo-width": "3000",
      "data-photo-height": "2000",
    }),
    el("article", {
      "data-photo-item": "",
      "data-photo-type": "video",
      "data-photo-src": "/photos/media?path=clip.mp4",
      "data-photo-title": "Clip",
    }),
  ]);

  const items = context.window.BearStack.photos.collectItems(root);
  assert.equal(items.length, 2);
  assert.equal(items[0].title, "Albumfoto");
  assert.equal(context.window.BearStack.photos.bestPhotoDisplaySrc(items[0]), "/photos/thumbnail?size=1600");
  context.devicePixelRatio = 2;
  assert.equal(context.window.BearStack.photos.bestPhotoDisplaySrc(items[0]), "/photos/thumbnail?size=1600");
  context.innerWidth = 300;
  context.innerHeight = 200;
  context.devicePixelRatio = 1;
  assert.equal(context.window.BearStack.photos.bestPhotoDisplaySrc(items[0]), "/photos/thumbnail?size=320");
  const details = context.window.BearStack.photos.applyPhotoItemDetails({}, {
    path: "album/photo.jpg", large_preview: "/large", date_time: "18.05.2026 12:00", width: "3000", height: "2000"
  });
  assert.equal(details.largePreview, "/large");
  assert.equal(details.dateTime, "18.05.2026 12:00");
  assert.equal(details.width, 3000);
  assert.equal(details.detailsLoaded, true);
  assert.equal(context.window.BearStack.photos.bestPhotoDisplaySrc({ type: "image", original: "/original" }), "/original");
  assert.equal(items[1].src, "/photos/media?path=clip.mp4");
  assert.equal(context.window.BearStack.photos.formatPhotoRating("2.5"), "2,5 Sterne");
}

function peopleSelectionControls() {
  return el("div", { "data-people-merge": "" }, [
    el("span", { "data-people-merge-target": "" }),
    el("button", { "data-people-edit-button": "" }),
    el("button", { "data-people-merge-button": "" }),
  ]);
}

async function testPeopleRefreshRetainsImagesWhenCountsChange() {
  const document = new TestDocument();
  const img = el("img", { src: "/photos/faces/10/thumbnail" });
  let imageWrites = 0;
  Object.defineProperty(img, "src", { set(value) { imageWrites++; this.setAttribute("src", value); } });
  const count = el("span", { text: "1 Foto" });
  const title = el("strong", { text: "Alt" });
  const ignore = el("button", { "data-ignore-face": "10" });
  const checkbox = el("input", { "data-person-select": "", value: "1" });
  const card = el("div", { "data-person-id": "1", class: "person-overview-card" }, [
    el("a", { class: "person-card" }, [img, title, count]), checkbox, ignore,
  ]);
  const query = card.querySelector.bind(card);
  card.querySelector = (selector) => selector === ".person-card > span" ? count : query(selector);
  const overview = el("div", { "data-people-overview": "", "data-can-ignore": "true" }, [card]);
  const status = el("p", { "data-people-status": "" });
  document.body.append(overview, status,
    peopleSelectionControls(),
    el("div", { class: "people-pagination" }));
  const context = createContext(document);
  context.location.href = "http://example.test/photos/people";
  context.history = { state: null, replaceState() {} };
  context.fetch = async (url, options) => ({ ok: true, json: async () => options.method === "POST" ?
    { ok: true } : { people: [{ id: 1, face_id: 10, name: "Neu", count: 2 }], page: 2, total_pages: 3, has_prev: true, has_next: true } });
  runScripts(context, ["app-people.js"]);
  overview.dispatchEvent({ type: "click", target: ignore });
  await new Promise((resolve) => setImmediate(resolve));
  assert.equal(status.textContent, "Gesicht ignoriert.");
  assert.equal(overview.children[0], card);
  assert.equal(card.querySelector("img"), img);
  assert.equal(imageWrites, 0);
  assert.equal(count.textContent, "2 Fotos");
  assert.equal(title.textContent, "Neu");
  assert.equal(checkbox.getAttribute("aria-label"), "Person auswählen: Neu");
  const links = document.querySelector(".people-pagination").children;
  assert.equal(links[0].textContent, "Erste Seite");
  assert.equal(new URL(links[0].href, "http://example.test").searchParams.get("page"), "1");
  assert.equal(links[links.length - 1].textContent, "Letzte Seite");
  assert.equal(new URL(links[links.length - 1].href, "http://example.test").searchParams.get("page"), "3");
}

function testPeopleRemembersPageAndHonorsExplicitFilters() {
  const key = "bearstack.people.lastPage:manager";
  const stored = JSON.stringify({ page: 7, q: "Petra", known: true, ignored: true });
  function setup(href, initial = stored, blocked = false) {
    const document = new TestDocument();
    document.body.append(el("nav", { "data-people-page": "2", "data-people-user": "manager" }));
    const context = createContext(document);
    const values = new Map([[key, initial]]);
    context.location.href = href;
    context.location.replace = (url) => { context.redirect = url; };
    context.localStorage = { getItem: (k) => values.get(k), setItem: (k, value) => values.set(k, value) };
    if (blocked) context.localStorage = { getItem() { throw new Error("blocked"); }, setItem() { throw new Error("blocked"); } };
    runScripts(context, ["app-people.js"]);
    return { context, values };
  }
  const restored = setup("http://example.test/photos/people");
  const destination = new URL(restored.context.redirect, "http://example.test");
  assert.equal(destination.searchParams.get("page"), "7");
  assert.equal(destination.searchParams.get("q"), "Petra");
  assert.equal(destination.searchParams.get("known"), "1");
  assert.equal(destination.searchParams.get("ignored"), "1");
  for (const query of ["?page=2", "?q=", "?ignored=1", "?known=1"]) {
    const explicit = setup("http://example.test/photos/people" + query);
    assert.equal(explicit.context.redirect, undefined);
    assert.equal(JSON.parse(explicit.values.get(key)).page, 2);
  }
  const invalid = setup("http://example.test/photos/people", '{"page":-1,"q":"x"}');
  assert.equal(invalid.context.redirect, undefined);
  const broken = setup("http://example.test/photos/people", "not json");
  assert.equal(broken.context.redirect, undefined);
  assert.equal(setup("http://example.test/photos/people", stored, true).context.redirect, undefined);
}

async function testPeopleMergePrefersNamedSelection() {
  for (const names of [["", "Petra", ""], ["", "", "Petra"], ["Petra", "Marie", ""], ["", "", ""]]) {
    const document = new TestDocument();
    const inputs = names.map((name, index) => el("input", { "data-person-select": "", value: String(index + 1) }));
    const cards = names.map((name, index) => el("div", { "data-person-id": String(index + 1), "data-person-name": name }, [el("strong", { text: name || "Unbenannt" }), inputs[index]]));
    const overview = el("div", { "data-people-overview": "", "data-can-ignore": "true" }, cards);
    const controls = peopleSelectionControls();
    const target = controls.querySelector("[data-people-merge-target]");
    const button = controls.querySelector("[data-people-merge-button]");
    document.body.append(overview, el("p", { "data-people-status": "" }), controls);
    const context = createContext(document);
    let request;
    context.fetch = async (url, options) => { request = { url, options }; return { ok: false, status: 503 }; };
    runScripts(context, ["app-people.js"]);
    inputs.forEach((input) => { input.checked = true; overview.dispatchEvent({ type: "change", target: input }); });
    const expected = Math.max(0, names.findIndex(Boolean));
    assert.equal(target.textContent, "3 ausgewählt · Ziel: " + (names[expected] || "Unbenannt"));
    button.dispatchEvent({ type: "click" });
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(request.options.body.get("target"), String(expected + 1));
    const merged = [request.url.match(/people\/(\d+)\/merge/)[1], ...request.options.body.getAll("person_id")];
    assert.deepEqual(merged.sort(), ["1", "2", "3"].filter((id) => id !== String(expected + 1)));
  }
}

const tests = [
  testPeopleMergePrefersNamedSelection,
  testPeopleRemembersPageAndHonorsExplicitFilters,
  testPeopleRefreshRetainsImagesWhenCountsChange,
  testTagPickerUsesBackendDisplayValues,
  testTagPickerFallsBackToConfiguredDisplayMode,
  testUploadLifecycleUsesXHRBoundary,
  testRuleFormsUseCoreScript,
  testBulkSelectionControllerUpdatesActionsAndRanges,
  testDocumentBatchMenuUsesDesktopAndCompactStates,
  testDocumentExportDownloadCookieClearsBatchBusy,
  testDocumentThumbnailLoaderStopsAfterLoad,
  testDocumentThumbnailLoaderUsesThumbLinkOverlay,
  testShareButtonCopiesReadOnlyLinkAndShowsToast,
  testPDFPreviewIsSelectedLazilyOnlyWhenEnabled,
  testDocumentDetailUsesCustomPDFPreview,
  testPDFPreviewControlsAndDestroyCancelWork,
  testPhotoMediaHelpersWorkWithoutGallery,
];

for (const test of tests) {
  await test();
  console.log(`ok ${test.name}`);
}
