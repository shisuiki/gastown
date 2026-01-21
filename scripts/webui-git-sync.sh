#!/usr/bin/env bash
set -euo pipefail

repo_root="${GT_WEB_GIT_REPO_ROOT:-}"
commit_msg="${GT_WEB_GIT_COMMIT_MSG:-Update via WebUI}"
paths_csv="${GT_WEB_GIT_PATHS:-}"

if [[ -z "$repo_root" ]]; then
  echo "GT_WEB_GIT_REPO_ROOT not set" >&2
  exit 1
fi

cd "$repo_root"

if [[ -n "$paths_csv" ]]; then
  IFS=',' read -r -a paths <<< "$paths_csv"
  if [[ ${#paths[@]} -gt 0 && -n "${paths[0]}" ]]; then
    git add -- "${paths[@]}"
  else
    git add -A
  fi
else
  git add -A
fi

if git diff --cached --quiet; then
  exit 0
fi

git commit -m "$commit_msg"
git push origin HEAD
