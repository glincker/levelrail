import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PlugGlassIcon, RefreshCwGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Progress } from '@/components/ui/progress'
import { toast } from '@/components/ui/toast'
import { useBrand } from '../../hooks/useBrand'
import { isDockerHubHost } from '../../lib/dockerHubHost'
import {
  useDockerHubStatus,
  useRefreshDockerHubLimits,
  useReplaceDockerHubToken,
  useRetestDockerHub,
} from '../../queries/registryCredentials'
import type { RegistryCredential } from '../../types/registryCredential'
import type {
  DockerHubLimits,
  DockerHubStatus,
} from '../../types/dockerHubAccount'
import { BrandIcon } from '../BrandIcon'

const SECONDS_PER_HOUR = 3600

function formatTime(value?: string | null): string {
  return value ? new Date(value).toLocaleString() : '-'
}

type TokenState = 'valid' | 'invalid' | 'expired' | 'unknown'

function tokenState(
  credential: RegistryCredential,
  status: DockerHubStatus | undefined,
): TokenState {
  if (credential.expiry_status === 'expired') return 'expired'
  if (!status) return 'unknown'
  return status.valid ? 'valid' : 'invalid'
}

const STATE_BADGE = {
  valid: 'success',
  invalid: 'destructive',
  expired: 'destructive',
  unknown: 'muted',
} as const

function LimitsMeter({
  credentialId,
  limits,
}: {
  credentialId: string
  limits: DockerHubLimits | null
}) {
  const { t } = useTranslation('dockerHubConnection')
  const refresh = useRefreshDockerHubLimits(credentialId)
  const pct =
    limits && limits.known && limits.limit > 0
      ? Math.max(0, Math.min(100, (limits.remaining / limits.limit) * 100))
      : 0

  return (
    <div className="space-y-1" data-testid="dockerhub-limits">
      <div className="flex flex-wrap items-center gap-2 text-xs">
        <span className="font-medium">{t('limits.label')}</span>
        {limits ? (
          <Badge variant="outline">
            {limits.authenticated
              ? t('limits.authenticated')
              : t('limits.anonymous')}
          </Badge>
        ) : null}
        {limits?.low ? (
          <Badge variant="warning">
            {t('limits.low', { percent: limits.warn_percent })}
          </Badge>
        ) : null}
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={refresh.isPending}
          onClick={() => refresh.mutate()}
        >
          <RefreshCwGlassIcon aria-hidden="true" className="size-5" />
          {refresh.isPending ? t('limits.refreshing') : t('limits.refresh')}
        </Button>
      </div>
      {limits && limits.known ? (
        <>
          <Progress
            value={pct}
            aria-label={t('limits.meter')}
            className="gap-0"
          />
          <p className="text-xs text-muted-foreground">
            {t('limits.remaining', {
              remaining: limits.remaining.toLocaleString(),
              limit: limits.limit.toLocaleString(),
            })}
            {' · '}
            {t('limits.window', {
              hours: Math.round(limits.window_seconds / SECONDS_PER_HOUR),
            })}
            {' · '}
            {limits.source === 'ip'
              ? t('limits.sourceIp')
              : t('limits.sourceAccount')}
          </p>
        </>
      ) : (
        <p className="text-xs text-muted-foreground">
          {limits ? t('limits.unknown') : t('limits.unavailable')}
        </p>
      )}
      {refresh.error ? (
        <p role="alert" className="text-xs text-destructive">
          {t('limits.refreshFailed', { message: refresh.error.message })}
        </p>
      ) : null}
    </div>
  )
}

function ReplaceTokenForm({
  credential,
  onDone,
}: {
  credential: RegistryCredential
  onDone: () => void
}) {
  const { t } = useTranslation('dockerHubConnection')
  const [token, setToken] = useState('')
  const replace = useReplaceDockerHubToken()
  const inputId = `dockerhub-token-${credential.id}`

  return (
    <form
      className="flex flex-wrap items-end gap-2"
      onSubmit={(e) => {
        e.preventDefault()
        replace.mutate(
          {
            id: credential.id,
            name: credential.name,
            registry_host: credential.registry_host,
            username: credential.username,
            password: token,
          },
          {
            onSuccess: () => {
              toast.add({
                title: t('actions.replaced', { name: credential.name }),
                type: 'success',
              })
              onDone()
            },
          },
        )
      }}
    >
      <label htmlFor={inputId} className="min-w-48 flex-1 space-y-1 text-xs">
        <span>{t('actions.tokenLabel')}</span>
        <Input
          id={inputId}
          type="password"
          autoComplete="new-password"
          value={token}
          onChange={(e) => setToken(e.target.value)}
        />
      </label>
      <Button
        type="submit"
        size="sm"
        disabled={replace.isPending || token.trim() === ''}
      >
        {replace.isPending ? t('actions.saving') : t('actions.save')}
      </Button>
      <Button type="button" size="sm" variant="outline" onClick={onDone}>
        {t('actions.cancel')}
      </Button>
      <p className="basis-full text-xs text-muted-foreground">
        {t('actions.tokenHelp')}
      </p>
      {replace.error ? (
        <p role="alert" className="basis-full text-xs text-destructive">
          {replace.error.message}
        </p>
      ) : null}
    </form>
  )
}

export function DockerHubCredentialRow({
  credential,
}: {
  credential: RegistryCredential
}) {
  const { t } = useTranslation('dockerHubConnection')
  const { BinaryName } = useBrand()
  const statusQuery = useDockerHubStatus(credential.id)
  const retest = useRetestDockerHub(credential.id)
  const [replacing, setReplacing] = useState(false)
  const status = statusQuery.data
  const state = tokenState(credential, status)
  const problem = state === 'invalid' || state === 'expired'
  const failed = statusQuery.isError

  function runTest() {
    retest.mutate(undefined, {
      onSuccess: (data) => {
        toast.add({
          title: data.valid
            ? t('actions.testOk', { name: credential.name })
            : t('actions.testRejected', { name: credential.name }),
          type: data.valid ? 'success' : 'error',
        })
      },
      onError: (error) => {
        toast.add({
          title: t('actions.testFailed', { name: credential.name }),
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  const scopes = status?.scopes ?? []

  return (
    <div className="space-y-3 border-t pt-3 first:border-t-0 first:pt-0">
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <span className="font-medium">{credential.name}</span>
        <span className="text-xs text-muted-foreground">
          {credential.username}
        </span>
        <Badge variant={STATE_BADGE[state]} aria-label={t('status.label')}>
          {statusQuery.isLoading ? t('status.checking') : t(`status.${state}`)}
        </Badge>
        {status ? (
          <span className="text-xs text-muted-foreground">
            {t('status.checkedAt', { when: formatTime(status.checked_at) })}
          </span>
        ) : null}
        <div className="ml-auto flex flex-wrap gap-2">
          <Button
            type="button"
            size="sm"
            variant={problem || failed ? 'default' : 'outline'}
            disabled={retest.isPending}
            onClick={runTest}
          >
            <PlugGlassIcon aria-hidden="true" className="size-5" />
            {retest.isPending
              ? t('actions.testing')
              : problem || failed
                ? t('actions.retest')
                : t('actions.test')}
          </Button>
          {problem ? (
            <Button
              type="button"
              size="sm"
              onClick={() => setReplacing(true)}
              disabled={replacing}
            >
              {t('actions.replaceToken')}
            </Button>
          ) : null}
        </div>
      </div>

      {failed ? (
        <p role="alert" className="text-xs text-destructive">
          {t('status.loadFailed', { message: statusQuery.error.message })}
        </p>
      ) : null}
      {status && !status.valid && status.message ? (
        <p role="alert" className="text-xs text-destructive">
          {status.message}
        </p>
      ) : null}
      {status?.valid ? (
        <p className="text-xs text-muted-foreground">
          {t('status.kind', { kind: status.kind })}
          {' · '}
          {scopes.length > 0
            ? t('status.scopes', { scopes: scopes.join(', ') })
            : t('status.noScopes')}
          {status.private_repos_visible ? ` · ${t('status.privateRepos')}` : ''}
          {status.expiry_known
            ? ` · ${t('status.expiresOn', { when: formatTime(status.expires_at) })}`
            : ''}
        </p>
      ) : null}

      {replacing ? (
        <ReplaceTokenForm
          credential={credential}
          onDone={() => setReplacing(false)}
        />
      ) : null}

      {statusQuery.isLoading ? (
        <div
          role="status"
          aria-label={t('status.checking')}
          className="h-8 animate-pulse rounded-md bg-muted motion-reduce:animate-none"
        />
      ) : status ? (
        <LimitsMeter credentialId={credential.id} limits={status.limits} />
      ) : null}

      {problem || failed ? (
        <details className="text-xs text-muted-foreground">
          <summary className="cursor-pointer">{t('cli.summary')}</summary>
          <p className="mt-1">{t('cli.hint')}</p>
          <code className="mt-1 block rounded bg-muted px-2 py-1 font-mono">
            {`${BinaryName} registry-credentials status ${credential.id} --refresh`}
          </code>
        </details>
      ) : null}
    </div>
  )
}

export function DockerHubConnectionCard({
  credentials,
}: {
  credentials: RegistryCredential[] | null
}) {
  const { t } = useTranslation('dockerHubConnection')
  const hub = (credentials ?? []).filter((c) =>
    isDockerHubHost(c.registry_host),
  )
  if (hub.length === 0) return null
  return (
    <section className="space-y-3" aria-labelledby="dockerhub-connection">
      <div>
        <h2 id="dockerhub-connection" className="text-sm font-medium">
          {t('title')}
        </h2>
        <p className="text-xs text-muted-foreground">{t('description')}</p>
      </div>
      <Card>
        <CardContent className="space-y-3">
          <div className="flex items-center gap-2">
            <BrandIcon name="docker" aria-hidden="true" className="size-6" />
          </div>
          {hub.map((c) => (
            <DockerHubCredentialRow key={c.id} credential={c} />
          ))}
        </CardContent>
      </Card>
    </section>
  )
}

export default DockerHubConnectionCard
