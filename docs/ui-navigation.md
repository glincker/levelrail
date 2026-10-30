---
description: Rules for where a new dashboard setting or page goes, and the size caps that keep the sidebars lean.
---

# UI navigation

## Where a new setting goes

1. A new setting is a section with an anchor on an existing page, not a new page.
2. A new page is justified only when it has its own lifecycle (its own data, status and actions).
3. Sibling pages under one topic become tabs on a single sidebar entry, as Platform does.
4. Every setting also gets a Cmd+K palette entry that deep links to its anchor.
5. The global sidebar holds at most 40 items.
6. A Settings group holds at most 8 items.
7. No new Settings group is added without owner approval.
8. Groups start closed except the one holding the current page, and each viewer's open state is remembered locally.
9. Prefer a status strip or a tile on an existing page over a new entry that only repeats information.
10. When a cap is reached, merge items into tabs before asking for a new group.
