# Roadmap

## Phase 0: Requirements
- Review existing crew/mayor prompts and locate Runtime Docs references.
- Translate new requirements into concise, agent-friendly prompt text.
- Identify all human-dependency lines to remove.

## Phase 1: Prompt updates (long)
- Update crew long prompt Runtime Docs Protocol (Memory/Roadmap/Progress/summary/Log).
- Update mayor long prompt Runtime Docs Protocol similarly.
- Remove human-dependency guidance from crew/mayor long prompts.
- Add Log.md append-only instructions.

## Phase 2: Prompt updates (short)
- Update crew system prompt with Runtime Docs protocol.
- Update mayor system prompt with Runtime Docs protocol.
- Remove any “escalate to humans” guidance.

## Phase 3: Validation
- Ensure prompts describe Roadmap/Progress/summary roles clearly.
- Ensure Log.md instruction is explicit: append with timestamp using `>>`, never read.
- Confirm autonomy language (no “mail the human” dependency).

## Acceptance criteria
- Crew and Mayor prompts include clear Runtime Docs guidance (Memory/Roadmap/Progress/summary/Log).
- No human-dependency lines remain in crew/mayor prompts.
- Log.md is documented as append-only with timestamps.
