#!/usr/bin/env node
import assert from "node:assert/strict";
import familyTreeLayout from "../internal/server/static/app-family-tree-layout.js";
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

function testTagModuleReadsOptionsWhenItLoads() {
  const document = new TestDocument();
  const context = loadCore(document);
  // Feature markup may arrive after the shared core has initialized.
  document.body.append(el("span", {
    "data-tag-option": "", "data-name": "protected", "data-display-name": "Geschützt",
    "data-delete-protected": "true",
  }));
  runScripts(context, ["app-tags.js"]);
  const tags = context.window.BearStack.tags;
  assert.equal(tags.displayTagName("protected"), "Geschützt");
  assert.equal(tags.isDeleteProtected("protected"), true);
  assert.equal(tags.isDeleteProtected("unknown"), false);
  tags.addTagOption({ name: "protected", display_name: "Freigegeben", delete_protected: false });
  assert.equal(tags.isDeleteProtected("protected"), false);
  assert.equal(tags.displayTagName("protected"), "Freigegeben");
}

async function testDocumentMetadataUsesTagModuleProtection() {
  for (const protectedTag of [false, true]) {
    const document = new TestDocument();
    document.body.dataset.documentDeleteProtected = "false";
    const picker = el("div", { "data-tag-select": "" }, [
      el("div", { "data-tag-select-summary": "" }),
      el("div", { "data-tag-select-inputs": "" }),
      el("button", { "data-tag-select-trigger": "" }),
    ]);
    const message = el("div", { "data-metadata-message": "" });
    const form = el("form", { "data-metadata-form": "", action: "/documents/1/metadata" }, [
      picker, message, el("button", { type: "submit" }),
    ]);
    document.body.append(form);
    const context = loadCore(document);
    runScripts(context, ["app-tags.js", "app-documents.js"]);
    context.window.BearStack.tags.addTagOption({ name: "updated", display_name: "Aktualisiert", delete_protected: protectedTag });
    context.fetch = async () => ({ ok: true, json: async () => ({ tags: ["updated"] }) });
    form.dispatchEvent({ type: "submit" });
    await new Promise(setImmediate);
    assert.equal(picker.querySelector("[data-tag-select-summary]").children[0].textContent, "Aktualisiert");
    assert.equal(context.location.reloadCalled, protectedTag);
    if (!protectedTag) assert.equal(message.textContent, "Metadaten gespeichert.");
  }
}

function testUploadLifecycleUsesXHRBoundary() {
  const document = new TestDocument();
  const uploadStatus = el("section", { "data-upload-status": "", class: "hidden" });
  const uploadProgress = el("progress", { "data-upload-progress": "" });
  const uploadMessage = el("div", { "data-upload-message": "" });
  const uploadList = el("ul", { "data-upload-list": "" });
  const minimize = el("button", { "data-upload-minimize": "" });
  document.body.append(uploadStatus, uploadProgress, uploadMessage, uploadList, minimize);

  const context = createContext(document);
  runScripts(context, ["app-upload.js"]);
  minimize.dispatchEvent({ type: "click" });
  assert.equal(uploadStatus.classList.contains("minimized"), true);
  context.window.BearStack.upload.uploadFiles([{ name: "rechnung.pdf" }]);
  assert.equal(uploadStatus.classList.contains("minimized"), false);

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
  context.BearStack.upload.uploadFiles([{ name: "offline.pdf" }]);
  FakeXMLHttpRequest.instances[1].dispatchEvent({ type: "error" });
  assert.equal(uploadMessage.textContent, "Upload fehlgeschlagen");
  assert.equal(uploadList.children[0].textContent, "Netzwerkfehler beim Hochladen");
}

async function testUploadRefreshUsesDocumentModuleAndFallback() {
  for (const mode of ["refresh", "unavailable", "failed", "absent"]) {
    const document = new TestDocument();
    document.body.append(el("div", { "data-upload-message": "" }));
    const context = createContext(document);
    const timers = [];
    context.setTimeout = (callback) => { timers.push(callback); return timers.length; };
    runScripts(context, ["app-upload.js"]);
    let refreshes = 0;
    if (mode !== "absent") context.BearStack.documents = { async refreshList() {
      refreshes++;
      if (mode === "failed") throw new Error("offline");
      return mode === "refresh";
    } };
    context.BearStack.upload.uploadFiles([{ name: "invoice.pdf" }]);
    const request = FakeXMLHttpRequest.instances[0];
    request.responseText = JSON.stringify({ uploaded: [{ filename: "invoice.pdf" }] });
    request.dispatchEvent({ type: "load" });
    assert.equal(timers.length, 1);
    await timers.shift()();
    assert.equal(refreshes, mode === "absent" ? 0 : 1);
    assert.equal(context.location.reloadCalled, mode !== "refresh", mode);
  }
}

function ocrFixture({ terminal = false, dismissed = false } = {}) {
  const document = new TestDocument();
  const state = el("span", { "data-ocr-state": "" });
  const progress = el("progress", { "data-ocr-progress": "" });
  const progressRow = el("div", { "data-ocr-progress-row": "" }, [progress]);
  const progressText = el("span", { "data-ocr-progress-text": "" });
  const message = el("p", { "data-ocr-message": "" });
  const dismiss = el("button", { "data-ocr-dismiss": "" });
  const panel = el("section", {
    "data-ocr-status": "", "data-ocr-status-url": "/documents/1/ocr/status",
    "data-ocr-job-id": "42", "data-ocr-active": terminal ? "0" : "1", "data-ocr-terminal": terminal ? "1" : "0",
  }, [state, progressRow, progressText, message, dismiss]);
  document.body.append(panel);
  const context = createContext(document), timers = [];
  context.setTimeout = (callback, delay) => { timers.push({ callback, delay }); return timers.length; };
  if (dismissed) context.sessionStorage.setItem("bearstack.ocr.dismissed.42", "1");
  return { context, timers, panel, state, progress, progressRow, progressText, message, dismiss,
    async tick() { const timer = timers.shift(); assert.ok(timer); timer.callback(); await new Promise(setImmediate); },
  };
}

async function testOCRPollingAndCompletionOwnTheirState() {
  const f = ocrFixture();
  let requests = 0;
  f.context.fetch = async (url, options) => {
    assert.equal(url, "/documents/1/ocr/status");
    assert.equal(options.credentials, "same-origin");
    requests++;
    return { ok: true, json: async () => ({ job: requests === 1
      ? { id: 42, active: true, terminal: false, status: "running", status_text: "Läuft", progress_percent: 50, current_page: 1, total_pages: 2 }
      : { id: 42, active: false, terminal: true, status: "completed", status_text: "Fertig", progress_percent: 100, text_length: 321 },
    }) };
  };
  runScripts(f.context, ["app-ocr.js"]);
  assert.equal(f.timers[0].delay, 1000);
  await f.tick();
  assert.equal(f.state.textContent, "Läuft");
  assert.equal(f.progress.value, 50);
  assert.equal(f.progressText.textContent, "1 von 2");
  assert.equal(f.dismiss.hidden, true);
  assert.equal(f.timers[0].delay, 2000);
  await f.tick();
  assert.equal(f.panel.classList.contains("ocr-status-completed"), true);
  assert.equal(f.message.textContent, "321 Zeichen wurden in den Textinhalt übernommen.");
  assert.equal(f.progressRow.hidden, true);
  assert.equal(f.dismiss.hidden, false);
  assert.equal(f.context.sessionStorage.getItem("bearstack.ocr.dismissed.42"), "1");
  assert.equal(f.timers[0].delay, 900);
  await f.tick();
  assert.equal(f.context.location.reloadCalled, true);
  assert.equal(f.timers.length, 0);
  assert.equal(requests, 2);
}

async function testOCRRetriesAndRemembersDismissal() {
  for (const failure of ["network", "http"]) {
    const f = ocrFixture();
    let calls = 0;
    f.context.fetch = async () => {
      if (++calls === 1) {
        if (failure === "network") throw new Error("offline");
        return { ok: false };
      }
      return { ok: true, json: async () => ({ job: { id: 42, active: false, terminal: true, status: "failed", error: "Konverterfehler" } }) };
    };
    runScripts(f.context, ["app-ocr.js"]);
    await f.tick();
    assert.equal(f.timers[0].delay, 5000);
    await f.tick();
    assert.equal(f.message.textContent, "Konverterfehler");
    assert.equal(f.panel.hidden, false);
    f.dismiss.dispatchEvent({ type: "click" });
    assert.equal(f.panel.hidden, true);
    assert.equal(f.context.sessionStorage.getItem("bearstack.ocr.dismissed.42"), "1");
    assert.equal(f.timers.length, 0);
  }
  const restored = ocrFixture({ terminal: true, dismissed: true });
  runScripts(restored.context, ["app-ocr.js"]);
  assert.equal(restored.panel.hidden, true);
  assert.equal(restored.timers.length, 0);
  const empty = createContext();
  empty.setTimeout = () => { throw new Error("OCR polling without a panel"); };
  runScripts(empty, ["app-ocr.js"]);
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
      "data-photo-folder-name": "Album",
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
  assert.equal(items[0].folderName, "Album");
  assert.equal(items[1].folderName, "-");
  assert.equal(context.window.BearStack.photos.bestPhotoDisplaySrc(items[0]), "/photos/thumbnail?size=1600");
  context.devicePixelRatio = 2;
  assert.equal(context.window.BearStack.photos.bestPhotoDisplaySrc(items[0]), "/photos/thumbnail?size=1600");
  context.innerWidth = 300;
  context.innerHeight = 200;
  context.devicePixelRatio = 1;
  assert.equal(context.window.BearStack.photos.bestPhotoDisplaySrc(items[0]), "/photos/thumbnail?size=320");
  const details = context.window.BearStack.photos.applyPhotoItemDetails({}, {
    path: "album/photo.jpg", folder_name: "Sommer Urlaub", large_preview: "/large", date_time: "18.05.2026 12:00", width: "3000", height: "2000"
  });
  assert.equal(details.largePreview, "/large");
  assert.equal(details.dateTime, "18.05.2026 12:00");
  assert.equal(details.folderName, "Sommer Urlaub");
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
  const count = el("span", { "data-person-count": "", text: "1 Foto" });
  const title = el("strong", { text: "Alt" });
  const ignore = el("button", { "data-ignore-face": "10" });
  const checkbox = el("input", { "data-person-select": "", value: "1" });
  const card = el("div", { "data-person-id": "1", class: "person-overview-card" }, [
    el("a", { class: "person-card" }, [img, title, count]), checkbox, ignore,
  ]);
  const overview = el("div", { "data-people-overview": "", "data-can-ignore": "true" }, [card]);
  const status = el("p", { "data-people-status": "" });
  document.body.append(overview, status,
    peopleSelectionControls(),
    el("div", { class: "people-pagination" }));
  const context = createContext(document);
  context.location.href = "http://example.test/photos/people?unknown=1&sort=count_desc";
  context.history = { state: null, replaceState() {} };
  let pageData = { page: 2, total_pages: 3, has_prev: true, has_next: true };
  context.fetch = async (url, options) => ({ ok: true, json: async () => options.method === "POST" ?
    { ok: true } : { people: [{ id: 1, face_id: 10, name: "Neu", count: 2 }], ...pageData } });
  runScripts(context, ["app-person-picker.js", "app-person-dialog.js", "app-people-controls.js", "app-people.js"]);
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
  assert.equal(links[0].getAttribute("aria-label"), "Erste Seite");
  assert.equal(new URL(links[0].href, "http://example.test").searchParams.get("page"), "1");
  assert.equal(new URL(links[0].href, "http://example.test").searchParams.get("unknown"), "1");
  assert.equal(new URL(links[0].href, "http://example.test").searchParams.get("sort"), "count_desc");
  assert.equal(links[links.length - 1].getAttribute("aria-label"), "Letzte Seite");
  assert.equal(new URL(links[links.length - 1].href, "http://example.test").searchParams.get("page"), "3");
  assert.equal(new URL(links[links.length - 1].href, "http://example.test").searchParams.get("unknown"), "1");
  // Refreshes can shrink the result to one page and later restore navigation.
  for (const [page, total, labels] of [
    [1, 1, ["Seite 1 von 1"]],
    [1, 3, ["Erste Seite", "Seite 1 von 3", "Weiter", "Letzte Seite"]],
    [3, 3, ["Erste Seite", "Zurück", "Seite 3 von 3", "Letzte Seite"]],
  ]) {
    pageData = { page, total_pages: total, has_prev: page > 1, has_next: page < total };
    overview.dispatchEvent({ type: "click", target: ignore });
    await new Promise((resolve) => setImmediate(resolve));
    const controls = document.querySelector(".people-pagination").children;
    assert.deepEqual(Array.from(controls, control => control.getAttribute("aria-label")), labels);
    if (total > 1) assert.equal(controls[page === 1 ? 0 : controls.length - 1].getAttribute("aria-disabled"), "true");
    const counter = Array.from(controls).find(control => control.getAttribute("aria-current") === "page");
    assert.equal(counter.dataset.shortPage, `${page} / ${total}`);
    for (const control of Array.from(controls).filter(control => control !== counter)) {
      assert.equal(control.children[0].getAttribute("aria-hidden"), "true");
      assert.equal(control.children[1].textContent, control.getAttribute("aria-label"));
    }
    assert.equal(imageWrites, 0);
  }
}

function testPeopleRemembersPageAndHonorsExplicitFilters() {
  const key = "bearstack.people.lastPage:manager";
  const stored = JSON.stringify({ page: 7, q: "Petra", known: true, ignored: true, sort: "date_desc" });
  function setup(href, initial = stored, blocked = false) {
    const document = new TestDocument();
    document.body.append(el("nav", { "data-people-page": "2", "data-people-user": "manager" }));
    const context = createContext(document);
    const values = new Map([[key, initial]]);
    context.location.href = href;
    context.location.replace = (url) => { context.redirect = url; };
    context.localStorage = { getItem: (k) => values.get(k), setItem: (k, value) => values.set(k, value) };
    if (blocked) context.localStorage = { getItem() { throw new Error("blocked"); }, setItem() { throw new Error("blocked"); } };
    runScripts(context, ["app-person-picker.js", "app-person-dialog.js", "app-people-controls.js", "app-people.js"]);
    return { context, values };
  }
  const restored = setup("http://example.test/photos/people");
  const destination = new URL(restored.context.redirect, "http://example.test");
  assert.equal(destination.searchParams.get("page"), "7");
  assert.equal(destination.searchParams.get("q"), "Petra");
  assert.equal(destination.searchParams.get("known"), "1");
  assert.equal(destination.searchParams.get("ignored"), "1");
  assert.equal(destination.searchParams.get("sort"), "date_desc");
  for (const query of ["?sort=name_asc", "?sort=count_desc", "?page=2", "?q=", "?ignored=1", "?known=1", "?unknown=1", "?unknown=0", "?filter=all", "?filter=known", "?filter=unknown", "?filter=ignored"]) {
    const explicit = setup("http://example.test/photos/people" + query);
    assert.equal(explicit.context.redirect, undefined);
    assert.equal(JSON.parse(explicit.values.get(key)).page, 2);
  }
  const unknownState = JSON.stringify({ page: 2, q: "", unknown: true, known: true, ignored: true });
  const unknown = new URL(setup("http://example.test/photos/people", unknownState).context.redirect, "http://example.test");
  assert.equal(unknown.searchParams.get("unknown"), "1");
  assert.equal(unknown.searchParams.has("known"), false);
  assert.equal(unknown.searchParams.has("ignored"), false);
  const reset = setup("http://example.test/photos/people?page=1&q=", unknownState);
  assert.equal(reset.context.redirect, undefined);
  assert.equal(JSON.parse(reset.values.get(key)).unknown, false);
  assert.equal(JSON.parse(reset.values.get(key)).sort, "name_asc");
  assert.equal(JSON.parse(setup("http://example.test/photos/people?sort=folder_desc").values.get(key)).sort, "folder_desc");
  const invalidSort = new URL(setup("http://example.test/photos/people", JSON.stringify({page:1,q:"",sort:"unsafe"})).context.redirect, "http://example.test");
  assert.equal(invalidSort.searchParams.has("sort"), false);
  const explicitUnknown = setup("http://example.test/photos/people?unknown=1&known=1&ignored=1");
  assert.equal(JSON.parse(explicitUnknown.values.get(key)).unknown, true);
  assert.equal(JSON.parse(explicitUnknown.values.get(key)).known, false);
  assert.equal(JSON.parse(explicitUnknown.values.get(key)).ignored, false);
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
    const cards = names.map((name, index) => el("div", { class: "person-overview-card", "data-person-id": String(index + 1), "data-person-name": name }, [el("strong", { text: name || "Unbenannt" }), inputs[index]]));
    const overview = el("div", { "data-people-overview": "", "data-can-ignore": "true" }, cards);
    const controls = peopleSelectionControls();
    const target = controls.querySelector("[data-people-merge-target]");
    const button = controls.querySelector("[data-people-merge-button]");
    document.body.append(overview, el("p", { "data-people-status": "" }), controls);
    const context = createContext(document);
    let request;
    context.showAppConfirm = async () => true;
    context.fetch = async (url, options) => { request = { url, options }; return { ok: false, status: 503 }; };
    runScripts(context, ["app-person-picker.js", "app-person-dialog.js", "app-people-controls.js", "app-people.js"]);
    inputs.forEach((input) => { input.checked = true; overview.dispatchEvent({ type: "change", target: input }); });
    const expected = Math.max(0, names.findIndex(Boolean));
    assert.equal(target.textContent, "3 Gruppen auf dieser Seite · Zusammenführen mit: " + (names[expected] || "Unbenannt"));
    button.dispatchEvent({ type: "click" });
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(request.options.body.get("target"), String(expected + 1));
    const merged = [request.url.match(/people\/(\d+)\/merge/)[1], ...request.options.body.getAll("person_id")];
    assert.deepEqual(merged.sort(), ["1", "2", "3"].filter((id) => id !== String(expected + 1)));
  }
}

function frameFixture({ total = 605, hidden = false, delayPage = 0, video = false } = {}) {
  const document = new TestDocument();
  document.hidden = hidden;
  const image = el("img", { "data-photo-frame-image": "" });
  const movie = el("video", { "data-photo-frame-video": "", hidden: true });
  let plays = 0, pauses = 0;
  movie.pause = () => { pauses++; };
  movie.play = () => { plays++; return Promise.resolve(); };
  const title = el("span", { "data-photo-frame-title": "" });
  const count = el("span", { "data-photo-frame-count": "" });
  document.body.append(el("div", { "data-photo-frame": "", "data-photo-frame-items-url": "/photos/frame/items" }, [image, movie, title, count]));
  const context = createContext(document);
  const windowEvents = new Map();
  context.addEventListener = (name, callback) => windowEvents.set(name, callback);
  const requests = [];
  let downloads = 0, aborted = 0, release;
  context.Image = class extends TestElement {
    constructor() { super("img"); }
    set src(value) { this.setAttribute("src", value); downloads++; this.onload?.(); }
  };
  context.BearStack = { photos: {
    applyPhotoItemDetails: (item, data) => Object.assign(item, data),
    bestPhotoDisplaySrc: (item) => item.src,
  } };
  context.fetch = (url, options) => {
    const page = Number(new URL(url).searchParams.get("page"));
    requests.push(page);
    const response = () => ({ ok: true, json: async () => ({ total, has_next: page * 200 < total,
      media: Array.from({ length: Math.max(0, Math.min(200, total - (page - 1) * 200)) }, (_, i) => {
        const id = (page - 1) * 200 + i;
        return { title: String(id), type: video ? "video" : "image", src: "/image/" + id };
      }) }) });
    if (page !== delayPage) return Promise.resolve(response());
    return new Promise((resolve, reject) => {
      release = () => resolve(response());
      options.signal.addEventListener("abort", () => { aborted++; reject(Object.assign(new Error("aborted"), { name: "AbortError" })); });
    });
  };
  // Observe the actual frame array's peak size without retaining its entries.
  vm.runInContext(`globalThis.framePeak = 0;
    const arrayPrototype = Object.getPrototypeOf([]);
    const originalPush = arrayPrototype.push;
    arrayPrototype.push = function(...values) {
      const length = originalPush.apply(this, values);
      if (values[0]?.detailsLoaded) globalThis.framePeak = Math.max(globalThis.framePeak, length);
      return length;
    };`, context);
  runScripts(context, ["app-photos-frame.js"]);
  document.dispatchEvent({ type: "DOMContentLoaded" });
  return { context, document, image, movie, title, count, requests,
    downloads: () => downloads, aborted: () => aborted, plays: () => plays, pauses: () => pauses,
    release: () => release(),
    visibility(value) { document.hidden = value; document.dispatchEvent({ type: "visibilitychange" }); },
    event(name) { windowEvents.get(name)(); },
    async tick() { context.__runIntervals(); await new Promise(setImmediate); },
  };
}

async function testPhotoFrameBoundsMemoryAndPreservesSequenceAcrossCycles() {
  const f = frameFixture();
  await new Promise(setImmediate);
  const seen = [f.title.textContent];
  for (let i = 1; i < 1220; i++) { await f.tick(); seen.push(f.title.textContent); }
  assert.deepEqual(seen, Array.from({ length: 1220 }, (_, i) => String(i % 605)));
  assert.ok(f.context.framePeak <= 400, `retained ${f.context.framePeak} items`);
  assert.deepEqual(f.requests, [1, 2, 3, 4, 1, 2, 3, 4, 1]);
  assert.equal(f.count.textContent, "10 von 605 Medien");
}

async function testPhotoFramePausesHiddenPagesAndResumesWithoutAdvancing() {
  const f = frameFixture({ hidden: true, video: true });
  await f.tick();
  assert.equal(f.requests.length, 0);
  f.visibility(false);
  await new Promise(setImmediate);
  assert.equal(f.title.textContent, "0");
  const plays = f.plays();
  f.visibility(true);
  for (let i = 0; i < 20; i++) await f.tick();
  assert.equal(f.requests.length, 1);
  assert.equal(f.plays(), plays);
  assert.ok(f.pauses() > 0);
  f.visibility(false);
  assert.equal(f.title.textContent, "0");
  await f.tick();
  assert.equal(f.title.textContent, "1");
  f.event("pagehide");
  await f.tick();
  assert.equal(f.title.textContent, "1");
  f.event("pageshow");
  await f.tick();
  assert.equal(f.title.textContent, "2");
}

async function testPhotoFrameWaitsForSlowPagesAndCancelsHiddenFetches() {
  const f = frameFixture({ delayPage: 2 });
  await new Promise(setImmediate);
  for (let i = 0; i < 205; i++) await f.tick();
  assert.equal(f.title.textContent, "199");
  assert.deepEqual(f.requests, [1, 2]);
  f.visibility(true);
  await new Promise(setImmediate);
  assert.equal(f.aborted(), 1);
  const downloads = f.downloads();
  for (let i = 0; i < 10; i++) await f.tick();
  assert.equal(f.downloads(), downloads);
  f.visibility(false);
  await f.tick();
  assert.deepEqual(f.requests, [1, 2, 2]);
  f.release();
  await new Promise(setImmediate);
  assert.equal(f.title.textContent, "200");
  await f.tick();
  assert.equal(f.title.textContent, "201");
}

async function testPhotoFrameEmptyGalleryStopsAndInitializationIsIdempotent() {
  const f = frameFixture({ total: 0 });
  await new Promise(setImmediate);
  f.document.dispatchEvent({ type: "DOMContentLoaded" });
  for (let i = 0; i < 20; i++) await f.tick();
  assert.deepEqual(f.requests, [1]);
  assert.equal(f.downloads(), 0);
  assert.equal(f.count.textContent, "Keine Medien");
}

function testFamilyTreeLayoutPreservesPeopleAndGenerations() {
  const people = Array.from({length: 8}, (_, i) => ({id: i + 1, name: `Person ${i + 1}`}));
  const relations = [
    {from: 1, to: 3, kind: "mother"}, {from: 2, to: 3, kind: "father"},
    {from: 1, to: 2, kind: "marriage"}, {from: 3, to: 4, kind: "sibling"},
    {from: 4, to: 5, kind: "mother"}, {from: 6, to: 7, kind: "marriage"},
    {from: 7, to: 8, kind: "marriage"}, {from: 6, to: 8, kind: "sibling"}
  ];
  const tree = {people, relations}, result = familyTreeLayout.layout(tree);
  assert.equal(result.nodes.size, people.length);
  assert.equal(result.nodes.get(1).y, result.nodes.get(2).y);
  assert.equal(result.nodes.get(3).y, result.nodes.get(4).y);
  for (const edge of relations.filter(edge => edge.kind === "mother" || edge.kind === "father")) assert.ok(result.nodes.get(edge.from).y < result.nodes.get(edge.to).y);
  const nodes = Array.from(result.nodes.values());
  for (let i = 0; i < nodes.length; i++) for (let j = i + 1; j < nodes.length; j++) assert.ok(nodes[i].y !== nodes[j].y || Math.abs(nodes[i].x - nodes[j].x) >= familyTreeLayout.cardWidth);
  assert.deepEqual(result, familyTreeLayout.layout(tree));
  const crossGeneration = familyTreeLayout.layout({people: people.slice(0, 3), relations: [{from: 1, to: 2, kind: "mother"}, {from: 2, to: 3, kind: "father"}, {from: 1, to: 3, kind: "marriage"}]});
  assert.equal(crossGeneration.nodes.size, 3);
  assert.ok(crossGeneration.nodes.get(1).y < crossGeneration.nodes.get(3).y);
}
function testFamilyTreeLayoutAlignsUnequalBranchesAndOrdersSiblings() {
  const people = [1, 2, 3, 4, 5, 6, 7, 100, 101, 102, 103, 104, 105, 106, 107, 108, 109].map(id => ({id, name: `Person ${id}`, birth_date: id >= 100 ? `${2009 - (id - 100)}-01-01` : ""}));
  const relations = [
    {from: 1, to: 2, kind: "sibling"}, {from: 1, to: 3, kind: "mother"},
    {from: 3, to: 4, kind: "marriage"}, {from: 2, to: 5, kind: "father"},
    {from: 5, to: 6, kind: "marriage"}, {from: 5, to: 7, kind: "father"}, {from: 6, to: 7, kind: "mother"},
    ...people.filter(person => person.id >= 100).flatMap(person => [{from: 3, to: person.id, kind: "mother"}, {from: 4, to: person.id, kind: "father"}])
  ];
  const result = familyTreeLayout.layout({people, relations}), center = id => result.nodes.get(id).x + familyTreeLayout.cardWidth / 2;
  // Unequal branch widths must not pull a narrow family towards the page center.
  assert.ok(Math.abs(center(1) - center(3)) < 5);
  assert.ok(Math.abs(center(2) - center(5)) < 5);
  assert.ok(Math.abs(center(7) - (center(5) + center(6)) / 2) < 5);
  const children = people.filter(person => person.id >= 100).map(person => result.nodes.get(person.id)).sort((a,b) => a.x - b.x);
  assert.deepEqual(children.map(person => person.id), [109,108,107,106,105,104,103,102,101,100]);
  const childrenCenter = children.reduce((sum, person) => sum + center(person.id), 0) / children.length;
  assert.ok(Math.abs(childrenCenter - (center(3) + center(4)) / 2) < 5);
  for (const edge of relations.filter(edge => edge.kind === "mother" || edge.kind === "father")) {
    assert.ok(result.nodes.get(edge.to).y - result.nodes.get(edge.from).y - familyTreeLayout.cardHeight >= 250);
  }
  const byRow = new Map();
  result.nodes.forEach(person => { if (!byRow.has(person.y)) byRow.set(person.y, []); byRow.get(person.y).push(person); });
  byRow.forEach(row => { row.sort((a,b) => a.x-b.x); row.slice(1).forEach((person, i) => assert.ok(person.x >= row[i].x + row[i].width + 30)); });
  // SQL/input ordering must not rearrange the family on each visit.
  assert.deepEqual(result, familyTreeLayout.layout({people: people.slice().reverse(), relations: relations.slice().reverse()}));
}
function testFamilyTreeLayoutKeepsCoParentsAndRemarriagesTogether() {
  const people = [8, 3, 99, 5, 1, 44, 66].map(id => ({id, name: `Person ${id}`}));
  const relations = [
    {from: 8, to: 3, kind: "marriage"}, {from: 3, to: 99, kind: "marriage"},
    {from: 8, to: 5, kind: "mother"}, {from: 3, to: 5, kind: "father"},
    {from: 3, to: 1, kind: "father"}, {from: 99, to: 1, kind: "mother"},
    {from: 44, to: 66, kind: "father"}, {from: 5, to: 66, kind: "mother"}
  ];
  const result = familyTreeLayout.layout({people, relations}), nodes = result.nodes;
  for (const [a,b] of [[8,3],[3,99],[44,5]]) {
    assert.equal(nodes.get(a).y, nodes.get(b).y);
    assert.equal(Math.abs(nodes.get(a).x-nodes.get(b).x), familyTreeLayout.cardWidth+38);
  }
  assert.ok(nodes.get(66).y > nodes.get(44).y);
  assert.equal(result.nodes.size, people.length);
}
function testFamilyTreeLayoutHandlesLongFamiliesWithoutRecursion() {
  const people = Array.from({length: 10000}, (_, i) => ({id: i + 1, name: `Person ${i + 1}`}));
  const relations = people.slice(1).map(person => ({from: person.id - 1, to: person.id, kind: "mother"}));
  const result = familyTreeLayout.layout({people, relations});
  assert.equal(result.nodes.size, 10000);
  assert.ok(Number.isFinite(result.width) && Number.isFinite(result.height));
  assert.ok(result.nodes.get(9999).y < result.nodes.get(10000).y);
}

function thumbnailPollingFixture(respond, count = 1) {
  const document = new TestDocument();
  const images = [], wraps = [];
  for (let i = 0; i < count; i++) {
    const image = el("img", {
      "data-photo-thumb-image": "", "data-photo-thumb-ready": "0",
      "data-photo-thumb-src": `/photos/thumbnail?path=${i}.jpg&size=420`,
    });
    image.getBoundingClientRect = () => ({ top: 0, bottom: 20 });
    Object.defineProperty(image, "src", {
      get() { return this.getAttribute("src") || ""; },
      set(value) { this.setAttribute("src", value); queueMicrotask(() => this.onload?.()); },
    });
    const wrap = el("div", { class: "photo-thumb-wrap" }, [image, el("span", { class: "photo-thumb-loader" })]);
    images.push(image); wraps.push(wrap); document.body.append(wrap);
  }
  const context = createContext(document);
  const timers = new Map(), requests = [];
  let next = 1, now = 0;
  context.innerHeight = 800;
  context.setTimeout = (callback, delay = 0) => { const id = next++; timers.set(id, { callback, at: now + delay }); return id; };
  context.clearTimeout = (id) => timers.delete(id);
  context.requestAnimationFrame = (callback) => context.setTimeout(callback, 0);
  context.fetch = async (url, options) => {
    const items = JSON.parse(options.body).items;
    requests.push(items);
    return respond(requests.length, items, options.signal);
  };
  runScripts(context, ["app-photos-thumbnails.js"]);
  context.BearStack.photos.thumbnails.init();
  return { images, wraps, requests, timers, async drain(limit = 300) {
    for (let i = 0; timers.size && i < limit; i++) {
      const [id, task] = [...timers].sort((a, b) => a[1].at - b[1].at)[0];
      timers.delete(id); now = task.at; task.callback();
      await new Promise(setImmediate);
    }
  } };
}

async function testPhotoThumbnailPollingBoundsTransientFailures() {
  for (const mode of ["offline", "500", "408", "429", "json", "empty", "timeout"]) {
    const fixture = thumbnailPollingFixture(async (_, items, signal) => {
      if (mode === "offline") throw new Error("offline");
      if (mode === "timeout") return new Promise((resolve, reject) => signal.addEventListener("abort", () => reject(new Error("timeout"))));
      if (mode === "json") return { ok: true, status: 200, json: async () => { throw new Error("malformed"); } };
      if (mode === "empty") return { ok: true, status: 200, json: async () => ({ items: [] }) };
      return { ok: false, status: Number(mode) };
    });
    await fixture.drain();
    assert.equal(fixture.requests.length, 40, mode);
    assert.equal(fixture.timers.size, 0, mode);
    assert.equal(fixture.wraps[0].classList.contains("is-error"), true, mode);
    assert.equal(fixture.images[0].dataset.photoThumbLoaded, "1", mode);
  }
}

async function testPhotoThumbnailPollingStopsPermanentFailures() {
  for (const status of [400, 401, 403, 404, 413]) {
    const fixture = thumbnailPollingFixture(async () => ({ ok: false, status }));
    await fixture.drain();
    assert.equal(fixture.requests.length, 1, String(status));
    assert.equal(fixture.timers.size, 0);
    assert.equal(fixture.wraps[0].classList.contains("is-error"), true);
  }
  const redirected = thumbnailPollingFixture(async () => ({ ok: true, status: 200, redirected: true }));
  await redirected.drain();
  assert.equal(redirected.requests.length, 1);
  assert.equal(redirected.timers.size, 0);
}

async function testPhotoThumbnailPollingRecoversAndBoundsBatches() {
  const fixture = thumbnailPollingFixture(async (attempt, items) => {
    if (attempt < 3) throw new Error("temporarily offline");
    return { ok: true, status: 200, json: async () => ({ items: items.map(item => ({ ...item, ready: true })) }) };
  }, 205);
  await fixture.drain(500);
  assert.ok(fixture.requests.every(items => items.length <= 200));
  assert.ok(fixture.requests.length >= 4);
  assert.equal(fixture.timers.size, 0);
  assert.ok(fixture.images.every(image => image.dataset.photoThumbLoaded === "1" && image.src));
  assert.ok(fixture.wraps.every(wrap => !wrap.classList.contains("is-error")));
}

const tests = [
  testPhotoThumbnailPollingBoundsTransientFailures,
  testPhotoThumbnailPollingStopsPermanentFailures,
  testPhotoThumbnailPollingRecoversAndBoundsBatches,
  testFamilyTreeLayoutPreservesPeopleAndGenerations,
  testFamilyTreeLayoutHandlesLongFamiliesWithoutRecursion,
  testFamilyTreeLayoutAlignsUnequalBranchesAndOrdersSiblings,
  testFamilyTreeLayoutKeepsCoParentsAndRemarriagesTogether,
  testPhotoFrameEmptyGalleryStopsAndInitializationIsIdempotent,
  testPhotoFrameBoundsMemoryAndPreservesSequenceAcrossCycles,
  testPhotoFramePausesHiddenPagesAndResumesWithoutAdvancing,
  testPhotoFrameWaitsForSlowPagesAndCancelsHiddenFetches,
  testPeopleMergePrefersNamedSelection,
  testPeopleRemembersPageAndHonorsExplicitFilters,
  testPeopleRefreshRetainsImagesWhenCountsChange,
  testTagPickerUsesBackendDisplayValues,
  testTagPickerFallsBackToConfiguredDisplayMode,
  testTagModuleReadsOptionsWhenItLoads,
  testDocumentMetadataUsesTagModuleProtection,
  testUploadLifecycleUsesXHRBoundary,
  testUploadRefreshUsesDocumentModuleAndFallback,
  testOCRPollingAndCompletionOwnTheirState,
  testOCRRetriesAndRemembersDismissal,
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
