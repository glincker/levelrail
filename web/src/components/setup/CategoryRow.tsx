import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CaretDownIcon,
  CheckCircleIcon,
  CircleNotchIcon,
  CpuIcon,
  GlobeHemisphereWestIcon,
  HardDrivesIcon,
  ShieldCheckIcon,
  StackIcon,
  WarningCircleIcon,
  XCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { cn } from '@/lib/utils'
import { TONE } from '../kit/tone'
import type { Tone } from '../kit/tone'
import type {
  CategoryReadiness,
  CategoryVerdict,
  ReadinessCategoryId,
} from '../../lib/setupReadiness'
import type { DoctorCheckStatus } from '../../queries/systemDoctor'

const CATEGORY_ICON: Record<ReadinessCategoryId, Icon> = {
  runtime: StackIcon,
  network: GlobeHemisphereWestIcon,
  security: ShieldCheckIcon,
  storage: HardDrivesIcon,
  capacity: CpuIcon,
}

const VERDICT_TONE: Record<CategoryVerdict, Tone> = {
  ready: 'success',
  attention: 'warning',
  blocked: 'danger',
}

const STATUS_ICON: Record<DoctorCheckStatus, Icon> = {
  ok: CheckCircleIcon,
  warn: WarningCircleIcon,
  fail: XCircleIcon,
  unknown: CircleNotchIcon,
}

const STATUS_TONE: Record<DoctorCheckStatus, Tone> = {
  ok: 'success',
  warn: 'warning',
  fail: 'danger',
  unknown: 'neutral',
}

function Chip({ tone, children }: { tone: Tone; children: string }) {
  const c = TONE[tone]
  return (
    <span
      className={cn(
        'rounded-full border px-2 py-0.5 text-xs tabular-nums',
        c.text,
        c.soft,
        c.border,
      )}
    >
      {children}
    </span>
  )
}

/** CategoryRow is one readiness category: a verdict line and count chips, expandable into the individual checks. */
export function CategoryRow({
  category,
  pending,
}: {
  category: CategoryReadiness | undefined
  pending?: boolean
}) {
  const { t } = useTranslation('setup')
  const [open, setOpen] = useState(false)
  if (!category) return null
  const id = category.id
  const CategoryIcon = CATEGORY_ICON[id]
  const tone = VERDICT_TONE[category.verdict]
  const issues = category.warn + category.fail

  let verdict = t('server.verdict.ready')
  if (category.verdict === 'blocked') verdict = t('server.verdict.blocked')
  else if (category.verdict === 'attention') {
    verdict = t('server.verdict.attention', { count: issues })
  }

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className="rounded-xl border border-border bg-card"
    >
      <CollapsibleTrigger
        disabled={pending}
        className="flex w-full items-center gap-3 rounded-xl p-3 text-left outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
      >
        <span
          className={cn(
            'flex size-9 shrink-0 items-center justify-center rounded-lg border',
            TONE[tone].soft,
            TONE[tone].border,
            TONE[tone].text,
          )}
        >
          {pending ? (
            <CircleNotchIcon
              className="size-5 animate-spin motion-reduce:animate-none"
              aria-hidden="true"
            />
          ) : (
            <CategoryIcon className="size-5" aria-hidden="true" />
          )}
        </span>
        <span className="min-w-0 flex-1">
          <span className="block text-sm font-medium text-foreground">
            {t(`server.categories.${id}.name`)}
          </span>
          <span className="block truncate text-xs text-muted-foreground">
            {pending ? t('server.running') : verdict}
            {pending ? null : (
              <span className="hidden sm:inline">
                {' · '}
                {t(`server.categories.${id}.blurb`)}
              </span>
            )}
          </span>
        </span>
        {pending ? null : (
          <span className="hidden shrink-0 items-center gap-1.5 sm:flex">
            {category.ok > 0 ? (
              <Chip tone="success">
                {t('server.chips.passed', { count: category.ok })}
              </Chip>
            ) : null}
            {category.warn > 0 ? (
              <Chip tone="warning">
                {t('server.chips.warn', { count: category.warn })}
              </Chip>
            ) : null}
            {category.fail > 0 ? (
              <Chip tone="danger">
                {t('server.chips.fail', { count: category.fail })}
              </Chip>
            ) : null}
            {category.unknown > 0 ? (
              <Chip tone="neutral">
                {t('server.chips.unknown', { count: category.unknown })}
              </Chip>
            ) : null}
          </span>
        )}
        {pending ? null : (
          <>
            <CaretDownIcon
              className={cn(
                'size-4 shrink-0 text-muted-foreground transition-transform motion-reduce:transition-none',
                open && 'rotate-180',
              )}
              aria-hidden="true"
            />
            <span className="sr-only">
              {open ? t('server.collapse') : t('server.expand')}
            </span>
          </>
        )}
      </CollapsibleTrigger>
      <CollapsiblePanel>
        <ul className="space-y-2 border-t border-border px-3 py-3">
          {category.checks.map((check) => {
            const StatusIcon = STATUS_ICON[check.status]
            return (
              <li key={check.code} className="flex items-start gap-2.5">
                <StatusIcon
                  className={cn(
                    'mt-0.5 size-4 shrink-0',
                    TONE[STATUS_TONE[check.status]].text,
                  )}
                  aria-hidden="true"
                />
                <span className="min-w-0 text-sm">
                  <span className="font-medium text-foreground">
                    {check.name}
                  </span>
                  <span className="block break-words text-xs text-muted-foreground">
                    {check.message}
                  </span>
                </span>
              </li>
            )
          })}
        </ul>
      </CollapsiblePanel>
    </Collapsible>
  )
}
