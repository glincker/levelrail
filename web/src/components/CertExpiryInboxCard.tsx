import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { RefreshCwGlassIcon, SearchGlassIcon } from '@thesvg/ui-react'
import { Button, buttonVariants } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { certificatesQueryOptions } from '../queries/certificates'
import {
  useCertExpiryEmail,
  useSetCertExpiryEmail,
} from '../queries/certExpiryEmail'
import { useRenewDomainCertificate } from '../queries/domainTlsCert'
import { useNotificationChannelsOptional } from '../queries/notificationChannels'
import { isDnsReason, type CertNotice } from './certNotice'

function RenewButton({ app, domain }: { app: string; domain: string }) {
  const { t } = useTranslation('inbox')
  const renew = useRenewDomainCertificate(app, domain)
  return (
    <Button
      type="button"
      size="sm"
      disabled={renew.isPending}
      onClick={() => {
        renew.mutate(undefined, {
          onSuccess: () => {
            toast.add({
              title: t('certCard.renewRequested', { domain }),
              type: 'success',
            })
          },
          onError: (err) => {
            toast.add({
              title: t('certCard.renewError', { domain }),
              description: err.message,
              type: 'error',
            })
          },
        })
      }}
    >
      <RefreshCwGlassIcon size="1.4em" aria-hidden="true" />
      {renew.isPending ? t('certCard.renewing') : t('certCard.renewNow')}
    </Button>
  )
}

export function CertExpiryInboxCard({ notice }: { notice: CertNotice }) {
  const { t } = useTranslation('inbox')
  const certs = useQuery({ ...certificatesQueryOptions(), retry: false })
  const channels = useNotificationChannelsOptional()
  const email = useCertExpiryEmail()
  const setEmail = useSetCertExpiryEmail()
  const app =
    certs.data?.find((c) => c.domain === notice.domain)?.apps?.[0] ?? ''
  const dns = notice.dns ?? isDnsReason(notice.reason)
  const noChannel = channels.data !== undefined && channels.data.length === 0
  const emailReady = email.data?.available === true
  const emailHint =
    email.data?.reason === 'address_not_verified'
      ? t('certCard.emailNotVerified')
      : t('certCard.emailNoProvider')
  const query = new URLSearchParams({
    ...(app ? { app } : {}),
    tab: 'overview',
    fix: 'dns',
  })
  const dnsHref = `/domains/${encodeURIComponent(notice.domain)}?${query.toString()}`

  return (
    <div className="space-y-3 rounded-md border bg-muted/30 p-3 text-sm">
      <dl className="grid gap-x-4 gap-y-1 sm:grid-cols-[auto_1fr]">
        <dt className="text-muted-foreground">{t('certCard.domain')}</dt>
        <dd className="font-medium">{notice.domain}</dd>
        <dt className="text-muted-foreground">{t('certCard.issuer')}</dt>
        <dd>{notice.issuer}</dd>
        <dt className="text-muted-foreground">{t('certCard.timeLeft')}</dt>
        <dd>
          {notice.daysLeft < 0
            ? t('certCard.expired')
            : t('certCard.daysLeft', { count: notice.daysLeft })}
        </dd>
        <dt className="text-muted-foreground">{t('certCard.why')}</dt>
        <dd>{notice.reason}</dd>
      </dl>
      <div className="flex flex-wrap items-center gap-3">
        {dns || !app ? (
          <a className={buttonVariants({ size: 'sm' })} href={dnsHref}>
            <SearchGlassIcon size="1.4em" aria-hidden="true" />
            {t('certCard.checkDns')}
          </a>
        ) : (
          <RenewButton app={app} domain={notice.domain} />
        )}
        <label className="flex items-center gap-2">
          <Switch
            checked={email.data?.enabled === true}
            disabled={!emailReady || setEmail.isPending}
            onCheckedChange={(v) => {
              setEmail.mutate(v, {
                onError: () => {
                  toast.add({ title: t('certCard.emailError'), type: 'error' })
                },
              })
            }}
          />
          {t('certCard.alsoEmail')}
        </label>
        {emailReady || email.data === undefined ? null : (
          <span className="text-xs text-muted-foreground">{emailHint}</span>
        )}
      </div>
      {noChannel ? (
        <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <span>{t('certCard.noChannel')}</span>
          <a
            className={buttonVariants({ size: 'sm', variant: 'outline' })}
            href="/settings/notification-channels"
          >
            {t('certCard.connectChannel')}
          </a>
        </div>
      ) : null}
    </div>
  )
}
