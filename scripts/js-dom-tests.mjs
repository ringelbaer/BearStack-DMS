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

  requestSubmit() {
    this.dispatchEvent({ type: "submit" });
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
    setInterval: () => 0,
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

function testPhotoHelpersExposeLightboxInputs() {
  const document = new TestDocument();
  const context = createContext(document);
  context.innerWidth = 1000;
  context.innerHeight = 700;
  context.devicePixelRatio = 1;
  runScripts(context, ["app-photos-map.js", "app-photos-thumbnails.js", "app-photos.js", "app-photos-frame.js"]);

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
  assert.equal(items[1].src, "/photos/media?path=clip.mp4");
  assert.equal(context.window.BearStack.photos.formatPhotoRating("2.5"), "2,5 Sterne");
}

const tests = [
  testTagPickerUsesBackendDisplayValues,
  testTagPickerFallsBackToConfiguredDisplayMode,
  testUploadLifecycleUsesXHRBoundary,
  testRuleFormsUseCoreScript,
  testBulkSelectionControllerUpdatesActionsAndRanges,
  testDocumentThumbnailLoaderStopsAfterLoad,
  testDocumentThumbnailLoaderUsesThumbLinkOverlay,
  testShareButtonCopiesReadOnlyLinkAndShowsToast,
  testPhotoHelpersExposeLightboxInputs,
];

for (const test of tests) {
  await test();
  console.log(`ok ${test.name}`);
}
