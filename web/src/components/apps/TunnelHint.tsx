import { useTranslation } from 'react-i18next'
import { CheckGlassIcon, CopyGlassIcon, PlugGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'

interface TunnelHintProps {
  appName: string
  port: number
  publishedPorts?: { container_port: number; protocol: string }[]
}

function tunnelPorts(
  port: number,
  publishedPorts: TunnelHintProps['publishedPorts'],
): number[] {
  const published = (publishedPorts ?? [])
    .filter((p) => p.protocol !== 'udp')
    .map((p) => p.container_port)
  const all = [port, ...published]
  return Array.from(new Set(all.filter((p) => p > 0)))
}

function tunnelCommand(appName: string, port: number): string {
  return `levelrail-cli apps tunnel ${appName} --port ${String(port)}`
}

function CopyCommand({ command, port }: { command: string; port: number }) {
  const { t } = useTranslation('appExposure')
  const { copied, copy } = useCopyToClipboard()
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon-sm"
      aria-label={copied ? t('tunnel.copied') : t('tunnel.copy', { port })}
      onClick={() => {
        copy(command)
      }}
    >
      {copied ? (
        <CheckGlassIcon size="1.4em" aria-hidden="true" />
      ) : (
        <CopyGlassIcon size="1.4em" aria-hidden="true" />
      )}
    </Button>
  )
}

// Shows the exact CLI command per container port, for ports that stay unpublished.
export function TunnelHint({ appName, port, publishedPorts }: TunnelHintProps) {
  const { t } = useTranslation('appExposure')
  const ports = tunnelPorts(port, publishedPorts)
  if (ports.length === 0) return null
  return (
    <div className="mt-5 rounded-md bg-muted p-3">
      <p className="flex items-center gap-2 text-xs font-medium tracking-wide text-muted-foreground uppercase">
        <PlugGlassIcon className="size-5" aria-hidden="true" />
        {t('tunnel.title')}
      </p>
      <p className="mt-1 text-xs text-muted-foreground">
        {t('tunnel.description')}
      </p>
      <ul className="mt-2 space-y-1">
        {ports.map((p) => {
          const command = tunnelCommand(appName, p)
          return (
            <li key={p} className="flex items-center gap-2">
              <code className="min-w-0 flex-1 truncate font-mono text-xs text-foreground">
                {command}
              </code>
              <CopyCommand command={command} port={p} />
            </li>
          )
        })}
      </ul>
    </div>
  )
}
