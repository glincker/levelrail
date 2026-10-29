import { Link } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { WrenchGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useIsRoot } from '../hooks/useIsRoot'
import { domainScope, isStandaloneDomain } from '../lib/standaloneDomain'
import { useClearDomainTLSCert } from '../queries/domainTlsCert'
import { domainKeys, type Domain, type DomainIssue } from '../queries/domains'

function SwitchToAcme({ domain }: { domain: Domain }) {
  const { t } = useTranslation('domains')
  const queryClient = useQueryClient()
  const isRoot = useIsRoot()
  const clear = useClearDomainTLSCert(domain.service_name, domain.domain)
  if (!isRoot) return null
  return (
    <Button
      type="button"
      size="xs"
      variant="outline"
      disabled={clear.isPending}
      onClick={() => {
        clear.mutate(undefined, {
          onSuccess: () => {
            void queryClient.invalidateQueries({ queryKey: domainKeys.all })
            toast.add({
              title: t('page.fix.acmeDone', { domain: domain.domain }),
              type: 'success',
            })
          },
          onError: (err) => {
            toast.add({ title: err.message, type: 'error' })
          },
        })
      }}
    >
      <WrenchGlassIcon size="1.4em" aria-hidden="true" />
      {t('page.fix.useAcme')}
    </Button>
  )
}

function DomainPageLink({ domain, label }: { domain: Domain; label: string }) {
  return (
    <Button
      size="xs"
      variant="outline"
      nativeButton={false}
      render={
        <Link
          to="/domains/$domain"
          params={{ domain: domain.domain }}
          search={{ app: domainScope(domain) }}
        />
      }
    >
      <WrenchGlassIcon size="1.4em" aria-hidden="true" />
      {label}
    </Button>
  )
}

/** DomainIssueFix gives each ingress issue code a typed fix: an in-place action where the control plane can act, else a link to the control that holds the fix. */
export function DomainIssueFix({
  domain,
  issue,
}: {
  domain: Domain
  issue: DomainIssue
}) {
  const { t } = useTranslation('domains')
  switch (issue.code) {
    case 'custom_cert_unresolved':
      return (
        <span className="mt-1 flex flex-wrap items-center gap-2">
          {isStandaloneDomain(domain) ? null : <SwitchToAcme domain={domain} />}
          <DomainPageLink domain={domain} label={t('page.fix.uploadAgain')} />
        </span>
      )
    case 'basic_auth_unresolved':
      return (
        <span className="mt-1 flex">
          <DomainPageLink domain={domain} label={t('page.fix.setPassword')} />
        </span>
      )
    case 'dns01_unavailable':
      return (
        <span className="mt-1 flex">
          <Button
            size="xs"
            variant="outline"
            nativeButton={false}
            render={<Link to="/dns" />}
          >
            <WrenchGlassIcon size="1.4em" aria-hidden="true" />
            {t('page.fix.connectDns')}
          </Button>
        </span>
      )
    case 'target_unavailable':
      return (
        <span className="mt-1 flex">
          {isStandaloneDomain(domain) ? (
            <DomainPageLink domain={domain} label={t('page.fix.checkTarget')} />
          ) : (
            <Button
              size="xs"
              variant="outline"
              nativeButton={false}
              render={
                <Link to="/apps/$name" params={{ name: domain.service_name }} />
              }
            >
              <WrenchGlassIcon size="1.4em" aria-hidden="true" />
              {t('page.fix.checkApp')}
            </Button>
          )}
        </span>
      )
    default:
      return null
  }
}
