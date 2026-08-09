const pdfJSPrefix = "/static/vendor/pdfjs-6.2.108";
let pdfJSModulePromise;

function loadPDFJS() {
  if (!pdfJSModulePromise) {
    pdfJSModulePromise = import(`${pdfJSPrefix}/build/pdf.mjs`).then((pdfjs) => {
      pdfjs.GlobalWorkerOptions.workerSrc = `${pdfJSPrefix}/build/pdf.worker.mjs`;
      return pdfjs;
    });
  }
  return pdfJSModulePromise;
}

class BearStackPDFPreview {
  constructor(root) {
    this.root = root;
    this.scroll = root.querySelector("[data-pdf-scroll]");
    this.pageContainer = root.querySelector("[data-pdf-page-container]");
    this.canvas = root.querySelector("[data-pdf-canvas]");
    this.textLayer = root.querySelector("[data-pdf-text-layer]");
    this.annotationLayer = root.querySelector("[data-pdf-annotation-layer]");
    this.status = root.querySelector("[data-pdf-status]");
    this.pageInput = root.querySelector("[data-pdf-page]");
    this.pagesLabel = root.querySelector("[data-pdf-pages]");
    this.zoomLabel = root.querySelector("[data-pdf-zoom]");
    this.pageNumber = 1;
    this.scale = 1;
    this.scaleMode = "page-width";
    this.generation = 0;
    this.bindControls();
  }

  bindControls() {
    this.root.querySelector("[data-pdf-previous]")?.addEventListener("click", () => this.goToPage(this.pageNumber - 1));
    this.root.querySelector("[data-pdf-next]")?.addEventListener("click", () => this.goToPage(this.pageNumber + 1));
    this.root.querySelector("[data-pdf-zoom-out]")?.addEventListener("click", () => this.changeZoom(1 / 1.2));
    this.root.querySelector("[data-pdf-zoom-in]")?.addEventListener("click", () => this.changeZoom(1.2));
    this.root.querySelector("[data-pdf-fit-width]")?.addEventListener("click", () => this.fit("page-width"));
    this.root.querySelector("[data-pdf-fit-page]")?.addEventListener("click", () => this.fit("page-fit"));
    this.pageInput?.addEventListener("change", () => this.goToPage(Number(this.pageInput.value)));
    this.scroll?.addEventListener("keydown", (event) => {
      if (event.key === "PageUp" || event.key === "ArrowLeft") {
        event.preventDefault();
        this.goToPage(this.pageNumber - 1);
      } else if (event.key === "PageDown" || event.key === "ArrowRight") {
        event.preventDefault();
        this.goToPage(this.pageNumber + 1);
      } else if (event.key === "+" || event.key === "=") {
        event.preventDefault();
        this.changeZoom(1.2);
      } else if (event.key === "-") {
        event.preventDefault();
        this.changeZoom(1 / 1.2);
      }
    });
    if (typeof ResizeObserver === "function") {
      this.resizeObserver = new ResizeObserver(() => {
        if (this.pdf && this.scaleMode !== "manual") this.scheduleRender();
      });
      this.resizeObserver.observe(this.scroll);
    }
  }

  async load(url, title, downloadURL) {
    await this.destroy();
    const generation = ++this.generation;
    this.root.hidden = false;
    this.setStatus("PDF wird geladen …", false);
    this.root.querySelector("[data-pdf-native]").href = url;
    this.root.querySelector("[data-pdf-download]").href = downloadURL;
    this.canvas.setAttribute("aria-label", `Seite 1 von ${title || "PDF"}`);
    const pdfjs = await loadPDFJS();
    if (generation !== this.generation) return;
    this.pdfjs = pdfjs;
    this.loadingTask = pdfjs.getDocument({
      url,
      cMapUrl: `${pdfJSPrefix}/cmaps/`,
      cMapPacked: true,
      standardFontDataUrl: `${pdfJSPrefix}/standard_fonts/`,
      wasmUrl: `${pdfJSPrefix}/wasm/`,
      iccUrl: `${pdfJSPrefix}/iccs/`,
      enableXfa: false,
      isEvalSupported: false,
      useWasm: true,
    });
    this.pdf = await this.loadingTask.promise;
    if (generation !== this.generation) return;
    this.pageNumber = 1;
    this.pagesLabel.textContent = String(this.pdf.numPages);
    this.pageInput.max = String(this.pdf.numPages);
    this.setStatus("", false);
    await this.renderPage(generation);
  }

  async destroy() {
    this.generation += 1;
    this.renderTask?.cancel();
    this.textTask?.cancel();
    this.renderTask = null;
    this.textTask = null;
    if (this.loadingTask) {
      try { await this.loadingTask.destroy(); } catch { /* already closed */ }
    }
    this.loadingTask = null;
    this.pdf = null;
    this.textLayer.replaceChildren();
    this.annotationLayer.replaceChildren();
    this.canvas.width = 0;
    this.canvas.height = 0;
    this.root.hidden = true;
  }

  scheduleRender() {
    clearTimeout(this.resizeTimer);
    this.resizeTimer = setTimeout(() => this.renderPage(this.generation).catch(() => {}), 80);
  }

  goToPage(number) {
    if (!this.pdf || !Number.isFinite(number)) return;
    this.pageNumber = Math.min(this.pdf.numPages, Math.max(1, Math.round(number)));
    this.renderPage(this.generation).catch(() => {});
  }

  changeZoom(factor) {
    if (!this.pdf) return;
    this.scaleMode = "manual";
    this.scale = Math.min(4, Math.max(0.25, this.scale * factor));
    this.renderPage(this.generation).catch(() => {});
  }

  fit(mode) {
    if (!this.pdf) return;
    this.scaleMode = mode;
    this.renderPage(this.generation).catch(() => {});
  }

  fittedScale(viewport) {
    const width = Math.max(120, this.scroll.clientWidth - 32);
    const height = Math.max(120, this.scroll.clientHeight - 32);
    if (this.scaleMode === "page-fit") return Math.min(width / viewport.width, height / viewport.height);
    if (this.scaleMode === "page-width") return width / viewport.width;
    return this.scale;
  }

  async renderPage(generation) {
    if (!this.pdf || generation !== this.generation) return;
    this.renderTask?.cancel();
    this.textTask?.cancel();
    this.setStatus(`Seite ${this.pageNumber} wird geladen …`, false);
    try {
      const page = await this.pdf.getPage(this.pageNumber);
      if (generation !== this.generation) return;
      const baseViewport = page.getViewport({ scale: 1 });
      this.scale = Math.min(4, Math.max(0.25, this.fittedScale(baseViewport)));
      const viewport = page.getViewport({ scale: this.scale });
      const deviceScale = Math.min(2, window.devicePixelRatio || 1);
      const maxPixels = 16777216;
      const requestedPixels = viewport.width * viewport.height * deviceScale * deviceScale;
      const outputScale = requestedPixels > maxPixels ? Math.sqrt(maxPixels / (viewport.width * viewport.height)) : deviceScale;
      this.canvas.width = Math.max(1, Math.floor(viewport.width * outputScale));
      this.canvas.height = Math.max(1, Math.floor(viewport.height * outputScale));
      this.canvas.style.width = `${viewport.width}px`;
      this.canvas.style.height = `${viewport.height}px`;
      this.pageContainer.style.width = `${viewport.width}px`;
      this.pageContainer.style.height = `${viewport.height}px`;
      this.pageInput.value = String(this.pageNumber);
      this.zoomLabel.textContent = `${Math.round(this.scale * 100)} %`;
      this.canvas.setAttribute("aria-label", `PDF-Seite ${this.pageNumber} von ${this.pdf.numPages}`);
      const context = this.canvas.getContext("2d", { alpha: false });
      this.renderTask = page.render({ canvasContext: context, viewport, transform: outputScale === 1 ? null : [outputScale, 0, 0, outputScale, 0, 0] });
      await this.renderTask.promise;
      if (generation !== this.generation) return;
      this.textLayer.replaceChildren();
      this.textTask = new this.pdfjs.TextLayer({ textContentSource: page.streamTextContent(), container: this.textLayer, viewport });
      await this.textTask.render();
      await this.renderAnnotations(page, viewport);
      this.setStatus(`Seite ${this.pageNumber} von ${this.pdf.numPages}`, true);
    } catch (error) {
      if (error?.name === "RenderingCancelledException" || generation !== this.generation) return;
      throw error;
    }
  }

  async renderAnnotations(page, viewport) {
    this.annotationLayer.replaceChildren();
    const annotations = await page.getAnnotations({ intent: "display" });
    if (!annotations.length) return;
    const linkService = {
      externalLinkEnabled: true,
      addLinkAttributes(link, url) {
        link.href = url;
        link.target = "_blank";
        link.rel = "noopener noreferrer nofollow";
      },
      getDestinationHash: () => "#",
      getAnchorUrl: () => "#",
      goToDestination: () => {},
      executeNamedAction: (action) => {
        if (action === "NextPage") this.goToPage(this.pageNumber + 1);
        if (action === "PrevPage") this.goToPage(this.pageNumber - 1);
      },
    };
    const layer = new this.pdfjs.AnnotationLayer({ div: this.annotationLayer, page, viewport, linkService });
    await layer.render({ annotations, renderForms: false, linkService });
  }

  setStatus(message, visuallyHidden) {
    this.status.textContent = message;
    this.status.classList.toggle("visually-hidden", visuallyHidden);
  }
}

const controllers = new WeakMap();

window.BearStackPDFPreviewController = BearStackPDFPreview;
window.bearStackPDFPreview = {
  load(root, url, title, downloadURL) {
    let controller = controllers.get(root);
    if (!controller) {
      controller = new BearStackPDFPreview(root);
      controllers.set(root, controller);
    }
    return controller.load(url, title, downloadURL);
  },
  destroy(root) {
    return controllers.get(root)?.destroy();
  },
};
