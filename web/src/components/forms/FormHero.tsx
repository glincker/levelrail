import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ChevronDownGlassIcon,
  ClipboardListGlassIcon,
  InboxGlassIcon,
  PencilLineGlassIcon,
} from '@thesvg/ui-react'
import { cn } from '@/lib/utils'
import { usePlayback } from './usePlayback'
import './formHero.css'

const STEPS = [
  { key: 'design', Icon: PencilLineGlassIcon },
  { key: 'embed', Icon: ClipboardListGlassIcon },
  { key: 'read', Icon: InboxGlassIcon },
] as const

function HeroSvg() {
  const { t } = useTranslation('forms')
  const { ref, motion, paused } = usePlayback<HTMLDivElement>()
  return (
    <div
      ref={ref}
      className="lr-hero"
      data-motion={motion ? 'on' : 'off'}
      data-paused={paused ? 'true' : 'false'}
      data-testid="form-hero"
    >
      <svg
        viewBox="0 0 640 190"
        role="img"
        aria-label={t('hero.alt')}
        className="h-auto w-full max-w-3xl"
      >
        <g className="fill-card stroke-border" strokeWidth="1.5">
          <rect x="16" y="20" width="190" height="150" rx="10" />
          <rect x="434" y="20" width="190" height="150" rx="10" />
          <rect
            className="lr-node"
            x="285"
            y="70"
            width="70"
            height="50"
            rx="12"
          />
        </g>
        <path
          d="M206 95 H285 M355 95 H434"
          className="stroke-border"
          strokeWidth="1.5"
          strokeDasharray="4 5"
          fill="none"
        />
        <g className="fill-muted stroke-border" strokeWidth="1">
          <rect x="32" y="48" width="158" height="20" rx="5" />
          <rect x="32" y="76" width="158" height="20" rx="5" />
          <rect x="32" y="104" width="158" height="26" rx="5" />
        </g>
        <rect
          className="lr-fill lr-fill1 fill-muted-foreground"
          x="38"
          y="55"
          width="100"
          height="6"
          rx="3"
        />
        <rect
          className="lr-fill lr-fill2 fill-muted-foreground"
          x="38"
          y="83"
          width="124"
          height="6"
          rx="3"
        />
        <rect
          className="lr-btn fill-primary"
          x="32"
          y="142"
          width="60"
          height="18"
          rx="5"
        />
        <rect
          className="fill-muted stroke-border"
          x="450"
          y="48"
          width="158"
          height="20"
          rx="5"
          opacity="0.6"
        />
        <g className="lr-row">
          <rect
            className="fill-muted stroke-border"
            x="450"
            y="76"
            width="158"
            height="20"
            rx="5"
          />
          <rect
            className="fill-primary"
            x="450"
            y="76"
            width="4"
            height="20"
            rx="2"
          />
          <rect
            className="fill-muted-foreground"
            x="462"
            y="83"
            width="90"
            height="6"
            rx="3"
          />
        </g>
        <g className="lr-chip">
          <rect
            className="fill-primary/15 stroke-primary"
            x="470"
            y="138"
            width="130"
            height="22"
            rx="11"
          />
          <text
            x="535"
            y="153"
            textAnchor="middle"
            className="fill-foreground text-[10px]"
          >
            {t('hero.notice')}
          </text>
        </g>
        <circle className="lr-packet fill-primary" cx="100" cy="151" r="6" />
        <g className="fill-muted-foreground text-[10px]" textAnchor="middle">
          <text x="111" y="188">
            {t('hero.site')}
          </text>
          <text x="320" y="140">
            {t('hero.endpoint')}
          </text>
          <text x="529" y="188">
            {t('hero.inbox')}
          </text>
        </g>
      </svg>
    </div>
  )
}

export function FormSteps() {
  const { t } = useTranslation('forms')
  return (
    <ol className="grid gap-3 sm:grid-cols-3">
      {STEPS.map(({ key, Icon }, i) => (
        <li
          key={key}
          className="flex items-start gap-3 rounded-lg border border-border bg-card p-3"
        >
          <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
            <Icon className="size-4" />
          </span>
          <div className="min-w-0">
            <p className="text-sm font-medium">
              {i + 1}. {t(`steps.${key}.title`)}
            </p>
            <p className="text-xs text-muted-foreground">
              {t(`steps.${key}.body`)}
            </p>
          </div>
        </li>
      ))}
    </ol>
  )
}

function HeroBody() {
  const { t } = useTranslation('forms')
  return (
    <div className="space-y-4">
      <p className="max-w-2xl text-sm text-muted-foreground">{t('intro')}</p>
      <HeroSvg />
      <p className="text-xs text-muted-foreground">{t('hero.caption')}</p>
      <FormSteps />
    </div>
  )
}

// Full banner when the app has no forms yet, a collapsible "How it works" once it has some.
export function FormHero({ compact }: { compact: boolean }) {
  const { t } = useTranslation('forms')
  const [open, setOpen] = useState(false)
  if (!compact) {
    return (
      <section className="rounded-xl border border-border bg-card/50 p-4">
        <HeroBody />
      </section>
    )
  }
  return (
    <section className="rounded-xl border border-border">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
        className="flex w-full items-center justify-between gap-2 px-4 py-2.5 text-left text-sm font-medium"
      >
        {t('howItWorks')}
        <ChevronDownGlassIcon
          className={cn('size-4.5 transition-transform', open && 'rotate-180')}
        />
      </button>
      {open && (
        <div className="border-t border-border p-4">
          <HeroBody />
        </div>
      )}
    </section>
  )
}
