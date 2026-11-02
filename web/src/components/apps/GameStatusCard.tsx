import { useTranslation } from 'react-i18next'
import { CheckGlassIcon, CopyGlassIcon, PlugGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import { useGameStatus, type GameStatus } from '../../queries/appGameStatus'

// An HTTP app in direct mode also has a TCP port; only show it as a game when it answers like one.
function isGame(s: GameStatus): boolean {
  if (s.exposure === 'none' || s.exposure === 'remote_node') return false
  if (s.exposure === 'direct') return s.kind === 'minecraft'
  return true
}

function JoinAddress({ status }: { status: GameStatus }) {
  const { t } = useTranslation('appExposure')
  const { copied, copy } = useCopyToClipboard()
  if (!status.join_address) {
    return (
      <p className="text-xs text-muted-foreground">
        {status.join_port
          ? t('game.noPublicHost', { port: status.join_port })
          : t('game.notExposed')}
      </p>
    )
  }
  const address = status.join_address
  return (
    <div className="flex items-center gap-1">
      <code className="rounded bg-muted px-2 py-1 font-mono text-sm">
        {address}
      </code>
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        aria-label={copied ? t('game.copied') : t('game.copy')}
        onClick={() => {
          copy(address)
        }}
      >
        {copied ? (
          <CheckGlassIcon size="1.4em" aria-hidden="true" />
        ) : (
          <CopyGlassIcon size="1.4em" aria-hidden="true" />
        )}
      </Button>
    </div>
  )
}

// Online state, players and the join address of a game server on a TCP port.
export function GameStatusCard({ appName }: { appName: string }) {
  const { t } = useTranslation('appExposure')
  const { data } = useGameStatus(appName)
  if (!data || !isGame(data)) return null
  const minecraft = data.kind === 'minecraft'
  return (
    <div className="mt-5 rounded-md bg-muted p-3">
      <div className="flex flex-wrap items-center gap-2">
        <p className="flex items-center gap-2 text-xs font-medium tracking-wide text-muted-foreground uppercase">
          <PlugGlassIcon className="size-5" aria-hidden="true" />
          {t('game.title')}
        </p>
        <Badge variant={data.online ? 'success' : 'muted'}>
          {data.online ? t('game.online') : t('game.offline')}
        </Badge>
        {data.online && minecraft && (
          <span className="text-xs text-muted-foreground">
            {t('game.players', {
              online: data.players,
              max: data.max_players,
            })}
            {data.version ? ` · ${data.version}` : ''}
          </span>
        )}
        {data.online && (
          <span className="text-xs text-muted-foreground">
            {t('game.latency', { ms: data.latency_ms })}
          </span>
        )}
      </div>
      <div className="mt-2">
        <JoinAddress status={data} />
      </div>
    </div>
  )
}
