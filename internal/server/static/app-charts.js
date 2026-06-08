const statNumberFormat = new Intl.NumberFormat("de-DE");

function svgElement(name, attributes = {}) {
  const element = document.createElementNS("http://www.w3.org/2000/svg", name);
  Object.entries(attributes).forEach(([key, value]) => {
    element.setAttribute(key, String(value));
  });
  return element;
}

function drawLineChart(svg, years, series) {
  const width = 720;
  const height = 260;
  const padding = { top: 18, right: 24, bottom: 38, left: 52 };
  const plotWidth = width - padding.left - padding.right;
  const plotHeight = height - padding.top - padding.bottom;
  const maximum = Math.max(
    1,
    ...series.flatMap((item) => years.map((year) => item.data.get(year) || 0)),
  );
  const xFor = (index) => padding.left + (years.length <= 1 ? plotWidth / 2 : (index * plotWidth) / (years.length - 1));
  const yFor = (count) => padding.top + plotHeight - (count * plotHeight) / maximum;

  svg.textContent = "";
  svg.append(
    svgElement("line", { class: "tag-timeline-axis", x1: padding.left, y1: padding.top, x2: padding.left, y2: padding.top + plotHeight }),
    svgElement("line", { class: "tag-timeline-axis", x1: padding.left, y1: padding.top + plotHeight, x2: padding.left + plotWidth, y2: padding.top + plotHeight }),
  );

  Array.from(new Set([0, Math.ceil(maximum / 2), maximum])).forEach((value) => {
    const y = yFor(value);
    svg.append(svgElement("line", { class: "tag-timeline-grid", x1: padding.left, y1: y, x2: padding.left + plotWidth, y2: y }));
    const label = svgElement("text", { class: "tag-timeline-y-label", x: padding.left - 10, y: y + 4, "text-anchor": "end" });
    label.textContent = statNumberFormat.format(value);
    svg.append(label);
  });

  years.forEach((year, index) => {
    const x = xFor(index);
    const tick = svgElement("text", { class: "tag-timeline-x-label", x, y: height - 10, "text-anchor": "middle" });
    tick.textContent = year;
    svg.append(tick);
  });

  series.forEach((item) => {
    const points = years.map((year, index) => {
      const count = item.data.get(year) || 0;
      return { x: xFor(index), y: yFor(count), count };
    });
    svg.append(svgElement("polyline", {
      class: "tag-timeline-line",
      points: points.map((point) => `${point.x},${point.y}`).join(" "),
      stroke: item.color,
    }));
    points.forEach((point, index) => {
      const circle = svgElement("circle", {
        class: "tag-timeline-point",
        cx: point.x,
        cy: point.y,
        r: 4,
        fill: item.color,
      });
      const title = svgElement("title");
      title.textContent = `${item.label}, ${years[index]}: ${statNumberFormat.format(point.count)}`;
      circle.append(title);
      svg.append(circle);
    });
  });
}

function updateDocumentDateChart(chart) {
  const svg = chart.querySelector("[data-document-date-svg]");
  if (!svg) return;
  const years = [];
  const data = new Map();
  chart.querySelectorAll("[data-document-date-point]").forEach((point) => {
    const year = point.dataset.year || "";
    const count = Number(point.dataset.count || 0);
    if (!year) return;
    if (!years.includes(year)) years.push(year);
    data.set(year, count);
  });
  years.sort();
  drawLineChart(svg, years, [{ label: "Aktive Dokumente", color: "#5d7f48", data }]);
}

function initializeDocumentDateCharts(root = document) {
  initializeOnce(root, "[data-document-date-chart]", updateDocumentDateChart);
}

function updateTagTimeline(chart) {
  const svg = chart.querySelector("[data-tag-timeline-svg]");
  if (!svg) return;

  const toggles = Array.from(chart.querySelectorAll("[data-tag-timeline-toggle]"));

  const years = [];
  const data = new Map();
  chart.querySelectorAll("[data-tag-timeline-point]").forEach((point) => {
    const year = point.dataset.year || "";
    const tag = point.dataset.tag || "";
    const count = Number(point.dataset.count || 0);
    if (!year || !tag) return;
    if (!years.includes(year)) years.push(year);
    if (!data.has(tag)) data.set(tag, new Map());
    data.get(tag).set(year, count);
  });
  years.sort();

  const series = toggles.flatMap((input) => {
    const tag = input.value;
    if (!input.checked) return [];
    const tagPill = input.nextElementSibling;
    const color = getComputedStyle(tagPill).getPropertyValue("--tag-color").trim() || "#176b87";
    return [{ label: tagPill?.textContent?.trim() || tag, color, data: data.get(tag) || new Map() }];
  });
  drawLineChart(svg, years, series);
}

function initializeTagTimelines(root = document) {
  initializeOnce(root, "[data-tag-timeline]", (chart) => {
    chart.querySelectorAll("[data-tag-timeline-toggle]").forEach((input) => {
      input.addEventListener("change", () => updateTagTimeline(chart));
    });
    updateTagTimeline(chart);
  });
}

document.addEventListener("DOMContentLoaded", () => {
  initializeDocumentDateCharts();
  initializeTagTimelines();
});
