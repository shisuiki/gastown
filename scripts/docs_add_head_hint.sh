#!/usr/bin/env bash
set -euo pipefail

git diff --name-only --diff-filter=AM -- docs | while read -r file; do
  [[ -z "$file" ]] && continue
  [[ ! -f "$file" ]] && continue
  first_line=$(sed -n '1p' "$file" 2>/dev/null || true)
  if [[ "$first_line" != '---' ]]; then
    echo "warning: $file is missing frontmatter. Copy docs/_templates/runbook.md or docs/_templates/evergreen.md and tweak the head." >&2
  fi
done
