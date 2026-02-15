const TYPE_MAPPING = {
  bug: 'Bug',
  enhancement: 'Enhancement',
  documentation: 'Documentation',
  feature: 'Enhancement',
  docs: 'Documentation',
  infra: 'Infra',
  refactor: 'Refactor',
  release: 'Release',
};

function inferType(labels) {
  for (const label of labels) {
    const normalized = String(label.name || '').trim().toLowerCase();
    if (TYPE_MAPPING[normalized]) {
      return TYPE_MAPPING[normalized];
    }
  }
  return null;
}

let passed = 0;
let failed = 0;

function assertEqual(actual, expected, message) {
  if (actual === expected) {
    passed++;
    return;
  }
  failed++;
  console.error(`FAIL: ${message}; expected=${expected} actual=${actual}`);
}

assertEqual(inferType([{ name: 'bug' }]), 'Bug', 'bug maps to Bug');
assertEqual(inferType([{ name: 'enhancement' }]), 'Enhancement', 'enhancement maps to Enhancement');
assertEqual(inferType([{ name: 'documentation' }]), 'Documentation', 'documentation maps to Documentation');
assertEqual(inferType([{ name: 'feature' }]), 'Enhancement', 'feature alias maps to Enhancement');
assertEqual(inferType([{ name: 'docs' }]), 'Documentation', 'docs alias maps to Documentation');
assertEqual(inferType([{ name: 'infra' }]), 'Infra', 'infra maps to Infra');
assertEqual(inferType([{ name: 'refactor' }]), 'Refactor', 'refactor maps to Refactor');
assertEqual(inferType([{ name: 'release' }]), 'Release', 'release maps to Release');
assertEqual(inferType([{ name: 'unknown' }]), null, 'unknown label maps to null');
assertEqual(inferType([{ name: 'P1' }, { name: 'Documentation' }]), 'Documentation', 'case-insensitive matching');

console.log(`kanban-type-inference: ${passed} passed, ${failed} failed`);
if (failed > 0) {
  process.exit(1);
}
