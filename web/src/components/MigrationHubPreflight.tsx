import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  useApplyHubSession,
  type HubItem,
  type HubSeverity,
  type HubSession,
} from '../queries/migrationHub'
import { formatBytes, formatSeconds } from './migrationHubFormat'

const severityVariant: Record<
  HubSeverity,
  'success' | 'warning' | 'destructive'
> = {
  ok: 'success',
  warn: 'warning',
  block: 'destructive',
}

function ItemChecks({ item }: { item: HubItem }) {
  const { t } = useTranslation('migration')
  const label: Record<HubSeverity, string> = {
    ok: t('hub.preflight.ok'),
    warn: t('hub.preflight.warning'),
    block: t('hub.preflight.blocked'),
  }
  return (
    <li className="rounded-md border p-3 text-sm">
      <div className="flex flex-wrap items-baseline gap-2">
        <span className="font-medium">{item.source_db}</span>
        <span className="text-xs text-muted-foreground">
          {item.target_name} ({item.target_version})
        </span>
        <span className="ml-auto text-xs text-muted-foreground tabular-nums">
          {formatBytes(item.size_bytes)}
        </span>
      </div>
      <ul className="mt-2 space-y-1.5">
        {item.preflight.checks.map((c) => (
          <li key={c.id} className="flex flex-wrap items-start gap-2">
            <Badge variant={severityVariant[c.severity]}>
              {label[c.severity]}
            </Badge>
            <span className="min-w-0 flex-1">
              {c.message}
              {c.next_action ? (
                <span className="block text-muted-foreground">
                  {c.next_action}
                </span>
              ) : null}
            </span>
          </li>
        ))}
      </ul>
    </li>
  )
}

export function MigrationHubPreflight({
  session,
  onBack,
  onStarted,
}: {
  session: HubSession
  onBack: () => void
  onStarted: () => void
}) {
  const { t } = useTranslation('migration')
  const apply = useApplyHubSession()
  const [password, setPassword] = useState('')
  const sum = session.summary
  const selected = session.items.filter((i) => i.selected)
  const blocked = sum.blocked > 0
  const needPassword = !session.password_held
  const canStart =
    sum.selected > 0 &&
    !blocked &&
    !session.running &&
    (!needPassword || password !== '')

  return (
    <div className="space-y-4">
      <div className="space-y-1">
        <h3 className="text-base font-semibold">{t('hub.preflight.title')}</h3>
        <p className="text-sm text-muted-foreground">
          {t('hub.preflight.description')}
        </p>
        <p className="text-sm">
          {t('hub.preflight.required', {
            required: formatBytes(sum.required_bytes),
            free: formatBytes(session.free_bytes),
            time: formatSeconds(sum.estimate_seconds),
          })}
        </p>
      </div>
      {session.helper_network ? (
        <p className="text-sm text-muted-foreground">
          {t('hub.preflight.reach', { network: session.helper_network })}
        </p>
      ) : null}
      <ul className="space-y-2">
        {selected.map((i) => (
          <ItemChecks key={i.source_db} item={i} />
        ))}
      </ul>
      {blocked ? (
        <Alert variant="destructive">
          <AlertDescription>{t('hub.preflight.fixBlocked')}</AlertDescription>
        </Alert>
      ) : null}
      {needPassword ? (
        <div className="max-w-sm space-y-1.5">
          <Label htmlFor="hub-reenter">
            {t('hub.preflight.passwordTitle')}
          </Label>
          <Input
            id="hub-reenter"
            type="password"
            autoComplete="off"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
          <p className="text-xs text-muted-foreground">
            {t('hub.preflight.passwordHelp')}
          </p>
        </div>
      ) : null}
      {apply.isError ? (
        <Alert variant="destructive">
          <AlertTitle>{t('hub.preflight.startError')}</AlertTitle>
          <AlertDescription>{apply.error.message}</AlertDescription>
        </Alert>
      ) : null}
      <div className="flex gap-2">
        <Button
          disabled={!canStart || apply.isPending}
          onClick={() =>
            apply.mutate(
              { id: session.id, password },
              {
                onSuccess: () => {
                  setPassword('')
                  onStarted()
                },
              },
            )
          }
        >
          {apply.isPending
            ? t('hub.preflight.starting')
            : t('hub.preflight.start', { count: sum.selected })}
        </Button>
        <Button variant="outline" onClick={onBack}>
          {t('hub.preflight.back')}
        </Button>
      </div>
    </div>
  )
}
