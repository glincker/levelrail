import type { ComponentType, ReactNode, SVGProps } from 'react'
import { useTranslation } from 'react-i18next'
import {
  BracketsGlassIcon,
  ClockGlassIcon,
  DatabaseGlassIcon,
  QueueWorkerGlassIcon,
} from '@thesvg/ui-react'
import { RelativeTime } from '@/components/kit'
import { TONE } from '@/components/kit/tone'
import { cn } from '@/lib/utils'
import type { BackendKind } from '../../lib/appKind'
import { logoIdForImage } from '../../lib/imageLogo'
import type { AppListEntry } from '../../types/appDetail'
import { TemplateLogo } from '../TemplateLogo'

type Glyph = ComponentType<SVGProps<SVGSVGElement>>

const KIND_GLYPH: Record<BackendKind, Glyph> = {
  api: BracketsGlassIcon,
  worker: QueueWorkerGlassIcon,
  cron: ClockGlassIcon,
  database: DatabaseGlassIcon,
}

// Subtle by design: a hairline chip, the card body carries the identity.
export function KindBadge({ kind }: Readonly<{ kind: BackendKind }>) {
  const { t } = useTranslation('appRows')
  return (
    <span className="inline-flex shrink-0 items-center rounded border border-border px-1.5 py-px text-xs font-medium text-muted-foreground">
      {t(`kind.badge.${kind}`)}
    </span>
  )
}

function KindMark({
  kind,
  image,
}: Readonly<{ kind: BackendKind; image: string }>) {
  const Glyph = KIND_GLYPH[kind]
  const logo = logoIdForImage(image)
  return (
    <span className="flex size-10 shrink-0 items-center justify-center rounded-lg border border-border bg-background text-muted-foreground">
      <TemplateLogo
        id={logo ?? ''}
        className="size-5"
        fallback={<Glyph className="size-5" aria-hidden="true" />}
      />
    </span>
  )
}

function Row({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <p className="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
      {children}
    </p>
  )
}

const RUN_TONE: Record<string, string> = {
  succeeded: TONE.success.text,
  success: TONE.success.text,
  failed: TONE.danger.text,
  error: TONE.danger.text,
}

function readyTone(ready: string | undefined, failing: boolean): string {
  if (failing || ready === 'false') return TONE.danger.text
  if (ready === 'true') return TONE.success.text
  return 'text-muted-foreground'
}

function Readiness({
  app,
  failing,
}: Readonly<{ app: AppListEntry; failing: boolean }>) {
  const { t } = useTranslation('appRows')
  const ready = app.summary?.ready
  const key = failing || ready === 'false' ? 'false' : (ready ?? 'unknown')
  return (
    <span className={cn('font-medium', readyTone(ready, failing))}>
      {t(`kind.ready.${key}`)}
    </span>
  )
}

function DeployAge({ at }: Readonly<{ at?: string }>) {
  const { t } = useTranslation('appRows')
  if (!at) return <span>{t('kind.neverDeployed')}</span>
  return (
    <span>
      {t('card.lastDeploy')} <RelativeTime at={at} />
    </span>
  )
}

function Replicas({ count }: Readonly<{ count?: number }>) {
  const { t } = useTranslation('appRows')
  if (count === undefined) return null
  return <span>{t('kind.replicas', { count })}</span>
}

function CronBody({ app }: Readonly<{ app: AppListEntry }>) {
  const { t } = useTranslation('appRows')
  const s = app.summary
  const runTone = RUN_TONE[(s?.last_run_status ?? '').toLowerCase()]
  return (
    <>
      <Row>
        <span className="font-medium text-foreground">{t('kind.nextRun')}</span>
        {s?.next_run_at ? (
          <RelativeTime at={s.next_run_at} />
        ) : (
          <span>{t('kind.notScheduled')}</span>
        )}
      </Row>
      <Row>
        <span className="font-medium text-foreground">{t('kind.lastRun')}</span>
        {s?.last_run_at ? (
          <>
            <RelativeTime at={s.last_run_at} />
            {s.last_run_status ? (
              <span className={cn('font-medium', runTone)}>
                {s.last_run_status}
              </span>
            ) : null}
          </>
        ) : (
          <span>{t('kind.neverRan')}</span>
        )}
      </Row>
    </>
  )
}

function DatabaseBody({ app }: Readonly<{ app: AppListEntry }>) {
  const engine = app.image.split('@')[0]?.split('/').pop()?.split(':')[0]
  const failing = app.status.variant === 'destructive'
  return (
    <>
      <Row>
        <span className="truncate font-mono text-foreground">
          {engine ?? app.image}
        </span>
      </Row>
      <Row>
        <Readiness app={app} failing={failing} />
        <Replicas count={app.summary?.replicas} />
      </Row>
    </>
  )
}

function ServiceBody({
  app,
  kind,
}: Readonly<{ app: AppListEntry; kind: 'api' | 'worker' }>) {
  const failing = app.status.variant === 'destructive'
  const path = app.health?.readiness?.path
  return (
    <>
      <Row>
        <Readiness app={app} failing={failing} />
        <Replicas count={app.summary?.replicas} />
      </Row>
      <Row>
        <DeployAge at={app.summary?.last_deploy_at} />
      </Row>
      {kind === 'api' && path ? (
        <span
          title={path}
          className="max-w-full self-start truncate rounded border border-border px-1.5 py-px text-xs font-mono text-muted-foreground"
        >
          {path}
        </span>
      ) : null}
    </>
  )
}

// Fills the 16:9 media slot for apps with no visual UI, so grids stay even.
export function AppKindCard({
  app,
  kind,
}: Readonly<{ app: AppListEntry; kind: BackendKind }>) {
  const { t } = useTranslation('appRows')
  return (
    <div
      role="group"
      aria-label={t('kind.cardLabel', {
        name: app.name,
        kind: t(`kind.badge.${kind}`),
      })}
      data-testid={`kind-card-${kind}`}
      className="absolute inset-0 flex items-center gap-3 bg-muted/30 p-3"
    >
      <KindMark kind={kind} image={app.image} />
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <span className="mb-0.5 self-start">
          <KindBadge kind={kind} />
        </span>
        {kind === 'cron' ? <CronBody app={app} /> : null}
        {kind === 'database' ? <DatabaseBody app={app} /> : null}
        {kind === 'api' || kind === 'worker' ? (
          <ServiceBody app={app} kind={kind} />
        ) : null}
      </div>
    </div>
  )
}
