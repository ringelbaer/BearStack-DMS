(function () {
  "use strict";
  const root = document.querySelector("[data-transfer-page]");
  const modal = document.querySelector("[data-transfer-modal]");
  if (!root && !modal) return;
  const api = "/api/transfers/v1/";
  const states = {
    preparing: "Vorschau wird vorbereitet",
    planning: "Dateien und Ziel werden geprüft",
    ready: "Vorschau bereit",
    queued: "Wartet",
    running: "Wird übertragen",
    waiting: "Wartet auf Wiederholung",
    paused: "Pausiert",
    cancelled: "Abgebrochen",
    complete: "Abgeschlossen",
    partial: "Mit Konflikten oder Fehlern abgeschlossen",
    failed: "Fehlgeschlagen",
    missing: "Fehlt im Ziel",
    existing: "Bereits vorhanden",
    conflict: "Konflikt",
    done: "Übertragen",
    uploading: "Wird übertragen",
  };
  const node = (tag, text, cls) => {
    const n = document.createElement(tag);
    if (text !== undefined) n.textContent = text;
    if (cls) n.className = cls;
    return n;
  };
  const button = (text, action) => {
    const b = node("button", text);
    b.type = "button";
    b.addEventListener("click", action);
    return b;
  };
  const bytes = (value) => {
    let n = Number(value || 0),
      unit = 0;
    const units = ["B", "KiB", "MiB", "GiB", "TiB"];
    while (n >= 1024 && unit < units.length - 1) {
      n /= 1024;
      unit++;
    }
    return (
      n.toLocaleString("de-DE", { maximumFractionDigits: unit ? 1 : 0 }) +
      " " +
      units[unit]
    );
  };
  async function request(path, method = "GET", body) {
    const response = await fetch(api + path, {
      method,
      credentials: "same-origin",
      headers: {
        Accept: "application/json",
        ...(body === undefined ? {} : { "Content-Type": "application/json" }),
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    let result;
    try {
      result = await response.json();
    } catch (_) {
      throw new Error("Ungültige Serverantwort");
    }
    if (!response.ok) throw new Error(result.error || "Anfrage fehlgeschlagen");
    return result;
  }
  const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
  const status = root && root.querySelector("[data-transfer-status]");
  async function guard(fn, target = status) {
    try {
      if (target) target.textContent = "";
      await fn();
    } catch (error) {
      if (target) target.textContent = error.message;
    }
  }
  function summary(job) {
    const box = node("div", undefined, "transfer-summary");
    box.append(
      node(
        "p",
        job.source_name + " → " + job.connection_name + " · " + job.provider,
      ),
      node("p", "Ziel: " + job.base.name + " / " + job.target),
    );
    if (job.target_exists)
      box.append(
        node(
          "p",
          "Der Zielordner ist bereits vorhanden; fehlende Dateien werden ergänzt.",
        ),
      );
    box.append(
      node(
        "p",
        `${job.total} ${job.total === 1 ? "Datei" : "Dateien"} · ${bytes(job.bytes)} insgesamt · ${job.missing} neu (${bytes(job.missing_bytes)}) · ${job.existing} vorhanden · ${job.conflicts} Konflikte · ${job.failed} Fehler`,
      ),
    );
    box.append(
      node(
        "p",
        (states[job.state] || job.state) + (job.error ? ": " + job.error : ""),
      ),
    );
    if (
      [
        "running",
        "queued",
        "waiting",
        "paused",
        "complete",
        "partial",
      ].includes(job.state)
    ) {
      const progress = node("progress");
      progress.max = Math.max(1, job.uploaded_bytes + job.missing_bytes);
      progress.value =
        job.state === "complete"
          ? progress.max
          : Math.min(progress.max, job.uploaded_bytes + job.in_flight_bytes);
      progress.setAttribute("aria-label", "Übertragene Daten");
      box.append(
        progress,
        node(
          "p",
          `${job.done} übertragen · ${bytes(job.uploaded_bytes + job.in_flight_bytes)}`,
        ),
      );
    }
    return box;
  }
  async function appendItems(container, jobID, after = 0) {
    const items = await request(`jobs/${jobID}/items?after=${after}`);
    const list = node("ul");
    for (const item of items)
      list.append(
        node(
          "li",
          `${item.display_path} · ${bytes(item.size)} · ${states[item.state] || item.state}${item.error ? ": " + item.error : ""}`,
        ),
      );
    container.append(list);
    if (items.length === 200) {
      const more = button("Weitere Dateien", () =>
        guard(async () => {
          more.remove();
          await appendItems(container, jobID, items[items.length - 1].id);
        }),
      );
      container.append(more);
    }
  }
  if (root && root.dataset.transferPage === "connections") {
    const container = root.querySelector("[data-transfer-connections]");
    let providers = [],
      connections = [],
      loginGeneration = 0;
    const field = (label, type, value) => {
      const wrap = node("label", label);
      const input = node("input");
      input.type = type;
      input.value = value || "";
      wrap.append(input);
      return [wrap, input];
    };
    function renderConnection(connection) {
      const form = node("form", undefined, "transfer-connection");
      const [nameLabel, name] = field("Name", "text", connection.name);
      name.required = true;
      name.maxLength = 120;
      const providerLabel = node("label", "Anbieter"),
        provider = node("select");
      for (const info of providers) {
        const option = node("option", info.name);
        option.value = info.id;
        provider.append(option);
      }
      provider.value = connection.provider || providers[0].id;
      provider.disabled = Boolean(connection.id);
      providerLabel.append(provider);
      const fields = node("div");
      let inputs = [];
      function configFields() {
        fields.replaceChildren();
        inputs = [];
        const info = providers.find((p) => p.id === provider.value);
        for (const spec of info.fields || []) {
          const [wrap, input] = field(
            spec.label,
            spec.type,
            (connection.config || {})[spec.key],
          );
          input.required = true;
          fields.append(wrap);
          inputs.push([spec.key, input]);
        }
      }
      provider.addEventListener("change", configFields);
      configFields();
      const enabledLabel = node("label", " Aktiviert"),
        enabled = node("input");
      enabled.type = "checkbox";
      enabled.checked = Boolean(connection.enabled);
      enabledLabel.prepend(enabled);
      const save = node("button", "Speichern");
      save.type = "submit";
      form.append(
        node("h2", connection.name || "Neue Verbindung"),
        nameLabel,
        providerLabel,
        fields,
        enabledLabel,
        node(
          "p",
          connection.connected
            ? "Verbunden: " + connection.account
            : "Nicht verbunden",
        ),
        node(
          "p",
          "Basisziel: " +
            ((connection.base && connection.base.name) ||
              "Noch nicht ausgewählt"),
        ),
        save,
      );
      form.addEventListener("submit", (event) => {
        event.preventDefault();
        guard(async () => {
          save.disabled = true;
          try {
            const config = {};
            for (const [key, input] of inputs) config[key] = input.value.trim();
            await request(
              "connections" + (connection.id ? "/" + connection.id : ""),
              connection.id ? "PUT" : "POST",
              {
                name: name.value,
                provider: provider.value,
                enabled: enabled.checked,
                revision: connection.revision || 0,
                config,
              },
            );
            await load();
          } finally {
            save.disabled = false;
          }
        });
      });
      if (connection.id) {
        form.append(
          button("Konto verbinden", () => {
            const popup = window.open("about:blank", "_blank");
            if (popup) popup.opener = null;
            const generation = ++loginGeneration;
            guard(async () => {
              const login = await request(
                `connections/${connection.id}/login`,
                "POST",
              );
              if (popup) popup.location.replace(login.login_url);
              status.replaceChildren(
                node("span", "Anmeldung beim Speicheranbieter bestätigen. "),
              );
              const link = node("a", "Anmeldung öffnen");
              link.href = login.login_url;
              link.target = "_blank";
              link.rel = "noopener noreferrer";
              status.append(link);
              const deadline = Date.now() + 20 * 60 * 1000;
              while (generation === loginGeneration && Date.now() < deadline) {
                await sleep(2000);
                const result = await request(
                  `connections/${connection.id}/login/${login.token}`,
                  "POST",
                );
                if (result.connected) {
                  status.textContent =
                    "Konto verbunden. Jetzt Basis-Zielordner auswählen.";
                  await load();
                  return;
                }
              }
            });
          }),
        );
        const folders = button("Basis-Zielordner wählen", () =>
          guard(() => browse(connection)),
        );
        folders.disabled = !connection.connected || !connection.enabled;
        form.append(folders);
        const disconnect = button("Verbindung trennen", () =>
          guard(async () => {
            await request(`connections/${connection.id}/disconnect`, "POST");
            loginGeneration++;
            await load();
          }),
        );
        disconnect.disabled = !connection.connected;
        form.append(disconnect);
      }
      container.append(form);
    }
    async function load() {
      [providers, connections] = await Promise.all([
        request("providers"),
        request("connections"),
      ]);
      container.replaceChildren();
      for (const connection of connections) renderConnection(connection);
    }
    root.querySelector("[data-transfer-new]").addEventListener("click", () => {
      if (providers.length) renderConnection({ enabled: false });
    });
    const folders = document.querySelector("[data-transfer-folders]"),
      folderStatus = folders.querySelector("[data-transfer-folder-status]"),
      folderList = folders.querySelector("[data-transfer-folder-list]"),
      back = folders.querySelector("[data-transfer-folder-back]");
    folders
      .querySelector("[data-transfer-folder-close]")
      .addEventListener("click", () => folders.close());
    async function browse(connection) {
      const history = [];
      folders.showModal();
      async function show(parent, name) {
        folderStatus.textContent = "Ordner werden geladen …";
        const locations = await request(
          `connections/${connection.id}/locations?parent=${encodeURIComponent(parent)}`,
        );
        if (!folders.open) return;
        folderStatus.textContent = name || "Basis-Zielordner auswählen";
        folderList.replaceChildren();
        back.disabled = history.length === 0;
        if (parent)
          folderList.append(
            button("Diesen Ordner verwenden", () =>
              guard(async () => {
                await request(`connections/${connection.id}/base`, "POST", {
                  revision: connection.revision,
                  base: { id: parent, name },
                });
                folders.close();
                await load();
              }, folderStatus),
            ),
          );
        for (const location of locations) {
          if (location.id === parent) continue;
          folderList.append(
            button(location.name, () =>
              guard(async () => {
                history.push({ parent, name });
                await show(location.id, location.name);
              }, folderStatus),
            ),
          );
        }
      }
      back.onclick = () =>
        guard(async () => {
          const previous = history.pop();
          if (previous) await show(previous.parent, previous.name);
        }, folderStatus);
      await guard(() => show("", "Dateien"), folderStatus);
    }
    guard(load);
  }
  if (root && root.dataset.transferPage === "jobs") {
    const container = root.querySelector("[data-transfer-jobs]"),
      filter = root.querySelector("[data-transfer-filter]"),
      prev = root.querySelector("[data-transfer-prev]"),
      next = root.querySelector("[data-transfer-next]");
    let page = 1,
      loading = false;
    const cards = new Map();
    async function load() {
      if (loading) return;
      loading = true;
      try {
        const jobs = await request(
          `jobs?connection=${encodeURIComponent(filter.value)}&page=${page}`,
        );
        const wanted = new Set(jobs.map((job) => job.id));
        for (const [id, card] of cards)
          if (!wanted.has(id)) {
            card.element.remove();
            cards.delete(id);
          }
        const empty = container.querySelector("[data-transfer-empty]");
        if (empty) empty.remove();
        if (!jobs.length) {
          const hint = node("p", "Keine Aufträge vorhanden.");
          hint.dataset.transferEmpty = "";
          container.append(hint);
        }
        for (const [index, job] of jobs.entries()) {
          let card = cards.get(job.id);
          if (!card) {
            const element = node("section", undefined, "transfer-job"),
              overview = node("div"),
              actions = node("div");
            const details = node("details"),
              files = node("div");
            let loaded = false;
            const refresh = async () => {
              files.replaceChildren();
              await appendItems(files, job.id);
              loaded = true;
            };
            details.append(
              node("summary", "Dateien und Fehler"),
              button("Dateien aktualisieren", () => guard(refresh)),
              files,
            );
            details.addEventListener("toggle", () => {
              if (details.open && !loaded) guard(refresh);
            });
            element.append(overview, actions, details);
            card = { element, overview, actions, state: "" };
            cards.set(job.id, card);
          }
          card.overview.replaceChildren(summary(job));
          if (card.state !== job.state) {
            card.state = job.state;
            card.actions.replaceChildren();
            const actions = ["running", "queued", "waiting"].includes(job.state)
              ? [
                  ["pause", "Pause"],
                  ["cancel", "Abbrechen"],
                ]
              : job.state === "paused"
                ? [
                    ["resume", "Fortsetzen"],
                    ["cancel", "Abbrechen"],
                  ]
                : ["partial", "failed"].includes(job.state)
                  ? [
                      ["retry", "Wiederholen"],
                      ["cancel", "Abbrechen"],
                    ]
                  : [];
            for (const [action, label] of actions)
              card.actions.append(
                button(label, () =>
                  guard(async () => {
                    await request(`jobs/${job.id}/${action}`, "POST");
                    await load();
                  }),
                ),
              );
          }
          if (container.children[index] !== card.element)
            container.insertBefore(
              card.element,
              container.children[index] || null,
            );
        }
        prev.disabled = page === 1;
        next.disabled = jobs.length < 30;
        root.querySelector("[data-transfer-page-number]").textContent =
          "Seite " + page;
      } finally {
        loading = false;
      }
    }
    filter.addEventListener("change", () => {
      page = 1;
      guard(load);
    });
    prev.addEventListener("click", () => {
      page = Math.max(1, page - 1);
      guard(load);
    });
    next.addEventListener("click", () => {
      page++;
      guard(load);
    });
    guard(async () => {
      for (const c of await request("connections")) {
        const option = node("option", c.name);
        option.value = c.id;
        filter.append(option);
      }
      await load();
    });
    window.setInterval(() => {
      if (!document.hidden) guard(load);
    }, 2000);
  }
  if (modal) {
    const gallerySelection = JSON.parse(modal.dataset.transferSelection),
      connection = modal.querySelector("[data-transfer-connection]"),
      target = modal.querySelector("[data-transfer-target]"),
      preview = modal.querySelector("[data-transfer-preview]"),
      start = modal.querySelector("[data-transfer-start]"),
      message = modal.querySelector("[data-transfer-preview-status]"),
      output = modal.querySelector("[data-transfer-summary]");
    let generation = 0,
      job = null,
      selection = gallerySelection;
    const previewItems = modal.querySelector("[data-transfer-preview-items]");
    function invalidate() {
      generation++;
      job = null;
      start.disabled = true;
      output.replaceChildren();
      previewItems.replaceChildren();
      message.textContent = "Ziel bitte prüfen.";
      preview.disabled = false;
    }
    connection.addEventListener("change", invalidate);
    target.addEventListener("input", invalidate);
    modal
      .querySelector("[data-transfer-close]")
      .addEventListener("click", () => modal.close());
    modal.addEventListener("close", invalidate);
    for (const open of document.querySelectorAll("[data-transfer-open]"))
      open.addEventListener("click", () =>
        guard(async () => {
          invalidate();
          selection = { ...gallerySelection };
          if (open.dataset.transferOpen === "selection") {
            selection.paths = Array.from(
              document.querySelectorAll(
                '[data-photo-bulk-form] input[name="ids"]:checked',
              ),
              (input) => input.value,
            );
            if (!selection.paths.length)
              throw new Error("Bitte mindestens ein Medium auswählen.");
          }
          target.readOnly =
            !selection.paths &&
            !selection.query &&
            !selection.path.startsWith(".people/");
          target.value = "";
          connection.replaceChildren(node("option", "Verbindung auswählen"));
          connection.firstChild.value = "";
          modal.showModal();
          const opened = generation;
          const list = (await request("connections")).filter(
            (c) => c.enabled && c.connected && c.base.id,
          );
          if (!modal.open || opened !== generation) return;
          for (const c of list) {
            const option = node("option", c.name + " · " + c.base.name);
            option.value = c.id;
            connection.append(option);
          }
          if (list.length === 1) connection.value = list[0].id;
          if (!list.length) {
            preview.disabled = true;
            message.textContent =
              "Keine aktive, verbundene Speicherverbindung verfügbar.";
          }
        }, message),
      );
    preview.addEventListener("click", () =>
      guard(async () => {
        if (!connection.value)
          throw new Error("Bitte eine Verbindung auswählen.");
        invalidate();
        const current = generation;
        preview.disabled = true;
        try {
          let result = await request("previews", "POST", {
            connection_id: connection.value,
            selection,
            target: target.value,
          });
          while (current === generation && modal.open) {
            if (result.state === "ready") {
              job = result;
              target.value = result.target;
              output.replaceChildren(summary(result));
              message.textContent =
                "Bestehende Dateien werden erhalten. Gleiche Größe ist kein Nachweis gleicher Inhalte.";
              start.disabled = result.missing === 0;
              const details = node("details"),
                heading = node("summary", "Dateien und Konflikte ansehen");
              details.append(heading);
              let loaded = false;
              details.addEventListener("toggle", () => {
                if (details.open && !loaded) {
                  loaded = true;
                  guard(() => appendItems(details, result.id), message);
                }
              });
              previewItems.append(details);
              return;
            }
            if (!["preparing", "planning"].includes(result.state))
              throw new Error(
                result.error || "Vorschau konnte nicht erstellt werden.",
              );
            message.textContent = states[result.state];
            await sleep(1000);
            if (current !== generation) return;
            result = await request(`jobs/${result.id}`);
          }
        } finally {
          if (current === generation) preview.disabled = false;
        }
      }, message),
    );
    start.addEventListener("click", () =>
      guard(async () => {
        if (!job) return;
        start.disabled = true;
        try {
          await request(`jobs/${job.id}/start`, "POST");
          message.textContent =
            "Upload eingereiht. Den Fortschritt zeigt die Upload-Warteschlange.";
          output.replaceChildren(
            node(
              "p",
              "Der Upload läuft im Hintergrund. Dieses Fenster kann geschlossen werden.",
            ),
          );
        } catch (error) {
          start.disabled = false;
          throw error;
        }
      }, message),
    );
  }
})();
