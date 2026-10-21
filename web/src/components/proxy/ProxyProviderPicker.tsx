import { useTranslation } from 'react-i18next'
import { BrandIcon, type BrandIconName } from '@/components/BrandIcon'
import { Badge } from '@/components/ui/badge'
import type {
  ProxyProvider,
  ProxyProviderKind,
} from '../../queries/proxyIntegration'

const BRAND_MARKS: Partial<Record<ProxyProviderKind, BrandIconName>> = {
  embedded: 'caddy',
  caddy: 'caddy',
  traefik: 'traefik',
  nginx: 'nginx',
}

// HAProxy has no mark in @thesvg/react yet, so it gets the monogram tile.
function ProviderMark({ kind }: { kind: ProxyProviderKind }) {
  const name = BRAND_MARKS[kind]
  if (name) return <BrandIcon name={name} className="size-5 shrink-0" />
  return (
    <span
      aria-hidden="true"
      className="flex size-5 shrink-0 items-center justify-center rounded bg-muted text-xs font-semibold text-muted-foreground"
    >
      HA
    </span>
  )
}

export function ProxyProviderPicker({
  providers,
  value,
  onChange,
}: {
  providers: ProxyProvider[]
  value: ProxyProviderKind
  onChange: (kind: ProxyProviderKind) => void
}) {
  const { t } = useTranslation('domains')
  return (
    <fieldset className="space-y-2">
      <legend className="text-sm font-medium text-foreground">
        {t('proxyPicker.choose')}
      </legend>
      <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
        {providers.map((p) => (
          <label
            key={p.kind}
            className="flex cursor-pointer items-center gap-3 rounded-lg border border-border px-3 py-2 transition-colors hover:bg-muted/40 has-checked:border-ring has-checked:bg-muted/40 has-focus-visible:ring-2 has-focus-visible:ring-ring"
          >
            <input
              type="radio"
              name="proxy-provider"
              className="sr-only"
              checked={value === p.kind}
              onChange={() => onChange(p.kind)}
            />
            <ProviderMark kind={p.kind} />
            <span className="min-w-0 flex-1">
              <span className="block truncate text-sm font-medium text-foreground">
                {t(`proxyPicker.provider.${p.kind}`)}
              </span>
              <span className="block text-xs text-muted-foreground">
                {t(`proxyPicker.mode.${p.mode}`)}
              </span>
            </span>
            {p.detected ? (
              <Badge variant="success">{t('proxyPicker.detected')}</Badge>
            ) : null}
          </label>
        ))}
      </div>
    </fieldset>
  )
}
