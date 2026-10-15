import { Link } from '@tanstack/react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { domainKeys, domainsQueryOptions } from '../../queries/domains'
import { useClearDomainMaintenance } from '../../queries/domainMaintenance'
import { isEnforced } from '../../lib/domainEnforcement'

const MAX_SHOWN = 3

function DomainRow({
  app,
  domain,
  enforced,
}: {
  app: string
  domain: string
  enforced: boolean
}) {
  const { t } = useTranslation('maintenance')
  const queryClient = useQueryClient()
  const clear = useClearDomainMaintenance(app, domain)
  return (
    <Banner
      variant="warning"
      title={
        enforced
          ? t('domainBanner.title', { domain })
          : t('domainBanner.titleNotEnforced', { domain })
      }
      action={
        <div className="flex items-center gap-2">
          <Button
            size="sm"
            variant="ghost"
            className="text-muted-foreground"
            render={<Link to="/apps/$name/domains" params={{ name: app }} />}
          >
            {t('domainBanner.manage')}
          </Button>
          <Button
            size="sm"
            variant="outline"
            disabled={clear.isPending}
            onClick={() => {
              clear.mutate(undefined, {
                onSuccess: () => {
                  void queryClient.invalidateQueries({
                    queryKey: domainKeys.all,
                  })
                  toast.add({
                    title: t('domainBanner.ended', { domain }),
                    type: 'success',
                  })
                },
                onError: (err) => {
                  toast.add({ title: err.message, type: 'error' })
                },
              })
            }}
          >
            {clear.isPending ? t('domainBanner.ending') : t('domainBanner.end')}
          </Button>
        </div>
      }
    >
      <p>
        {enforced ? t('domainBanner.body') : t('domainBanner.bodyNotEnforced')}
      </p>
    </Banner>
  )
}

// Sticky for as long as any domain is in maintenance mode: it cannot be
// dismissed, so an operator never leaves a site down by accident.
export function DomainMaintenanceBanner() {
  const { t } = useTranslation('maintenance')
  const { data } = useQuery({
    ...domainsQueryOptions(),
    refetchInterval: 60_000,
  })
  const active = (data ?? []).filter((d) => d.maintenance_enabled)
  if (active.length === 0) return null
  return (
    <div className="flex flex-col gap-2 px-4 pt-2 md:px-6">
      {active.slice(0, MAX_SHOWN).map((d) => (
        <DomainRow
          key={d.domain}
          app={d.service_name}
          domain={d.domain}
          enforced={isEnforced(d.enforcement, 'maintenance')}
        />
      ))}
      {active.length > MAX_SHOWN ? (
        <p className="text-xs text-muted-foreground">
          {t('domainBanner.more', { count: active.length - MAX_SHOWN })}
        </p>
      ) : null}
    </div>
  )
}
