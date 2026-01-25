# Roadmap

## Phase 0: Requirements
- Locate `gt sling` implementation and current argument parsing.
- Define behavior for self-sling warning and confirmation flag.
- Identify user-facing messaging and exit status expectations.

## Phase 1: Implementation
- Detect self-sling target (current agent/rig) in `gt sling`.
- Add warning message with hint to use a confirmation flag.
- Add confirmation flag to allow explicit self-sling.

## Phase 2: Validation
- Verify normal sling behavior unchanged for other targets.
- Verify self-sling without flag warns and aborts.
- Verify self-sling with flag proceeds.

## Acceptance criteria
- `gt sling` warns and stops when target resolves to the current agent unless confirmation flag is provided.
- Flag name and help text clearly communicate intent.
- Changes are covered by runtime docs and pushed immediately.
