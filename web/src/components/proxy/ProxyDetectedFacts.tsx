import { useTranslation } from 'react-i18next'
import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import type { DetectedProxy } from '../../queries/proxyIntegration'

function Fact({ label, value }: { label: string; value: string }) {
  const { t } = useTranslation('domains')
  return (
    <div className="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-0.5 py-1">
      <dt className="w-full shrink-0 text-xs text-muted-foreground sm:w-36">
        {label}
      </dt>
      <dd className="min-w-0 flex-1 font-mono text-xs break-all text-foreground">
        {value || (
          <span className="font-sans text-muted-foreground">
            {t('proxySetup.facts.notFound')}
          </span>
        )}
      </dd>
      {value ? (
        <Badge variant="muted">{t('proxySetup.facts.detectedBadge')}</Badge>
      ) : null}
    </div>
  )
}

export function ProxyDetectedFacts({
  detected,
  quietMissing = false,
}: {
  detected: DetectedProxy
  quietMissing?: boolean
}) {
  const { t } = useTranslation('domains')
  return (
    <div className="min-w-0 space-y-3">
      <h3 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight) text-foreground">
        {t('proxySetup.facts.title')}
      </h3>
      <dl className="divide-y divide-border rounded-md border border-border px-3">
        <Fact
          label={t('proxySetup.facts.proxy')}
          value={t(`proxySetup.facts.kind.${detected.kind}`)}
        />
        <Fact
          label={t('proxySetup.facts.container')}
          value={detected.container}
        />
        <Fact label={t('proxySetup.facts.image')} value={detected.image} />
        <Fact
          label={t('proxySetup.facts.ports')}
          value={detected.published_ports.join(', ')}
        />
        <Fact
          label={t('proxySetup.facts.directory')}
          value={detected.dynamic_dir}
        />
        <Fact
          label={t('proxySetup.facts.entrypointHttp')}
          value={detected.entrypoint_http}
        />
        <Fact
          label={t('proxySetup.facts.entrypointHttps')}
          value={detected.entrypoint_https}
        />
        <Fact
          label={t('proxySetup.facts.resolver')}
          value={detected.cert_resolver}
        />
        <Fact
          label={t('proxySetup.facts.upstream')}
          value={detected.upstream_host}
        />
      </dl>
      {detected.complete || quietMissing ? null : (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertTitle>{t('proxySetup.facts.missingTitle')}</AlertTitle>
          <AlertDescription className="space-y-2">
            <p>{t('proxySetup.facts.missingBody')}</p>
            <ul className="list-disc pl-4 font-mono text-xs">
              {detected.missing.map((item) => (
                <li key={item}>{item}</li>
              ))}
            </ul>
          </AlertDescription>
        </Alert>
      )}
    </div>
  )
}
