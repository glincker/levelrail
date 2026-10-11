import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { ShieldWarningIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import {
  useDockerGuard,
  useUpdateDockerGuard,
  type DockerGuardMode,
  type DockerGuardStatus,
} from '../queries/dockerGuard'
import { SettingsFormAlerts } from './SettingsCard'

const MODES: DockerGuardMode[] = ['enforce', 'audit', 'off']

export function DockerGuardCard() {
  const { data: status } = useDockerGuard()
  if (!status) return null
  return <DockerGuardForm status={status} />
}

function DockerGuardForm({ status }: { status: DockerGuardStatus }) {
  const { t } = useTranslation('settings')
  const update = useUpdateDockerGuard()
  const [mode, setMode] = useState<DockerGuardMode>(status.mode)
  const pinned = status.source === 'env'
  const days = Math.round(status.window_seconds / 86400)

  function save(next: DockerGuardMode) {
    update.mutate(next, {
      onSuccess: (s) => {
        setMode(s.mode)
        toast.add({ title: t('dockerGuard.saved'), type: 'success' })
      },
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ShieldWarningIcon className="size-4" aria-hidden="true" />
          {t('dockerGuard.title')}
          <Badge
            variant={status.effective === 'enforce' ? 'default' : 'outline'}
          >
            {t(`dockerGuard.modes.${status.effective}.label`)}
          </Badge>
        </CardTitle>
        <CardDescription>{t('dockerGuard.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        {!status.configured ? (
          <p className="text-sm text-muted-foreground">
            {t('dockerGuard.notConfigured')}
          </p>
        ) : null}
        {status.restart_required ? (
          <Alert>
            <AlertDescription>
              {t('dockerGuard.restartRequired')}
            </AlertDescription>
          </Alert>
        ) : null}
        {status.ready_to_enforce ? (
          <Alert>
            <AlertDescription>
              {t('dockerGuard.readyToEnforce', { days })}
            </AlertDescription>
          </Alert>
        ) : null}

        <div
          role="radiogroup"
          aria-label={t('dockerGuard.modeLabel')}
          className="grid gap-2"
        >
          {MODES.map((m) => {
            const selected = mode === m
            return (
              <label
                key={m}
                className={cn(
                  'flex cursor-pointer items-start gap-3 rounded-lg border p-3 text-sm',
                  selected ? 'border-primary bg-muted' : 'border-border',
                  pinned && 'cursor-not-allowed opacity-60',
                )}
              >
                <input
                  type="radio"
                  name="docker-guard-mode"
                  value={m}
                  checked={selected}
                  disabled={pinned || !status.configured}
                  onChange={() => setMode(m)}
                  className="mt-1"
                />
                <span className="space-y-0.5">
                  <span className="block font-medium">
                    {t(`dockerGuard.modes.${m}.label`)}
                  </span>
                  <span className="block text-muted-foreground">
                    {t(`dockerGuard.modes.${m}.description`)}
                  </span>
                </span>
              </label>
            )
          })}
        </div>
        {pinned ? (
          <p className="text-sm text-muted-foreground">
            {t('dockerGuard.pinned')}
          </p>
        ) : null}

        <div className="space-y-2 text-sm">
          <p className="font-medium">
            {t('dockerGuard.windowTitle', { days })}
          </p>
          {status.window.length === 0 ? (
            <p className="text-muted-foreground">
              {t('dockerGuard.windowEmpty')}
            </p>
          ) : (
            <ul className="space-y-1">
              {status.window.map((r) => (
                <li
                  key={r.rule}
                  className="flex flex-wrap items-baseline gap-2"
                >
                  <code className="font-mono text-xs">{r.rule}</code>
                  <span className="text-muted-foreground">
                    {t('dockerGuard.ruleCounts', {
                      wouldDeny: r.would_deny,
                      denied: r.denied,
                    })}
                  </span>
                  <span className="truncate font-mono text-xs text-muted-foreground">
                    {r.last_path}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>

        <SettingsFormAlerts
          alerts={[
            update.isError && { key: 'update', message: update.error.message },
          ]}
        />

        <div className="flex flex-wrap items-center gap-3">
          <Button
            onClick={() => save(mode)}
            disabled={pinned || mode === status.mode || update.isPending}
          >
            {update.isPending ? t('dockerGuard.saving') : t('dockerGuard.save')}
          </Button>
          <Link
            to="/settings/audit-log"
            className="text-sm underline underline-offset-4"
          >
            {t('dockerGuard.viewAudit')}
          </Link>
        </div>
      </CardContent>
    </Card>
  )
}
