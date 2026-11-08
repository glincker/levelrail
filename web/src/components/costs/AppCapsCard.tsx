import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import {
  appPauseUrl,
  spendCapsQueryOptions,
  usePausePreview,
  useSpendCapActions,
  type SpendCap,
} from '../../queries/spendCap'
import { OverrideDeploysDialog } from '../spend/OverrideDeploysDialog'
import { PausePreviewDialog } from '../spend/PausePreviewDialog'

function CapRow({ cap }: { cap: SpendCap }) {
  const { t } = useTranslation('costs')
  const { save } = useSpendCapActions(cap.app)
  const preview = usePausePreview(appPauseUrl(cap.app))
  const [ceiling, setCeiling] = useState(String(cap.monthly_cost_ceiling))
  const [previewOpen, setPreviewOpen] = useState(false)
  const [overrideOpen, setOverrideOpen] = useState(false)
  const pct = Math.min(100, Math.round(cap.pct))
  const parsed = Number(ceiling)
  const dirty =
    ceiling.trim() !== '' &&
    Number.isFinite(parsed) &&
    parsed >= 0 &&
    parsed !== cap.monthly_cost_ceiling
  const blocked = cap.guard?.blocked === true
  const overridden = cap.guard?.override !== undefined

  const submit = () => {
    if (!dirty) return
    save.mutate(
      {
        monthly_egress_bytes: cap.monthly_egress_bytes,
        monthly_cost_ceiling: parsed,
        thresholds: cap.thresholds,
        action: cap.action,
        throttle_rps: cap.throttle_rps,
      },
      {
        onSuccess: () =>
          toast.add({
            title: t('appCaps.saved', { app: cap.app }),
            type: 'success',
          }),
        onError: (err) =>
          toast.add({
            title: t('appCaps.saveFailed', { message: err.message }),
            type: 'error',
          }),
      },
    )
  }

  return (
    <li className="flex flex-wrap items-center gap-x-4 gap-y-2 py-2">
      <Link
        to="/apps/$name"
        params={{ name: cap.app }}
        className="w-40 truncate text-sm font-medium hover:underline"
      >
        {cap.app}
      </Link>
      <Progress
        value={pct}
        aria-label={t('appCaps.usage', { pct })}
        className="w-32"
      />
      <span className="w-24 text-xs tabular-nums text-muted-foreground">
        {t('appCaps.usage', { pct })}
      </span>
      <Badge variant={cap.enforced === 'none' ? 'success' : 'warning'}>
        {t(`appCaps.enforced.${cap.enforced}`)}
      </Badge>
      {cap.forecast?.will_exceed ? (
        <Badge variant="info">{t('appCaps.forecast')}</Badge>
      ) : null}
      {blocked ? (
        <Badge variant="destructive">{t('appCaps.blocked')}</Badge>
      ) : null}
      {overridden ? <Badge>{t('appCaps.overridden')}</Badge> : null}
      <form
        className="flex items-center gap-1"
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <Input
          type="number"
          min={0}
          step="any"
          inputMode="decimal"
          value={ceiling}
          aria-label={t('appCaps.ceilingLabel', { app: cap.app })}
          onChange={(e) => setCeiling(e.target.value)}
          className="h-8 w-24"
        />
        <Button
          type="submit"
          size="sm"
          variant="secondary"
          disabled={!dirty || save.isPending}
        >
          {t('appCaps.set')}
        </Button>
      </form>
      <div className="ml-auto flex items-center gap-1">
        <Button
          type="button"
          size="sm"
          variant="ghost"
          onClick={() => {
            setPreviewOpen(true)
            preview.mutate()
          }}
        >
          {t('appCaps.preview')}
        </Button>
        {blocked ? (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={() => setOverrideOpen(true)}
          >
            {t('appCaps.override')}
          </Button>
        ) : null}
      </div>
      <PausePreviewDialog
        open={previewOpen}
        onOpenChange={setPreviewOpen}
        preview={preview.data}
        pending={preview.isPending}
        error={preview.error?.message ?? null}
      />
      <OverrideDeploysDialog
        appName={cap.app}
        open={overrideOpen}
        onOpenChange={setOverrideOpen}
      />
    </li>
  )
}

/** AppCapsCard lists every app with a spend cap: set the ceiling, dry-run a pause, override a deploy block. */
export function AppCapsCard() {
  const { t } = useTranslation('costs')
  const query = useQuery(spendCapsQueryOptions())
  let body
  if (query.isPending) {
    body = <Skeleton className="h-16 w-full" />
  } else if (query.isError) {
    body = (
      <p className="text-sm text-destructive" role="alert">
        {t('appCaps.loadError', { message: query.error.message })}
      </p>
    )
  } else if (query.data.length === 0) {
    body = <p className="text-sm text-muted-foreground">{t('appCaps.empty')}</p>
  } else {
    body = (
      <ul className="divide-y divide-border">
        {query.data.map((cap) => (
          <CapRow key={cap.app} cap={cap} />
        ))}
      </ul>
    )
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('appCaps.title')}</CardTitle>
        <p className="text-sm text-muted-foreground">
          {t('appCaps.description')}
        </p>
      </CardHeader>
      <CardContent>{body}</CardContent>
    </Card>
  )
}
