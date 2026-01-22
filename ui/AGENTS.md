# AGENTS.md — UI (HTML-first, REST-driven)

## Role
You generate a server-driven UI using HTML, CSS, and HTMX.
This is not a frontend application.
This is a website with interactions.

Optimize for predictability, readability, and regeneration safety.

---

## Core Principles
- HTML is the primary artifact.
- REST responses drive UI updates.
- The server owns behavior and state.
- The browser is a renderer, not an application runtime.

---

## HTMX (Strict)
- Use HTMX only to bind user actions to REST endpoints.
- One user action → one HTTP request → one DOM swap.
- No chaining, orchestration, or client-side flow logic.

---

## JavaScript (Very Strict)
- Do NOT introduce JavaScript by default.
- No client-side state management.
- No frontend routing.
- Small, isolated JS snippets are allowed only for UI polish (e.g. carousel).
- If logic feels necessary, it belongs on the server.

---

## HTML
- HTML must be readable as a document.
- Prefer forms and links over JS-driven interactions.
- Avoid deeply nested markup.
- Avoid clever templating logic.

---

## HTML Fragments (Strict)
- Server responses must be HTML fragments, not full documents.
- Do NOT emit <html>, <head>, <body>, or layout wrappers.
- Fragments must be directly swappable into existing containers via HTMX.
- No JavaScript or inline styles in fragments.
- Use semantic HTML with class names only; CSS is applied separately.

---

## CSS
- Plain CSS only.
- No CSS-in-JS.
- No dynamic styling logic.
- Prefer simple class-based styling.

---

## Responsiveness
- UI must work on small, medium, and large screens.
- Avoid fixed widths; prefer flexible layouts.
- Layout must degrade gracefully without JS.

---

## Comments
- Do not comment obvious markup.
- Comments may be used only to explain non-obvious UI constraints.
- When in doubt, remove the comment.

---

## What NOT to Build
- No SPA architecture.
- No client-side models or stores.
- No shared UI state.
- No component frameworks.

---

## Review Rule
If the UI cannot be understood by reading HTML alone, it is wrong.

---

## One-line Rule
This UI should feel like net/http — boring, explicit, and obvious.

