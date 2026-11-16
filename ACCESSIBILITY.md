# Accessibility statement

Last reviewed: 2026-11-16, against the 1.4.0 release line.

## Target and status

The goal for the web dashboard and the documentation site is WCAG 2.2 level AA. This is a goal, not a claim. Levelrail has not been through an external accessibility audit and does not claim conformance with any level. This page lists what is checked in the repository today and what is not.

## What is checked today

- **Automated DOM checks.** `web/src/test/a11y.ts` flags images without alt text, buttons and links without an accessible name, form fields without a label, ARIA roles without a name, positive tabindex, focusable elements inside `aria-hidden`, broken `aria-labelledby` references, nested interactive controls, empty headings, iframes without a title and duplicate ids. It is a purpose-built checker, not the axe-core library.
- **Page sweep.** `web/src/test/pagesA11y.test.tsx` mounts real routes (dashboard home, apps, deploys and others) and runs those checks. Some component tests run them too (`web/src/test/a11y.test.tsx`).
- **Color contrast.** `web/src/test/contrast.test.ts` reads the theme tokens and requires 4.5:1 for text pairs and 3:1 for non-text parts such as control borders and focus rings, in the themes it covers. A custom accent color is nudged until its text contrast reaches 4.5:1 (`web/src/lib/accentContrast.ts`).
- **Keyboard.** The dashboard has a skip to content link (`web/src/components/shell/SkipLink.tsx`, tested in `AppHeader.test.tsx`). Buttons use a visible `focus-visible` ring. List shortcuts are documented in [Keyboard shortcuts](docs/keyboard.md) and can be turned off in Settings > Appearance, because single key shortcuts can clash with screen readers and speech input (WCAG 2.1.4). Dialogs and menus use Base UI primitives; this repository has no dedicated test of their focus trapping.
- **Reduced motion.** `web/src/index.css` and the motion style sheets have `prefers-reduced-motion: reduce` rules, and the shared button disables its press animation under `motion-reduce`.
- **Status is not color only.** The shared status badge takes an icon next to its label (`web/src/components/ui/status-badge.tsx`). This is a component feature; it is not verified for every screen.
- **Language.** English is the source language and Spanish ships. Other registered languages fall back to English. See [i18n](docs/i18n.md).

## How to run the checks

```
cd web
npx vitest run src/test/pagesA11y
npx vitest run src/test/a11y.test.tsx src/test/contrast.test.ts
```

## Known gaps

- No external audit and no recorded screen reader testing. Do not assume any screen reader works well.
- The automated checks are static DOM rules in jsdom. They do not check rendered color, focus order, focus return after a dialog closes, or reading order.
- `web/eslint.config.js` does not enable a JSX accessibility lint plugin.
- Many older components still hardcode English text (see [i18n](docs/i18n.md)), so translated UIs are partly English.
- The pages in the sweep are a sample, not every route.
- The documentation site is built with a hosted docs generator. Its own markup is not checked by this repository.

## Command line and API

- The CLI prints plain text. With `CI=true`, `LEVELRAIL_CI=1` or when stdin or stdout is not a terminal it prints no color and no animation, and `NO_COLOR` is honored ([CLI reference](docs/cli-reference.md)). Most commands accept `--json`.
- The API returns JSON and carries no presentation. It is documented in the [API reference](docs/api-reference.md).
- Whether every CLI message avoids color-only meaning has not been audited.

## Report a problem

Accessibility problems are normal bugs. Open a [bug report](https://github.com/glincker/levelrail/issues/new?template=bug_report.yml) with the page, the browser, any assistive technology and the steps. For documentation problems use the [docs problem template](https://github.com/glincker/levelrail/issues/new?template=docs_problem.yml). General help is in [SUPPORT.md](SUPPORT.md). Support is best effort and there is no response time commitment.

The [security policy](.github/SECURITY.md) is only for vulnerabilities, not for accessibility reports.
