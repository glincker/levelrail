# UI kit

Shared visual primitives. Import from the barrel:

```tsx
import { MetricTile, StatusPill, Suggestion, Timeline } from '@/components/kit'
```

Live preview of every component and state on a light and dark toggle: `/dev/ui` (dev builds only, not in the sidebar).

## Usage

```tsx
<StatusPill tone="success" label="Running" live />

<MetricTile
  label="Requests"
  value={1280}
  unit="/min"
  series={[4, 6, 9, 12]}
  tone="info"
  delta={{ value: 12, direction: 'up', goodWhen: 'up' }}
  info="Requests per minute over the last hour."
/>

<SuggestionList
  emptyLabel="All clear"
  items={[{
    id: 'hc',
    tone: 'info',
    title: 'No health check',
    detail: '/healthz responds.',
    actions: [{ label: 'Add it', onClick: addHealthCheck }],
  }]}
/>

<Timeline items={events} loading={isPending} emptyLabel="No activity yet." />

<ActionMenu
  trigger={<Button variant="ghost">More</Button>}
  items={[{ id: 'rm', label: 'Delete', tone: 'danger', onSelect: remove }]}
/>
```

Tones: `neutral | success | warning | danger | info | accent`. Tokens live in `index.css` as `--tone-<name>-{fg,soft,border,solid}` and Tailwind classes `text-tone-*`, `bg-tone-*-soft`, `border-tone-*-border`, `bg-tone-*-solid`. Shadows: `shadow-raised`, `shadow-floating`. Motion vars: `--motion-fast|base|slow`, `--motion-ease`. Helper classes: `kit-shimmer`, `kit-enter`, `kit-exit`, `kit-pulse-ring` (all disabled under `prefers-reduced-motion`).

## Do

- One focal point per screen; answer the question in the first viewport.
- Prefer a number, sparkline, pill or icon over a sentence. One short sentence at most.
- Put help copy in an `InfoTip`, not inline.
- Use `Suggestion` with a one-click action for anything the app can fix itself.
- Show skeletons while loading and an `EmptyState` with a next action when empty.
- Use `RelativeTime` with `live` for times that should keep updating.
- Keep destructive actions in `ActionMenu` with `tone: 'danger'`.
- Show shortcuts with `Kbd`.

## Don't

- Don't nest bordered boxes inside bordered boxes.
- Don't show contradictory status pills together (Healthy and Reconciling).
- Don't give a destructive button the same weight as the primary one.
- Don't use spinners for page loads; use skeletons.
- Don't use color alone to convey meaning; pair it with a label or icon.
- Don't repeat the same banner on every page.
- Don't add motion that ignores `prefers-reduced-motion`.
