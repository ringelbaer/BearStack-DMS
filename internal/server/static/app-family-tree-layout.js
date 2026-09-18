// Pure, deterministic layout shared by the browser and regression tests.
(function () {
  "use strict";
  const cardWidth = 250, cardHeight = 144, gap = 38, familyGap = 76, rowHeight = 400, marginX = 64, marginY = 128;
  function layout(tree) {
    const people = new Map(tree.people.map(person => [person.id, person]));
    const collator = new Intl.Collator("de", { numeric: true, sensitivity: "base" });
    function comparePeople(a, b) {
      const first = people.get(a), second = people.get(b);
      return (first.birth_date || "9999").localeCompare(second.birth_date || "9999") || collator.compare(first.name, second.name) || a - b;
    }
    const ids = Array.from(people.keys()).sort(comparePeople);
    const relations = tree.relations.filter(edge => people.has(edge.from) && people.has(edge.to)).slice().sort((a, b) => a.from - b.from || a.to - b.to || a.kind.localeCompare(b.kind) || (a.id || 0) - (b.id || 0));
    const parents = relations.filter(edge => edge.kind === "mother" || edge.kind === "father");
    const parentIDs = new Map(), parentNeighbors = new Map(ids.map(id => [id, new Set()])), childNeighbors = new Map(ids.map(id => [id, new Set()]));
    parents.forEach(edge => {
      if (!parentIDs.has(edge.to)) parentIDs.set(edge.to, []);
      parentIDs.get(edge.to).push(edge.from);
      parentNeighbors.get(edge.to).add(edge.from); childNeighbors.get(edge.from).add(edge.to);
    });
    const partners = relations.filter(edge => edge.kind === "marriage").map(edge => [edge.from, edge.to]);
    // Recorded co-parents occupy the same generation even without a marriage record.
    // This is only a placement constraint, never an invented relationship line.
    parentIDs.forEach(list => { list.sort(comparePeople); list.slice(1).forEach(id => partners.push([list[0], id])); });
    function disjoint() {
      const roots = new Map(ids.map(id => [id, id]));
      function find(id) { while (roots.get(id) !== id) { roots.set(id, roots.get(roots.get(id))); id = roots.get(id); } return id; }
      return { find, join(a, b) { a = find(a); b = find(b); roots.set(Math.max(a, b), Math.min(a, b)); } };
    }
    function generations(couple) {
      const sets = disjoint();
      if (couple) {
        partners.forEach(([a, b]) => sets.join(a, b));
        relations.filter(edge => edge.kind === "sibling").forEach(edge => sets.join(edge.from, edge.to));
      }
      const groups = new Map();
      ids.forEach(id => { const root = sets.find(id); if (!groups.has(root)) groups.set(root, { next: new Set(), incoming: 0, level: 0 }); });
      let conflict = false;
      parents.forEach(edge => {
        const a = sets.find(edge.from), b = sets.find(edge.to);
        if (a === b) { conflict = true; return; }
        if (!groups.get(a).next.has(b)) { groups.get(a).next.add(b); groups.get(b).incoming++; }
      });
      const ready = Array.from(groups.keys()).filter(id => !groups.get(id).incoming).sort((a, b) => a - b); let head = 0;
      while (head < ready.length) {
        const group = groups.get(ready[head++]);
        group.next.forEach(id => { const next = groups.get(id); next.level = Math.max(next.level, group.level + 1); if (--next.incoming === 0) ready.push(id); });
      }
      return { conflict: conflict || head !== groups.size, levels: new Map(ids.map(id => [id, groups.get(sets.find(id)).level])) };
    }
    let generation = generations(true);
    // Cross-generation marriages must remain visible without flattening ancestry.
    if (generation.conflict) generation = generations(false);

    // Generation cohorts may span a whole extended family. For horizontal placement
    // use smaller partner/co-parent units; siblings follow their own parent anchors.
    const sets = disjoint(), partnerNeighbors = new Map(ids.map(id => [id, new Set()]));
    partners.forEach(([a, b]) => {
      if (generation.levels.get(a) !== generation.levels.get(b)) return;
      sets.join(a, b); partnerNeighbors.get(a).add(b); partnerNeighbors.get(b).add(a);
    });
    const units = new Map(), unitFor = new Map(), offsets = new Map(), rows = new Map();
    ids.forEach(id => {
      const key = sets.find(id);
      if (!units.has(key)) units.set(key, { id: key, members: [], level: generation.levels.get(id), center: 0, width: 0, incoming: [], outgoing: [], siblings: new Set(), rank: 0 });
      const unit = units.get(key); unit.members.push(id); unitFor.set(id, unit);
    });
    function personCenter(id) { return unitFor.get(id).center + offsets.get(id); }
    function orderMembers(unit, direction) {
      const neighbors = direction === "parents" ? parentNeighbors : childNeighbors;
      const scores = new Map();
      unit.members.forEach(id => {
        const anchors = Array.from(neighbors.get(id)).filter(other => unitFor.get(other) !== unit);
        if (direction && anchors.length) scores.set(id, anchors.reduce((sum, other) => sum + personCenter(other), 0) / anchors.length);
      });
      function compare(a, b) { return (direction ? (scores.get(a) ?? personCenter(a)) - (scores.get(b) ?? personCenter(b)) : 0) || comparePeople(a, b); }
      // Walk the partner graph from an endpoint, keeping repeated marriages close
      // instead of splitting couples by database ID. No recursive family traversal.
      const starts = unit.members.slice().sort((a, b) => partnerNeighbors.get(a).size - partnerNeighbors.get(b).size || compare(a, b));
      const ordered = [], visited = new Set();
      starts.forEach(start => {
        if (visited.has(start)) return;
        const stack = [start];
        while (stack.length) {
          const id = stack.pop(); if (visited.has(id)) continue;
          visited.add(id); ordered.push(id);
          const next = Array.from(partnerNeighbors.get(id)).filter(other => !visited.has(other)).sort(compare);
          for (let i = next.length - 1; i >= 0; i--) stack.push(next[i]);
        }
      });
      unit.members = ordered; unit.width = ordered.length * (cardWidth + gap) - gap;
      ordered.forEach((id, index) => offsets.set(id, index * (cardWidth + gap) + cardWidth / 2 - unit.width / 2));
    }
    units.forEach(unit => {
      orderMembers(unit, null);
      if (!rows.has(unit.level)) rows.set(unit.level, []);
      rows.get(unit.level).push(unit);
    });
    parents.forEach(edge => {
      const from = unitFor.get(edge.from), to = unitFor.get(edge.to); if (from === to) return;
      from.outgoing.push({ own: edge.from, other: edge.to }); to.incoming.push({ own: edge.to, other: edge.from });
    });
    relations.filter(edge => edge.kind === "sibling").forEach(edge => {
      const a = unitFor.get(edge.from), b = unitFor.get(edge.to);
      if (a !== b && a.level === b.level) { a.siblings.add(b); b.siblings.add(a); }
    });
    const levels = Array.from(rows.keys()).sort((a, b) => a - b);
    function compareUnits(a, b) { return comparePeople(a.members[0], b.members[0]); }
    levels.forEach(level => {
      const row = rows.get(level), ordered = [], visited = new Set();
      // Keep explicit sibling families next to each other even with no known parents.
      row.sort(compareUnits).forEach(start => {
        if (visited.has(start)) return;
        const stack = [start];
        while (stack.length) {
          const unit = stack.pop(); if (visited.has(unit)) continue;
          visited.add(unit); ordered.push(unit);
          const next = Array.from(unit.siblings).filter(other => !visited.has(other)).sort(compareUnits);
          for (let i = next.length - 1; i >= 0; i--) stack.push(next[i]);
        }
      });
      let left = 0;
      ordered.forEach((unit, index) => { unit.rank = index; unit.center = left + unit.width / 2; left += unit.width + familyGap; });
      rows.set(level, ordered);
    });
    function target(unit, direction) {
      const links = direction === "parents" ? unit.incoming : direction === "children" ? unit.outgoing : unit.incoming.concat(unit.outgoing);
      if (!links.length) return { center: unit.center, weight: .25 };
      return { center: links.reduce((sum, edge) => sum + personCenter(edge.other) - offsets.get(edge.own), 0) / links.length, weight: links.length };
    }
    function alignRow(row, direction, reorder) {
      if (reorder) row.forEach(unit => { if (unit.members.length > 1) orderMembers(unit, direction); });
      const targets = new Map(row.map(unit => [unit, target(unit, direction)]));
      if (reorder) row.sort((a, b) => targets.get(a).center - targets.get(b).center || a.rank - b.rank);
      // Weighted isotonic compaction: find the nearest feasible coordinates to the
      // real parent/child anchors, respecting unit widths and inter-family spacing.
      // Independent row centering would displace narrow branches across the tree.
      const blocks = [], bases = []; let left = 0;
      row.forEach((unit, index) => {
        const base = left + unit.width / 2, desired = targets.get(unit); bases.push(base);
        left += unit.width + familyGap;
        blocks.push({ start: index, end: index, weight: desired.weight, sum: (desired.center - base) * desired.weight });
        while (blocks.length > 1) {
          const b = blocks[blocks.length - 1], a = blocks[blocks.length - 2];
          if (a.sum / a.weight <= b.sum / b.weight) break;
          a.end = b.end; a.weight += b.weight; a.sum += b.sum; blocks.pop();
        }
      });
      blocks.forEach(block => { for (let i = block.start; i <= block.end; i++) { row[i].center = bases[i] + block.sum / block.weight; row[i].rank = i; } });
    }
    // Fixed work per layer/edge: alternate ancestor and descendant anchors, then
    // settle coordinates against both directions without changing family order.
    for (let pass = 0; pass < 12; pass++) {
      const forward = pass % 2 === 0, direction = pass < 8 ? (forward ? "parents" : "children") : "both";
      (forward ? levels : levels.slice().reverse()).forEach(level => alignRow(rows.get(level), direction, pass < 8));
    }
    let left = Infinity, right = -Infinity;
    units.forEach(unit => { left = Math.min(left, unit.center - unit.width / 2); right = Math.max(right, unit.center + unit.width / 2); });
    const nodes = new Map();
    ids.forEach(id => {
      nodes.set(id, { ...people.get(id), x: personCenter(id) - cardWidth / 2 - left + marginX, y: marginY + generation.levels.get(id) * rowHeight, width: cardWidth, height: cardHeight });
    });
    return { nodes, width: nodes.size ? right - left + 2 * marginX : 2 * marginX, height: (levels.length ? levels[levels.length - 1] * rowHeight : 0) + cardHeight + 2 * marginY };
  }
  const api = { layout, cardWidth, cardHeight };
  if (typeof module !== "undefined" && module.exports) module.exports = api;
  else window.BearStackFamilyTreeLayout = api;
}());
