# Roadmap

## Phase 0: Investigation + requirements
- Confirm where terminal history is stored today (front-end only vs backend).
- Determine backend storage options (existing store vs new local JSON).
- Decide minimal API contract for reading/writing history.

## Phase 1: Backend history storage
- Add `/api/terminal/history` GET/POST endpoints.
- Persist history in a local store under the rig runtime dir.
- Validate keys and cap history size.

## Phase 2: Frontend integration
- Load history from API with localStorage fallback.
- Append new entries to API store; keep UI behavior unchanged.
- Provide clear handling for missing/empty history.

## Phase 3: UX validation + guardrails
- Confirm multi-device visibility of history.
- Ensure scroll performance with large histories.
- Add minimal constraints to prevent runaway growth.

## Acceptance criteria
- Terminal history persists across devices.
- LocalStorage is fallback only; API is primary source.
- API writes are bounded and safe under concurrent use.
