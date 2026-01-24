#!/usr/bin/env bash
set -euo pipefail

python - <<'PY'
import pathlib
import re
import sys

docs_root = pathlib.Path('docs').resolve()
md_files = sorted(docs_root.rglob('*.md'))
errors = []
warnings = []

required_frontmatter_keys = {'type', 'status', 'owner'}
entry_points = [docs_root / 'overview.md', docs_root / 'operations' / 'README.md']
linked_docs = set()

link_pattern = re.compile(r'\[[^\]]+\]\(([^)]+)\)')

for ep in entry_points:
    text = ep.read_text(encoding='utf-8')
    for match in link_pattern.findall(text):
        target = match.strip()
        if not target or target.startswith('http') or target.startswith('#'):
            continue
        normalized = (ep.parent / target).resolve()
        if not normalized.exists():
            errors.append(f"Missing link target: {target} referenced from {ep}")
            continue
        try:
            rel = normalized.relative_to(docs_root)
        except ValueError:
            continue
        linked_docs.add(str(rel).replace('\\', '/'))

all_docs = set()
for path in md_files:
    rel = path.relative_to(docs_root)
    rel_str = str(rel).replace('\\', '/')
    all_docs.add(rel_str)
    text = path.read_text(encoding='utf-8')
    if not text.lstrip().startswith('---'):
        errors.append(f"Missing frontmatter in {rel_str}")
        continue
    parts = text.split('---', 2)
    if len(parts) < 3:
        errors.append(f"Malformed frontmatter in {rel_str}")
        continue
    frontmatter = parts[1]
    front_lines = [line.strip() for line in frontmatter.splitlines() if line.strip()]
    keys = {}
    for line in front_lines:
        if ':' not in line:
            continue
        key, value = line.split(':', 1)
        keys[key.strip()] = value.strip()
    missing = required_frontmatter_keys - keys.keys()
    if missing:
        errors.append(f"Missing frontmatter keys {missing} in {rel_str}")
    doc_type = keys.get('type', '')
    if doc_type == 'runbook':
        if 'ttl_days' not in frontmatter:
            errors.append(f"Runbook {rel_str} missing ttl_days")
        body = parts[2]
        for heading in ('## Preconditions', '## Steps', '## Verification'):
            if heading not in body:
                errors.append(f"Runbook {rel_str} missing section: {heading}")

skip_orphans = {
    'overview.md',
    'operations/README.md',
    'operations/_steward_worklog.md',
    '_inventory.md',
}

orphans = []
for rel in sorted(all_docs):
    if rel.startswith('archive/'):
        continue
    if rel in skip_orphans:
        continue
    if rel not in linked_docs:
        orphans.append(rel)

if orphans:
    warnings.append(f"Orphaned docs (not referenced from overview or operations README): {', '.join(orphans)}")

for error in errors:
    print('error:', error)
for warning in warnings:
    print('warning:', warning)

if errors:
    sys.exit(1)
else:
    if warnings:
        sys.exit(0)
    print('docs lint: clean')
    sys.exit(0)
PY
