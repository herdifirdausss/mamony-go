---
name: mamony-technical-writing
description: Use when revising AI task briefs, tool descriptions, error messages, incident runbooks, or important English technical instructions where ambiguity can cause incorrect implementation.
---
# Unambiguous technical English

Informed by [ASD-STE100 agent skill](https://github.com/danyuchn/asd-ste100-skill); this is independently authored and does not claim full ASD-STE100 compliance.

1. Identify the actor, observable action, input, output, preconditions and expected failure response. Keep one primary command or requirement per sentence.
2. Prefer one meaning per technical term; define terms such as `Post`, `journal`, `replay` and `tenant` from the relevant contract. Avoid vague qualifiers such as 'secure', 'properly' or 'as needed' without testable conditions.
3. Keep MUST/SHOULD/PROPOSED/NOT RUN exact; do not turn drafts into commitments or narrow permissions based on prose style.
4. Use short active sentences and consistent vocabulary. Do not remove an accounting invariant, timeout, negative condition, recovery action, error code, or requirement ID to shorten text.
5. For behavior-changing edits, propose wording and an acceptance test side by side. Report meaning changes, not just grammar corrections.
