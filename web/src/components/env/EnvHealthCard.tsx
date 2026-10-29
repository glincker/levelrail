import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  CircleCheckGlassIcon,
  RestoreGlassIcon,
  RotateCwGlassIcon,
  TriangleAlertGlassIcon,
  WrenchGlassIcon,
} from '@thesvg/ui-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { toast } from '@/components/ui/toast'
import {
  useEnvCheck,
  useEnvHistory,
  useFixEnvFinding,
  type EnvFinding,
} from '../../queries/envHealth'

type Translate = TFunction<'envEditor'>

function kindText(t: Translate, f: EnvFinding): string {
  const detail = f.detail ?? ''
  switch (f.kind) {
    case 'empty_value':
      return t('health.kinds.empty_value')
    case 'shadowed':
      return t('health.kinds.shadowed')
    case 'case_duplicate':
      return t('health.kinds.case_duplicate', { detail })
    case 'missing_secret':
      return t('health.kinds.missing_secret')
    case 'dangling_database':
      return t('health.kinds.dangling_database', { detail })
    default:
      return f.kind
  }
}

function fixText(t: Translate, action: string): string {
  switch (action) {
    case 'remove_key':
      return t('health.fixes.remove_key')
    case 'remove_database_ref':
      return t('health.fixes.remove_database_ref')
    case 'set_secret':
      return t('health.fixes.set_secret')
    case 'rename':
      return t('health.fixes.rename')
    default:
      return action
  }
}

function findingKey(f: EnvFinding): string {
  return `${f.kind}:${f.key}`
}

/** EnvHealthCard lists variable problems with a Fix button each, plus the key-level change history. */
export function EnvHealthCard({ appName }: { appName: string }) {
  const { t } = useTranslation('envEditor')
  const check = useEnvCheck(appName)
  const history = useEnvHistory(appName)
  const fix = useFixEnvFinding(appName)

  const onFix = (f: EnvFinding) => {
    fix.mutate(f, {
      onSuccess: () => {
        toast.add({ title: t('health.fixed', { key: f.key }), type: 'success' })
      },
      onError: (err) => {
        toast.add({ title: err.message, type: 'error' })
      },
    })
  }

  const findings = check.data ?? []

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center justify-between gap-2">
          <span className="flex items-center gap-2">
            <WrenchGlassIcon className="size-5" aria-hidden />
            {t('health.title')}
          </span>
          <Button
            variant="outline"
            size="sm"
            onClick={() => void check.refetch()}
            disabled={check.isFetching}
          >
            <RotateCwGlassIcon size="1.4em" aria-hidden />
            {t('health.recheck')}
          </Button>
        </CardTitle>
        <CardDescription>{t('health.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {check.isError ? (
          <p className="text-sm text-destructive">{t('health.loadFailed')}</p>
        ) : findings.length === 0 ? (
          <p className="flex items-center gap-2 text-sm text-muted-foreground">
            <CircleCheckGlassIcon
              className="size-5 text-tone-success"
              aria-hidden
            />
            {check.isLoading ? t('health.loading') : t('health.clean')}
          </p>
        ) : (
          <ul className="divide-y">
            {findings.map((f) => (
              <li
                key={findingKey(f)}
                className="flex flex-wrap items-center justify-between gap-2 py-2"
              >
                <div className="min-w-0 space-y-0.5">
                  <p className="flex items-center gap-2 text-sm font-medium">
                    <TriangleAlertGlassIcon
                      className={
                        f.severity === 'error'
                          ? 'size-6 text-destructive'
                          : 'size-6 text-muted-foreground'
                      }
                      aria-hidden
                    />
                    <span className="font-mono">{f.key}</span>
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {kindText(t, f)}
                  </p>
                </div>
                {f.fix.type === 'auto' ? (
                  <Button
                    size="sm"
                    onClick={() => onFix(f)}
                    disabled={fix.isPending}
                  >
                    {fixText(t, f.fix.action)}
                  </Button>
                ) : (
                  <span className="text-xs text-muted-foreground">
                    {fixText(t, f.fix.action)}
                  </span>
                )}
              </li>
            ))}
          </ul>
        )}
        <div className="space-y-2">
          <h3 className="flex items-center gap-2 text-sm font-medium">
            <RestoreGlassIcon className="size-5" aria-hidden />
            {t('health.historyTitle')}
          </h3>
          {history.data && history.data.length > 0 ? (
            <ul className="space-y-1.5">
              {history.data.map((c) => (
                <li
                  key={c.id}
                  className="flex flex-wrap items-center gap-2 text-sm"
                >
                  <Badge variant="outline">
                    {t(`health.actions.${c.action}`)}
                  </Badge>
                  <span className="font-mono">{c.key}</span>
                  <span className="text-muted-foreground">
                    {c.actor !== ''
                      ? t('health.byActor', { actor: c.actor })
                      : null}{' '}
                    {new Date(c.at).toLocaleString()}
                  </span>
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-sm text-muted-foreground">
              {t('health.historyEmpty')}
            </p>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
