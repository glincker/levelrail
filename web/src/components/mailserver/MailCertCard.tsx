import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { StatusPill } from '@/components/kit'
import { toast } from '@/components/ui/toast'
import { useMailCert, useObtainMailCert } from '../../queries/mailCert'

/** MailCertCard shows the certificate the mail ports serve and can ask for a new one. */
export function MailCertCard({ installed }: { installed: boolean }) {
  const { t } = useTranslation('mailServer')
  const cert = useMailCert(installed)
  const obtain = useObtainMailCert()
  const data = cert.data
  if (!installed || !data) return null

  const tone = !data.served || data.self_signed || data.days_left < 15
  const rows: [string, string | undefined][] = [
    [t('cert.host'), data.hostname],
    [t('cert.source'), data.source],
    [t('cert.issuer'), data.issuer],
    [t('cert.expires'), data.not_after],
    [t('cert.serial'), data.serial],
    [t('cert.fingerprint'), data.sha256],
  ]

  return (
    <section
      aria-label={t('cert.title')}
      className="rounded-lg border border-border bg-card p-3"
    >
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-sm font-medium">{t('cert.title')}</h2>
        <span className="flex items-center gap-2">
          {data.served ? (
            <StatusPill
              tone={tone ? 'warning' : 'success'}
              label={
                data.self_signed
                  ? t('cert.selfSigned')
                  : t('cert.daysLeft', { count: data.days_left })
              }
              size="sm"
            />
          ) : null}
          <Button
            size="sm"
            variant="outline"
            disabled={obtain.isPending}
            writeAction
            onClick={() =>
              obtain.mutate(data.app, {
                onSuccess: () =>
                  toast.add({ title: t('cert.obtained'), type: 'success' }),
                onError: (e) =>
                  toast.add({
                    title: t('cert.failed'),
                    description: e.message,
                    type: 'error',
                  }),
              })
            }
          >
            {obtain.isPending ? t('cert.obtaining') : t('cert.obtain')}
          </Button>
        </span>
      </div>
      {data.served ? (
        <dl
          aria-label={t('cert.listLabel')}
          className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs"
        >
          {rows
            .filter(([, v]) => v)
            .map(([k, v]) => (
              <div key={k} className="contents">
                <dt className="text-muted-foreground">{k}</dt>
                <dd className="truncate font-mono" title={v}>
                  {v}
                </dd>
              </div>
            ))}
        </dl>
      ) : (
        <p className="text-sm text-muted-foreground">{t('cert.notServed')}</p>
      )}
      {data.probe_error ? (
        <p className="mt-1 text-xs text-destructive">
          {t('cert.probeError', { message: data.probe_error })}
        </p>
      ) : null}
      {data.next_step ? (
        <p className="mt-1 text-xs text-muted-foreground">{data.next_step}</p>
      ) : null}
    </section>
  )
}
