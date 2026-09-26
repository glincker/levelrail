import { useEffect, useState, type ReactNode } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import {
  CloudArrowUpIcon,
  GearIcon,
  GlobeIcon,
  MoonIcon,
  PencilSimpleIcon,
  RocketLaunchIcon,
  ShieldCheckIcon,
  SunIcon,
  TrashIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import {
  ActionMenu,
  AnimatedNumber,
  EmptyState,
  InfoTip,
  Kbd,
  MetricTile,
  RelativeTime,
  SkeletonLine,
  SkeletonList,
  SkeletonTile,
  Sparkline,
  StatusPill,
  Suggestion,
  SuggestionList,
  Timeline,
  TypedText,
  type SuggestionItem,
  type Tone,
} from '@/components/kit'

export const Route = createFileRoute('/dev/ui')({
  component: UiGallery,
})

const TONES: Tone[] = [
  'neutral',
  'success',
  'warning',
  'danger',
  'info',
  'accent',
]
const SERIES = [4, 6, 5, 9, 8, 12, 10, 14, 13, 18]
const FLAT = [5, 5, 5, 5]

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-sm font-semibold text-muted-foreground">{title}</h2>
      {children}
    </section>
  )
}

const ago = (s: number) => new Date(Date.now() - s * 1000)

function initialSuggestions(): SuggestionItem[] {
  const ok = async () => {
    await new Promise((r) => setTimeout(r, 900))
  }
  return [
    {
      id: 'hc',
      tone: 'info',
      icon: <ShieldCheckIcon />,
      title: 'No health check',
      detail: '/healthz responds. Add it as the readiness probe.',
      actions: [
        { label: 'Add it', onClick: ok },
        { label: 'Not now', kind: 'secondary', onClick: () => undefined },
      ],
    },
    {
      id: 'mem',
      tone: 'warning',
      icon: <WarningIcon />,
      title: 'Memory near the limit',
      detail: 'Peaked at 92% in the last hour.',
      actions: [{ label: 'Raise to 1Gi', onClick: ok }],
    },
  ]
}

function UiGallery() {
  const [dark, setDark] = useState(false)
  const [items, setItems] = useState<SuggestionItem[]>(initialSuggestions)
  const [n, setN] = useState(1280)
  const [typedKey, setTypedKey] = useState(0)

  useEffect(() => {
    const root = document.documentElement
    const had = root.classList.contains('dark')
    root.classList.toggle('dark', dark)
    return () => {
      root.classList.toggle('dark', had)
    }
  }, [dark])

  if (import.meta.env.PROD) {
    return (
      <div className="p-10 text-center text-sm text-muted-foreground">
        Page not found.
      </div>
    )
  }

  const timeline = [
    {
      id: '1',
      at: ago(120),
      icon: <RocketLaunchIcon />,
      tone: 'success' as const,
      title: 'Deployed a1b2c3d',
      detail: 'Rolled out in 42s',
      actor: 'gagan',
    },
    {
      id: '2',
      at: ago(3600 * 3),
      icon: <PencilSimpleIcon />,
      tone: 'info' as const,
      title: 'Env changed',
      actor: 'gagan',
      onClick: () => undefined,
    },
    {
      id: '3',
      at: ago(86400 * 2),
      icon: <WarningIcon />,
      tone: 'danger' as const,
      title: 'Deploy failed',
      detail: 'Readiness probe timed out',
    },
  ]

  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-10 bg-background p-6 text-foreground">
      <header className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">UI kit gallery</h1>
        <Button variant="outline" onClick={() => setDark((d) => !d)}>
          {dark ? <SunIcon /> : <MoonIcon />}
          {dark ? 'Light' : 'Dark'}
        </Button>
      </header>

      <Section title="StatusPill">
        <div className="flex flex-wrap gap-2">
          {TONES.map((t) => (
            <StatusPill key={t} tone={t} label={t} live={t === 'success'} />
          ))}
          <StatusPill tone="info" label="small" size="sm" />
          <StatusPill
            tone="accent"
            label="icon"
            icon={<GlobeIcon className="size-3" />}
          />
        </div>
      </Section>

      <Section title="Sparkline">
        <div className="flex flex-wrap items-center gap-6">
          <Sparkline
            values={SERIES}
            tone="success"
            ariaLabel="rising"
            showLast
          />
          <Sparkline
            values={SERIES}
            tone="accent"
            fill
            ariaLabel="area"
            showLast
          />
          <Sparkline values={FLAT} ariaLabel="flat" />
          <Sparkline values={[7]} tone="info" ariaLabel="single" />
          <Sparkline values={[]} ariaLabel="empty" />
        </div>
        <div className="w-full max-w-md">
          <Sparkline
            values={SERIES}
            tone="info"
            width="fill"
            height={48}
            fill
            showLast
            ariaLabel="fill width"
          />
        </div>
      </Section>

      <Section title="MetricTile">
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <MetricTile
            label="Requests"
            value={n}
            unit="/min"
            series={SERIES}
            tone="info"
            icon={<GlobeIcon />}
            delta={{ value: 12, direction: 'up', goodWhen: 'up' }}
            info="Requests per minute over the last hour."
          />
          <MetricTile
            label="Errors"
            value={2.4}
            unit="%"
            series={[1, 2, 2, 3, 5, 4]}
            tone="danger"
            delta={{ value: 8, direction: 'up', goodWhen: 'down' }}
          />
          <MetricTile
            label="p95"
            value="120 ms"
            delta={{ value: 5, direction: 'down', goodWhen: 'down' }}
            onClick={() => undefined}
          />
          <MetricTile label="Loading" value={0} loading />
        </div>
        <div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setN((v) => v + 137)}
          >
            Bump requests
          </Button>
        </div>
      </Section>

      <Section title="AnimatedNumber, TypedText, RelativeTime, Kbd, InfoTip">
        <div className="flex flex-wrap items-center gap-6 text-sm">
          <span className="text-2xl font-semibold">
            <AnimatedNumber value={n} />
          </span>
          <span>
            <TypedText key={typedKey} text="Everything is running smoothly." />
          </span>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setTypedKey((k) => k + 1)}
          >
            Retype
          </Button>
          <span>
            Deployed <RelativeTime at={ago(240)} live />
          </span>
          <Kbd keys={['Ctrl', 'K']} />
          <span className="inline-flex items-center gap-1">
            Health <InfoTip>Probes the app every 5 seconds.</InfoTip>
          </span>
        </div>
      </Section>

      <Section title="Suggestion and SuggestionList">
        <Suggestion
          tone="success"
          icon={<CloudArrowUpIcon />}
          title="Backups are on"
          detail="Last one finished 2h ago."
          actions={[{ label: 'View', onClick: () => undefined }]}
          onDismiss={() => undefined}
        />
        <Suggestion
          tone="danger"
          icon={<WarningIcon />}
          title="Pending state"
          actions={[
            { label: 'Working', pending: true, onClick: () => undefined },
          ]}
        />
        <SuggestionList
          items={items.map((i) => ({
            ...i,
            onDismiss: () => setItems((s) => s.filter((x) => x.id !== i.id)),
          }))}
          emptyLabel="All clear. Nothing to suggest."
        />
        <div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setItems(initialSuggestions())}
          >
            Reset list
          </Button>
        </div>
      </Section>

      <Section title="EmptyState">
        <div className="grid gap-3 sm:grid-cols-2">
          {(['rocket', 'globe', 'chart', 'database', 'cloud'] as const).map(
            (k) => (
              <EmptyState
                key={k}
                icon={<GearIcon />}
                illustration={k}
                title={`No ${k} yet`}
                description="One short sentence about the next step."
                action={<Button size="sm">Get started</Button>}
              />
            ),
          )}
          <EmptyState
            icon={<GearIcon />}
            title="Icon only"
            description="No illustration."
          />
        </div>
      </Section>

      <Section title="Skeletons">
        <div className="grid gap-3 sm:grid-cols-2">
          <div className="flex flex-col gap-2">
            <SkeletonLine />
            <SkeletonLine width="60%" />
          </div>
          <SkeletonTile />
          <SkeletonList rows={2} />
        </div>
      </Section>

      <Section title="ActionMenu">
        <ActionMenu
          trigger={<Button variant="outline">Actions</Button>}
          items={[
            {
              id: 'edit',
              label: 'Edit',
              icon: <PencilSimpleIcon />,
              onSelect: () => undefined,
              description: 'Change settings',
            },
            {
              id: 'dis',
              label: 'Disabled',
              onSelect: () => undefined,
              disabled: true,
            },
            {
              id: 'del',
              label: 'Delete',
              icon: <TrashIcon />,
              tone: 'danger',
              onSelect: () => undefined,
            },
          ]}
        />
      </Section>

      <Section title="Timeline">
        <div className="grid gap-6 sm:grid-cols-2">
          <Timeline items={timeline} />
          <Timeline items={[]} loading />
          <Timeline items={[]} emptyLabel="No activity yet." />
        </div>
      </Section>
    </div>
  )
}
