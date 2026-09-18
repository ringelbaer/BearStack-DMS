// Pure, deterministic layout shared by the browser and regression tests.
(function () {
  "use strict";
  const cardWidth = 250, cardHeight = 144, gap = 38, rowHeight = 250;
  function layout(tree) {
    const people = new Map(tree.people.map(person => [person.id, person]));
    const parents = tree.relations.filter(edge => edge.kind === "mother" || edge.kind === "father");
    function groups(couple) {
      const roots = new Map(tree.people.map(person => [person.id, person.id]));
      function find(id) { let current = id; while (roots.get(current) !== current) { roots.set(current, roots.get(roots.get(current))); current = roots.get(current); } return current; }
      if (couple) tree.relations.forEach(edge => {
        if (edge.kind !== "marriage" && edge.kind !== "sibling") return;
        const a = find(edge.from), b = find(edge.to); roots.set(Math.max(a, b), Math.min(a, b));
      });
      const result = new Map(), membership = new Map();
      tree.people.forEach(person => { const id = find(person.id); if (!result.has(id)) result.set(id, { id, members: [], next: new Set(), prev: new Set(), level: 0, order: 0 }); result.get(id).members.push(person.id); membership.set(person.id, id); });
      let conflict = false;
      parents.forEach(edge => { const a = membership.get(edge.from), b = membership.get(edge.to); if (a === b) { conflict = true; return; } result.get(a).next.add(b); result.get(b).prev.add(a); });
      const indegree = new Map(Array.from(result.values(), group => [group.id, group.prev.size]));
      const ready = Array.from(result.values()).filter(group => !group.prev.size).sort((a, b) => a.id - b.id); let head = 0;
      while (head < ready.length) {
        const group = ready[head++]; group.next.forEach(id => { const next = result.get(id); next.level = Math.max(next.level, group.level + 1); indegree.set(id, indegree.get(id) - 1); if (!indegree.get(id)) ready.push(next); });
      }
      return { result, conflict: conflict || head !== result.size };
    }
    let grouped = groups(true);
    // Cross-generation marriages or inconsistent sibling records must not hide
    // people or imply invented parentage. Keep those links across rows instead.
    if (grouped.conflict) grouped = groups(false);
    const rows = new Map();
    grouped.result.forEach(group => { if (!rows.has(group.level)) rows.set(group.level, []); rows.get(group.level).push(group); group.members.sort((a, b) => a - b); });
    const levels = Array.from(rows.keys()).sort((a, b) => a - b);
    levels.forEach(level => rows.get(level).sort((a, b) => a.id - b.id).forEach((group, index) => { group.order = index; }));
    // A bounded number of barycentric sweeps reduces crossings without pairwise
    // comparisons between all people or unbounded layout iterations.
    for (let pass = 0; pass < 4; pass++) {
      const ascending = pass % 2 === 0;
      (ascending ? levels : levels.slice().reverse()).forEach(level => {
        const row = rows.get(level), scores = new Map();
        row.forEach(group => { const neighbors = Array.from(ascending ? group.prev : group.next); scores.set(group.id, neighbors.length ? neighbors.reduce((sum, id) => sum + grouped.result.get(id).order, 0) / neighbors.length : group.order); });
        row.sort((a, b) => scores.get(a.id) - scores.get(b.id) || a.id - b.id).forEach((group, index) => { group.order = index; });
      });
    }
    let width = 0;
    const widths = new Map();
    levels.forEach(level => { const size = rows.get(level).reduce((sum, group) => sum + group.members.length * (cardWidth + gap) + gap, 0); widths.set(level, size); width = Math.max(width, size); });
    const nodes = new Map();
    levels.forEach(level => {
      let x = 48 + (width - widths.get(level)) / 2;
      rows.get(level).forEach(group => { group.members.forEach(id => { nodes.set(id, { ...people.get(id), x, y: 64 + level * rowHeight, width: cardWidth, height: cardHeight }); x += cardWidth + gap; }); x += gap; });
    });
    return { nodes, width: width + 96, height: (levels.length ? Math.max(...levels) : 0) * rowHeight + cardHeight + 128 };
  }
  const api = { layout, cardWidth, cardHeight };
  if (typeof module !== "undefined" && module.exports) module.exports = api;
  else window.BearStackFamilyTreeLayout = api;
}());
