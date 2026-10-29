import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckGlassIcon, CopyGlassIcon, PlugGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ConfirmAction } from '@/components/ui/confirm-action'
import { EmptyState } from '@/components/ui/empty-state'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import { ApiError } from '../../lib/apiError'
import {
  useAgentDetail,
  useAgentSessions,
  useCreateAgentSession,
  useDiscoverAgent,
  useRevokeAgent,
  type AgentContainer,
  type AgentCreated,
  type AgentSession,
} from '../../queries/migrateAgent'
import { MigrateAgentVerify } from './MigrateAgentVerify'

function stateVariant(s: AgentSession['state']) {
  if (s === 'connected') return 'success'
  if (s === 'pending') return 'info'
  if (s === 'lost') return 'warning'
  return 'muted'
}

function InstallCommand({ created }: { created: AgentCreated }) {
  const { t } = useTranslation('migrate')
  const { copied, copy } = useCopyToClipboard()
  return (
    <Banner variant="info" title={t('agent.installTitle')}>
      <div className="space-y-2">
        <p>{t('agent.installHint', { hours: created.session_max_hours })}</p>
        {created.command ? (
          <div className="flex items-start gap-2">
            <code className="block flex-1 break-all rounded-md bg-muted p-2 text-xs">
              {created.command}
            </code>
            <Button
              type="button"
              variant="outline"
              size="icon-sm"
              aria-label={copied ? t('agent.copied') : t('agent.copy')}
              onClick={() => copy(created.command ?? '')}
            >
              {copied ? (
                <CheckGlassIcon size="1.4em" aria-hidden="true" />
              ) : (
                <CopyGlassIcon size="1.4em" aria-hidden="true" />
              )}
            </Button>
          </div>
        ) : (
          <p>{t('agent.noAddress')}</p>
        )}
        <p className="text-xs text-muted-foreground">
          {t('agent.fingerprint', { fp: created.ca_fingerprint })}
        </p>
      </div>
    </Banner>
  )
}

function ContainerRow({ c }: { c: AgentContainer }) {
  const { t } = useTranslation('migrate')
  return (
    <li className="flex flex-wrap items-center gap-2 py-2 text-sm">
      <span className="font-medium">{c.name}</span>
      <span className="text-muted-foreground">{c.image}</span>
      <Badge variant={c.state === 'running' ? 'success' : 'muted'}>
        {c.state}
      </Badge>
      {c.database_engine ? (
        <Badge variant="accent">
          {t('agent.database', {
            engine: c.database_engine,
            version: c.database_version ?? '',
          })}
        </Badge>
      ) : null}
      {c.unlisted ? (
        <Badge variant="warning">{t('agent.unlisted')}</Badge>
      ) : null}
    </li>
  )
}

function SessionCard({ session }: { session: AgentSession }) {
  const { t } = useTranslation('migrate')
  const detail = useAgentDetail(session.id)
  const discover = useDiscoverAgent()
  const revoke = useRevokeAgent()
  const active = session.state === 'connected'
  const live = ['pending', 'connected', 'lost'].includes(session.state)
  const inv = detail.data?.inventory
  const unlisted = inv?.containers.filter((c) => c.unlisted).length ?? 0

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-center gap-2">
          <CardTitle>{session.label || session.id}</CardTitle>
          <Badge variant={stateVariant(session.state)}>
            {t(`agent.state.${session.state}`)}
          </Badge>
          {session.session_expires_at ? (
            <span className="text-sm text-muted-foreground">
              {t('agent.endsBy', {
                at: new Date(session.session_expires_at).toLocaleString(),
              })}
            </span>
          ) : null}
          {live ? (
            <ConfirmAction
              trigger={(open) => (
                <Button
                  size="sm"
                  variant="destructive"
                  className="ml-auto"
                  onClick={open}
                >
                  {t('agent.remove')}
                </Button>
              )}
              title={t('agent.removeTitle')}
              consequences={[
                t('agent.removeConsequence1'),
                t('agent.removeConsequence2'),
              ]}
              confirmLabel={t('agent.removeConfirm')}
              pending={revoke.isPending}
              onConfirm={(close) =>
                revoke.mutate(session.id, {
                  onSuccess: close,
                  onError: (e) =>
                    toast.add({ title: e.message, type: 'error' }),
                })
              }
            />
          ) : null}
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {session.state === 'pending' ? (
          <p className="text-sm text-muted-foreground">{t('agent.waiting')}</p>
        ) : null}
        {active ? (
          <div className="flex items-center gap-2">
            <Button
              size="sm"
              variant="outline"
              disabled={discover.isPending}
              onClick={() =>
                discover.mutate(session.id, {
                  onError: (e) =>
                    toast.add({ title: e.message, type: 'error' }),
                })
              }
            >
              {discover.isPending
                ? t('agent.discovering')
                : t('agent.discover')}
            </Button>
          </div>
        ) : null}
        {detail.error ? (
          <p className="text-sm text-muted-foreground">
            {detail.error instanceof ApiError && detail.error.status === 403
              ? t('agent.detailForbidden')
              : detail.error.message}
          </p>
        ) : null}
        {inv && inv.containers.length > 0 ? (
          <div>
            <h3 className="text-sm font-medium">
              {t('agent.found', {
                containers: inv.containers.length,
                volumes: inv.volumes.length,
              })}
            </h3>
            {unlisted > 0 ? (
              <p className="text-sm text-muted-foreground">
                {t('agent.unlistedHint', { count: unlisted })}
              </p>
            ) : null}
            <ul className="divide-y">
              {inv.containers.map((c) => (
                <ContainerRow key={c.id} c={c} />
              ))}
            </ul>
          </div>
        ) : null}
        {active ? <MigrateAgentVerify sessionId={session.id} /> : null}
      </CardContent>
    </Card>
  )
}

export function MigrateAgentPanel() {
  const { t } = useTranslation('migrate')
  const sessions = useAgentSessions()
  const create = useCreateAgentSession()
  const [label, setLabel] = useState('')
  const [created, setCreated] = useState<AgentCreated | null>(null)

  function start() {
    create.mutate(
      { label: label.trim() },
      {
        onSuccess: (c) => setCreated(c),
        onError: (e) => toast.add({ title: e.message, type: 'error' }),
      },
    )
  }

  return (
    <div className="space-y-4">
      <Banner variant="info" title={t('agent.safetyTitle')}>
        {t('agent.safety')}
      </Banner>
      <Card>
        <CardHeader>
          <CardTitle>{t('agent.startTitle')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <p className="text-sm text-muted-foreground">
            {t('agent.startHint')}
          </p>
          <div className="flex flex-wrap items-end gap-2">
            <div className="space-y-1">
              <Label htmlFor="agent-label">{t('agent.label')}</Label>
              <Input
                id="agent-label"
                value={label}
                maxLength={80}
                placeholder={t('agent.labelPlaceholder')}
                onChange={(e) => setLabel(e.target.value)}
              />
            </div>
            <Button disabled={create.isPending} onClick={start}>
              {create.isPending ? t('agent.creating') : t('agent.create')}
            </Button>
          </div>
          {created ? <InstallCommand created={created} /> : null}
        </CardContent>
      </Card>
      {sessions.isPending ? (
        <Skeleton className="h-32 w-full rounded-lg" />
      ) : sessions.isError ? (
        <Banner variant="critical" title={t('agent.loadError')}>
          {sessions.error.message}
        </Banner>
      ) : !sessions.data.some((s) =>
          ['pending', 'connected', 'lost'].includes(s.state),
        ) ? (
        <EmptyState
          icon={<PlugGlassIcon className="size-6" />}
          title={t('agent.empty')}
          description={t('agent.emptyHint')}
        />
      ) : (
        sessions.data
          .filter((s) => ['pending', 'connected', 'lost'].includes(s.state))
          .map((s) => <SessionCard key={s.id} session={s} />)
      )}
    </div>
  )
}
