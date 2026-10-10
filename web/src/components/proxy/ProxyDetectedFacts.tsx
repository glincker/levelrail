import { useTranslation } from 'react-i18next'
import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import type { DetectedProxy } from '../../queries/proxyIntegration'

function Fact({ label, value }: { label: string; value: string }) {
  const { t } = useTranslation('domains')
  return (
    <div className="flex items-baseline gap-2 py-1">
      <dt className="w-36 shrink-0 text-xs text-muted-foreground">{label}</dt>
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

export function ProxyDetectedFacts({ detected }: { detected: DetectedProxy }) {
  const { t } = useTranslation('domains')
  return (
    <div className="space-y-3">
      <h3 className="text-sm font-semibold text-foreground">
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
      {detected.complete ? null : (
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
