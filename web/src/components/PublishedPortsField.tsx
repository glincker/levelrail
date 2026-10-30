import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import type { PublishedPort } from '../types/publishedPort'

// One compact chip per published port: host port, protocol badge, and the
// container port it forwards to. Renders nothing when there are none.
export function PublishedPortsField({ ports }: { ports?: PublishedPort[] }) {
  const { t } = useTranslation('https')
  if (!ports || ports.length === 0) return null
  return (
    <ul
      aria-label={t('network.publishedPorts')}
      className="flex flex-wrap items-center gap-2"
    >
      {ports.map((p) => (
        <li
          key={`${p.host_port}/${p.protocol}`}
          className="inline-flex items-center gap-1.5 rounded-md border border-border px-2 py-0.5"
        >
          <span className="font-mono">
            {p.container_port !== p.host_port
              ? t('network.portMapping', {
                  host: p.host_port,
                  container: p.container_port,
                })
              : p.host_port}
          </span>
          <Badge variant={p.protocol === 'udp' ? 'info' : 'muted'}>
            {p.protocol.toUpperCase()}
          </Badge>
        </li>
      ))}
    </ul>
  )
}
