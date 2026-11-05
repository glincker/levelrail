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

## Layout primitives

```tsx
<TileGrid>
  <Tile title="Apps" size="S" icon={<BoxGlassIcon />} action={<Button size="sm">Add</Button>}>
    12
  </Tile>
  <Tile title="Alerts" size="M" empty={alerts.length === 0} hideWhenEmpty loading={isPending} error={isError} errorLabel="Could not load" onRetry={refetch}>
    ...
  </Tile>
  <Tile title="Activity" size="L" rows={2} footer={<Link to="/audit">All activity</Link>}>...</Tile>
</TileGrid>

<SettingsLayout sections={[{ id: 'color', label: 'Color' }, { id: 'type', label: 'Type' }]}>
  <SettingsSection id="color" title="Color" description="Theme and accent." icon={<PaletteGlassIcon />}>
    <Card>...</Card>
    <Card>...</Card>
  </SettingsSection>
</SettingsLayout>
```

`TileGrid` is a dense 12-column grid (1 column below sm, 6 from md, 12 from xl). `Tile` sizes: `S` is 3 of 12, `M` 6, `L` 12 (6 on md). `hideWhenEmpty` plus `empty` drops the tile and the grid reflows. Tiles fade in with a 30ms stagger, capped, and skip it under reduced motion.

`SettingsLayout` shows a sticky table of contents from xl (active section via IntersectionObserver, `aria-current`) and a native "Jump to" select below. `SettingsSection` takes `hideHeading` when its content already has titles and `columns={1}` for a single card column.

## Do

- One focal point per screen; answer the question in the first viewport.
- Prefer a number, sparkline, pill or icon over a sentence. One short sentence at most.
- Put help copy in an `InfoTip`, not inline. Pass `helpPath`/`helpLabel` to add a docs link below the copy; it renders nothing if that path has no bundled or hosted page.
- Use `Suggestion` with a one-click action for anything the app can fix itself.
- Show skeletons while loading and an `EmptyState` (`@/components/ui/empty-state`) with a next action when empty.
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
