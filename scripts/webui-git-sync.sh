#!/usr/bin/env bash
set -euo pipefail

repo_root="${GT_WEB_GIT_REPO_ROOT:-}"
commit_msg="${GT_WEB_GIT_COMMIT_MSG:-Update via WebUI}"
paths_csv="${GT_WEB_GIT_PATHS:-}"
author_name="${GT_WEB_GIT_AUTHOR_NAME:-}"
author_email="${GT_WEB_GIT_AUTHOR_EMAIL:-}"
committer_name="${GT_WEB_GIT_COMMITTER_NAME:-}"
committer_email="${GT_WEB_GIT_COMMITTER_EMAIL:-}"

if [[ -z "$repo_root" ]]; then
  echo "GT_WEB_GIT_REPO_ROOT not set" >&2
  exit 1
fi

if [[ -n "$author_name" ]]; then
  export GIT_AUTHOR_NAME="$author_name"
fi
if [[ -n "$author_email" ]]; then
  export GIT_AUTHOR_EMAIL="$author_email"
fi
if [[ -z "$committer_name" && -n "$author_name" ]]; then
  committer_name="$author_name"
fi
if [[ -z "$committer_email" && -n "$author_email" ]]; then
  committer_email="$author_email"
fi
if [[ -n "$committer_name" ]]; then
  export GIT_COMMITTER_NAME="$committer_name"
fi
if [[ -n "$committer_email" ]]; then
  export GIT_COMMITTER_EMAIL="$committer_email"
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
