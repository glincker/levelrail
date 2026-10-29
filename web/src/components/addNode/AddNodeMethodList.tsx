import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArrowRightGlassIcon,
  ServerGlassIcon,
  TerminalGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { BrandTile } from '../BrandLogoBadge'
import { nodeProviderBrand } from '../../lib/providerBrands'

export type CloudProviderId =
  'hetzner' | 'digitalocean' | 'aws' | 'azure' | 'gcp'

const CLOUD_PROVIDERS: readonly CloudProviderId[] = [
  'hetzner',
  'digitalocean',
  'aws',
  'azure',
  'gcp',
]

const NAME_KEYS = {
  hetzner: 'nodeProviders.names.hetzner',
  digitalocean: 'nodeProviders.names.digitalocean',
  aws: 'nodeProviders.names.aws',
  azure: 'nodeProviders.names.azure',
  gcp: 'nodeProviders.names.gcp',
} as const

const HINT_KEYS = {
  hetzner: 'nodeProviders.hints.hetzner',
  digitalocean: 'nodeProviders.hints.digitalocean',
  aws: 'nodeProviders.hints.aws',
  azure: 'nodeProviders.hints.azure',
  gcp: 'nodeProviders.hints.gcp',
} as const

interface RowProps {
  mark: React.ReactNode
  title: string
  hint: string
  trailing: React.ReactNode
  onClick?: () => void
  disabled?: boolean
}

function Row({ mark, title, hint, trailing, onClick, disabled }: RowProps) {
  const body = (
    <>
      {mark}
      <span className="min-w-0 flex-1">
        <span className="block text-sm font-medium text-foreground">
          {title}
        </span>
        <span className="block truncate text-xs text-muted-foreground">
          {hint}
        </span>
      </span>
      {trailing}
    </>
  )
  const base =
    'flex w-full items-center gap-3 rounded-lg border border-border p-3 text-left transition-colors'
  if (onClick === undefined) {
    return <div className={cn(base, disabled && 'bg-muted/30')}>{body}</div>
  }
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        base,
        'hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none',
      )}
    >
      {body}
    </button>
  )
}

// Method step of Add node: connected clouds first and selectable, the rest
// with a clear connect action, then the two bring-your-own-machine paths.
export function AddNodeMethodList({
  connected,
  loading,
  onPickProvider,
  onSSH,
  onManual,
}: {
  connected: ReadonlySet<string>
  loading: boolean
  onPickProvider: (id: CloudProviderId) => void
  onSSH: () => void
  onManual: () => void
}) {
  const { t } = useTranslation('settings')
  const ordered = [...CLOUD_PROVIDERS].sort(
    (a, b) => Number(connected.has(b)) - Number(connected.has(a)),
  )

  return (
    <div className="space-y-5">
      <section
        className="space-y-2"
        aria-label={t('nodeProviders.addNodeDialog.cloud')}
      >
        <h3 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
          {t('nodeProviders.addNodeDialog.cloud')}
        </h3>
        {ordered.map((id) => {
          const name = t(NAME_KEYS[id])
          const isConnected = connected.has(id)
          const mark = (
            <BrandTile name={nodeProviderBrand(id)} label={name} labelled />
          )
          if (isConnected) {
            return (
              <Row
                key={id}
                mark={mark}
                title={name}
                hint={t(HINT_KEYS[id])}
                onClick={() => {
                  onPickProvider(id)
                }}
                trailing={
                  <span className="flex shrink-0 items-center gap-2">
                    <Badge variant="success">
                      {t('nodeProviders.connected')}
                    </Badge>
                    <ArrowRightGlassIcon
                      aria-hidden="true"
                      className="size-4.5 text-muted-foreground"
                    />
                  </span>
                }
              />
            )
          }
          return (
            <Row
              key={id}
              mark={mark}
              title={name}
              hint={
                loading
                  ? t(HINT_KEYS[id])
                  : t('nodeProviders.addNodeDialog.connectFirst', {
                      provider: name,
                    })
              }
              disabled
              trailing={
                loading ? null : (
                  <Link
                    to="/settings/node-providers"
                    search={{ connect: id }}
                    className="shrink-0 rounded-md border border-border px-2.5 py-1 text-xs font-medium text-foreground hover:bg-muted"
                  >
                    {t('nodeProviders.connect')}
                  </Link>
                )
              }
            />
          )
        })}
      </section>
      <section
        className="space-y-2"
        aria-label={t('nodeProviders.addNodeDialog.own')}
      >
        <h3 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
          {t('nodeProviders.addNodeDialog.own')}
        </h3>
        <Row
          mark={
            <span className="flex size-(--glinr-tile-size) shrink-0 items-center justify-center rounded-lg border border-border bg-background">
              <TerminalGlassIcon aria-hidden="true" className="size-5.5" />
            </span>
          }
          title={t('nodeProviders.addNodeDialog.sshTitle')}
          hint={t('nodeProviders.addNodeDialog.sshHint')}
          onClick={onSSH}
          trailing={
            <ArrowRightGlassIcon
              aria-hidden="true"
              className="size-4.5 shrink-0 text-muted-foreground"
            />
          }
        />
        <Row
          mark={
            <span className="flex size-(--glinr-tile-size) shrink-0 items-center justify-center rounded-lg border border-border bg-background">
              <ServerGlassIcon aria-hidden="true" className="size-5.5" />
            </span>
          }
          title={t('nodeProviders.addNodeDialog.manualTitle')}
          hint={t('nodeProviders.addNodeDialog.manualHint')}
          onClick={onManual}
          trailing={
            <ArrowRightGlassIcon
              aria-hidden="true"
              className="size-4.5 shrink-0 text-muted-foreground"
            />
          }
        />
      </section>
    </div>
  )
}
