function ocrStatusMessage(job) {
  if (!job) return "";
  if (job.error) return job.error;
  if (job.status === "completed") {
    return `${job.text_length || 0} Zeichen wurden in den Textinhalt übernommen.`;
  }
  const details = [];
  if (job.message) {
    details.push(job.message);
  }
  if (job.active && job.updated_at) {
    const updatedAt = Date.parse(job.updated_at);
    if (!Number.isNaN(updatedAt)) {
      const ageSeconds = Math.max(0, Math.floor((Date.now() - updatedAt) / 1000));
      if (ageSeconds >= 120) {
        const ageMinutes = Math.max(2, Math.floor(ageSeconds / 60));
        details.push(`Keine neue Fortschrittsmeldung seit ${ageMinutes} Min.`);
      }
    }
  }
  if (details.length) {
    return details.join(" ");
  }
  if (job.status === "queued") {
    return "Der OCR-Vorgang wartet auf Ausführung.";
  }
  if (job.status === "running") {
    return "Der OCR-Vorgang wird verarbeitet.";
  }
  if (job.status === "interrupted") {
    return "Der OCR-Vorgang wurde unterbrochen.";
  }
  return "OCR-Status wurde aktualisiert.";
}

function ocrDismissKey(jobID) {
  return jobID ? `bearstack.ocr.dismissed.${jobID}` : "";
}

function isOCRDismissed(jobID) {
  const key = ocrDismissKey(jobID);
  if (!key) return false;
  try {
    return window.sessionStorage.getItem(key) === "1";
  } catch {
    return false;
  }
}

function markOCRDismissed(jobID) {
  const key = ocrDismissKey(jobID);
  if (!key) return;
  try {
    window.sessionStorage.setItem(key, "1");
  } catch {
    // Hiding the panel should still work when session storage is unavailable.
  }
}

function renderOCRStatus(job) {
  if (!ocrStatus || !job) return;
  const state = ocrStatus.querySelector("[data-ocr-state]");
  const progress = ocrStatus.querySelector("[data-ocr-progress]");
  const progressRow = ocrStatus.querySelector("[data-ocr-progress-row]");
  const progressText = ocrStatus.querySelector("[data-ocr-progress-text]");
  const message = ocrStatus.querySelector("[data-ocr-message]");
  const dismiss = ocrStatus.querySelector("[data-ocr-dismiss]");

  const jobID = String(job.id || "");
  ocrStatus.dataset.ocrJobId = jobID;
  ocrStatus.dataset.ocrActive = job.active ? "1" : "0";
  ocrStatus.dataset.ocrTerminal = job.terminal ? "1" : "0";
  ocrStatus.hidden = job.terminal && isOCRDismissed(jobID);
  ["queued", "running", "completed", "failed", "interrupted"].forEach((status) => {
    ocrStatus.classList.toggle(`ocr-status-${status}`, job.status === status);
  });
  if (state) {
    state.textContent = job.status_text || "Unbekannt";
  }
  if (progress && progressRow) {
    progress.value = job.progress_percent || 0;
    progressRow.hidden = !job.total_pages;
  }
  if (progressText) {
    progressText.textContent = job.total_pages ? `${job.current_page || 0} von ${job.total_pages}` : "";
  }
  if (message) {
    message.textContent = ocrStatusMessage(job);
  }
  if (dismiss) {
    dismiss.hidden = !job.terminal;
  }
}

function pollOCRStatus() {
  if (!ocrStatus || !ocrStatus.dataset.ocrStatusUrl) return;
  window
    .fetch(ocrStatus.dataset.ocrStatusUrl, { credentials: "same-origin", headers: { Accept: "application/json" } })
    .then((response) => {
      if (!response.ok) {
        throw new Error("OCR-Status konnte nicht gelesen werden");
      }
      return response.json();
    })
    .then((payload) => {
      if (!payload.job) return;
      const wasActive = ocrStatus.dataset.ocrActive === "1";
      renderOCRStatus(payload.job);
      if (wasActive && payload.job.status === "completed") {
        window.setTimeout(() => window.location.reload(), 900);
      }
      if (payload.job.active) {
        window.setTimeout(pollOCRStatus, 2000);
      }
    })
    .catch(() => {
      window.setTimeout(pollOCRStatus, 5000);
    });
}

if (ocrStatus && ocrStatus.dataset.ocrActive === "1") {
  window.setTimeout(pollOCRStatus, 1000);
}

if (ocrStatus) {
  const dismiss = ocrStatus.querySelector("[data-ocr-dismiss]");
  const jobID = ocrStatus.dataset.ocrJobId || "";
  const terminal = ocrStatus.dataset.ocrTerminal === "1";
  if (terminal && isOCRDismissed(jobID)) {
    ocrStatus.hidden = true;
  }
  dismiss?.addEventListener("click", () => {
    markOCRDismissed(ocrStatus.dataset.ocrJobId || "");
    ocrStatus.hidden = true;
  });
}
