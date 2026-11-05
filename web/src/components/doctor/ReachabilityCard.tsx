import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  CheckGlassIcon,
  CircleCheckGlassIcon,
  CircleXGlassIcon,
  CopyGlassIcon,
  RefreshCwGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { ApiError } from '../../lib/apiError'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import {
  useDoctorReachability,
  type ReachabilityExternal,
  type ReachabilityPort,
} from '../../queries/doctorReachability'

function portLabel(t: TFunction<'domains'>, name: string): string {
  if (name === 'dashboard') return t('reachability.ports.dashboard')
  if (name === 'http') return t('reachability.ports.http')
  if (name === 'https') return t('reachability.ports.https')
  return name
}

function externalLabel(
  t: TFunction<'domains'>,
  value: ReachabilityExternal,
): string {
  switch (value) {
    case 'reachable':
      return t('reachability.external.reachable')
    case 'unreachable':
      return t('reachability.external.unreachable')
    case 'unknown':
      return t('reachability.external.unknown')
    default:
      return t('reachability.external.not_checked')
  }
}

function CopyAdvice({ text }: { text: string }) {
  const { t } = useTranslation('domains')
  const { copied, copy } = useCopyToClipboard()
  return (
    <Button
      type="button"
      size="xs"
      variant="outline"
      onClick={() => {
        copy(text)
      }}
    >
      {copied ? (
        <CheckGlassIcon size="1.4em" />
      ) : (
        <CopyGlassIcon size="1.4em" />
      )}
      {copied ? t('reachability.copied') : t('reachability.copy')}
    </Button>
  )
}

function ExternalBadge({ value }: { value: ReachabilityExternal }) {
  const { t } = useTranslation('domains')
  const variant =
    value === 'reachable'
      ? 'success'
      : value === 'unreachable'
        ? 'warning'
        : 'outline'
  return <Badge variant={variant}>{externalLabel(t, value)}</Badge>
}

function PortRow({ port }: { port: ReachabilityPort }) {
  const { t } = useTranslation('domains')
  return (
    <li className="space-y-1 py-2">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm">
        <span className="w-24 font-medium">{portLabel(t, port.name)}</span>
        <span className="w-16 font-mono text-muted-foreground">
          {port.port}
        </span>
        <Badge variant={port.listening ? 'success' : 'warning'}>
          {port.listening
            ? t('reachability.listening.yes')
            : t('reachability.listening.no')}
        </Badge>
        <ExternalBadge value={port.external} />
      </div>
      {port.advice ? (
        <div className="flex flex-wrap items-start gap-2 text-sm">
          <p className="min-w-0 flex-1 text-muted-foreground">{port.advice}</p>
          <CopyAdvice text={port.advice} />
        </div>
      ) : null}
    </li>
  )
}

// Which of the server's ports are open, from here and (optionally) from the
// internet, with the exact next step for each blocked one.
export function ReachabilityCard() {
  const { t } = useTranslation('domains')
  const { data, error, isPending, isFetching, refetch } =
    useDoctorReachability()
  const ports = data?.ports ?? []
  const forbidden = error instanceof ApiError && error.status === 403

  return (
    <Card>
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
        <CardTitle>{t('reachability.title')}</CardTitle>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={isFetching}
          onClick={() => {
            void refetch()
          }}
        >
          <RefreshCwGlassIcon
            size="1.4em"
            className={
              isFetching ? 'animate-spin motion-reduce:animate-none' : ''
            }
          />
          {t('reachability.recheck')}
        </Button>
      </CardHeader>
      <CardContent className="space-y-3">
        {isPending ? (
          <Skeleton
            className="h-24 w-full"
            aria-label={t('reachability.loading')}
          />
        ) : forbidden ? (
          <p className="text-sm text-muted-foreground">
            {t('reachability.forbidden')}
          </p>
        ) : error ? (
          <Alert variant="destructive">
            <CircleXGlassIcon className="text-tone-danger" size="1.4em" />
            <AlertTitle>{t('reachability.errorTitle')}</AlertTitle>
            <AlertDescription>{t('reachability.errorBody')}</AlertDescription>
          </Alert>
        ) : (
          <>
            <p className="text-sm text-foreground" aria-live="polite">
              {data.message}
            </p>
            <ul
              className="divide-y divide-border"
              aria-label={t('reachability.title')}
            >
              {ports.map((p) => (
                <PortRow key={p.name} port={p} />
              ))}
            </ul>
            {data.dialback_error ? (
              <p className="text-sm text-tone-warning">
                {t('reachability.dialbackError', {
                  error: data.dialback_error,
                })}
              </p>
            ) : null}
            {data.firewall ? (
              <p className="text-sm text-muted-foreground">
                {t('reachability.firewall', { state: data.firewall })}
              </p>
            ) : null}
            <p className="flex flex-wrap items-center gap-x-2 text-sm text-muted-foreground">
              <CircleCheckGlassIcon aria-hidden="true" className="size-4" />
              {data.dialback_enabled
                ? t('reachability.dialbackOn')
                : t('reachability.dialbackOff')}
              <Link
                to="/settings/security-runtime"
                className="font-medium text-foreground underline underline-offset-4 hover:text-primary"
              >
                {t('reachability.settingLink')}
              </Link>
            </p>
          </>
        )}
      </CardContent>
    </Card>
  )
}
