import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { LockKeyIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { useDomainPorts, useRestrictPort } from '../../queries/domainPolicies'
import type { PortStream } from '../../queries/domainPolicyTypes'
import { FieldMessage, Section } from './PolicyShared'
import { splitList } from './policyUtils'

function RestrictForm({
  app,
  domain,
  stream,
}: {
  app: string
  domain: string
  stream: PortStream
}) {
  const { t } = useTranslation('domainPolicies')
  const restrict = useRestrictPort(app, domain)
  const [sources, setSources] = useState(stream.allowed_sources.join(', '))
  const id = `restrict-${stream.id}`
  return (
    <form
      className="flex flex-wrap items-end gap-2"
      onSubmit={(e) => {
        e.preventDefault()
        restrict.mutate({ port: stream.host_port, sources: splitList(sources) })
      }}
    >
      <label htmlFor={id} className="flex flex-1 flex-col gap-1 text-xs">
        <span className="text-muted-foreground">{t('ports.onlyFrom')}</span>
        <Input
          id={id}
          value={sources}
          placeholder="203.0.113.7, 198.51.100.0/24"
          onChange={(e) => setSources(e.target.value)}
        />
      </label>
      <Button
        type="submit"
        size="sm"
        variant="outline"
        disabled={restrict.isPending}
      >
        <LockKeyIcon />
        {t('ports.restrict')}
      </Button>
      {!stream.open_to_all ? (
        <Button
          type="button"
          size="sm"
          variant="ghost"
          disabled={restrict.isPending}
          onClick={() =>
            restrict.mutate({ port: stream.host_port, sources: null })
          }
        >
          {t('ports.openToAll')}
        </Button>
      ) : null}
      <FieldMessage message={restrict.error?.message} />
    </form>
  )
}

export function PortsTab({ app, domain }: { app: string; domain: string }) {
  const { t } = useTranslation('domainPolicies')
  const { data, error, isLoading } = useDomainPorts(app, domain)
  if (isLoading) return <Skeleton className="h-40 w-full" />
  if (error || !data) {
    return (
      <Alert variant="destructive">
        <AlertDescription>
          {error?.message ?? t('page.loadFailed')}
        </AlertDescription>
      </Alert>
    )
  }
  return (
    <Section
      title={t('ports.title')}
      description={t('ports.help', { port: data.public_https_port })}
    >
      {data.detection_note ? (
        <p className="text-xs text-muted-foreground">{data.detection_note}</p>
      ) : null}
      {data.streams.length === 0 ? (
        <div className="space-y-2">
          <p className="text-sm text-muted-foreground">{t('ports.empty')}</p>
          <Link
            to="/apps/$name/streams"
            params={{ name: app }}
            className="text-sm text-primary underline-offset-4 hover:underline"
          >
            {t('ports.addStream')}
          </Link>
        </div>
      ) : (
        <ul className="space-y-3">
          {data.streams.map((s) => (
            <li
              key={s.id}
              className="space-y-2 rounded-md border border-border p-3"
            >
              <div className="flex flex-wrap items-center gap-2 text-sm">
                <span className="font-mono">
                  {s.host_port}/{s.protocol}
                </span>
                <span className="text-muted-foreground">
                  {t('ports.to', { target: s.target })}
                </span>
                {s.open_to_all ? (
                  <Badge variant="warning">{t('ports.public')}</Badge>
                ) : (
                  <Badge variant="success">
                    {t('ports.restricted', { count: s.allowed_sources.length })}
                  </Badge>
                )}
              </div>
              {s.allowed_sources.length > 0 ? (
                <p className="text-xs text-muted-foreground">
                  {t('ports.allowed', {
                    sources: s.allowed_sources.join(', '),
                  })}
                </p>
              ) : null}
              {s.conflicts.map((c) => (
                <p
                  key={c}
                  role="alert"
                  className="flex items-center gap-1 text-xs text-destructive"
                >
                  <WarningIcon className="size-3" aria-hidden="true" />
                  {c}
                </p>
              ))}
              <RestrictForm app={app} domain={domain} stream={s} />
            </li>
          ))}
        </ul>
      )}
      <Link
        to="/apps/$name/streams"
        params={{ name: app }}
        className="text-xs text-primary underline-offset-4 hover:underline"
      >
        {t('ports.manageStreams')}
      </Link>
      <p className="text-xs text-muted-foreground">{t('ports.firewallNote')}</p>
    </Section>
  )
}
