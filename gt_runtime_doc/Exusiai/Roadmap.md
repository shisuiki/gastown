# Roadmap

## Phase 0: Requirements + survey
- Audit current nav layout and entry points for config/dev pages.
- Inspect account CLI behavior to mirror in WebUI.
- Decide account API shape and UI flow.

## Phase 1: Account API + page
- Add Accounts page to WebUI with list/add/default/switch actions.
- Implement account API endpoints for list/status/add/default/switch.
- Surface post-add login instructions and switch warnings.

## Phase 2: Navigation refactor
- Replace flat nav with main links + Cfg/Dev dropdowns.
- Ensure active state highlights and mobile dropdown behavior.
- Keep existing routes intact.

## Phase 3: UX validation
- Confirm account operations reflect `gt account` behavior.
- Verify nav works on mobile and desktop.

## Acceptance criteria
- WebUI accounts page covers list/add/default/switch.
- Nav matches requested structure with dropdowns and stays usable on mobile.
