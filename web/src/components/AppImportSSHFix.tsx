import { useTranslation } from 'react-i18next'
import { WrenchGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import type { SSHProbeResult } from '../queries/appImportSSH'

const SOCKET_CANDIDATES = ['/var/run/docker.sock', '/run/docker.sock']

interface Props {
  result: SSHProbeResult
  busy: boolean
  currentSocket: string
  onRecheck: () => void
  onUseFingerprint: (fingerprint: string) => void
  onUseSocket: (path: string) => void
  onAllowPrivate: () => void
}

export function AppImportSSHFix({
  result,
  busy,
  currentSocket,
  onRecheck,
  onUseFingerprint,
  onUseSocket,
  onAllowPrivate,
}: Props) {
  const { t } = useTranslation('migration')
  const kind = result.kind ?? 'other'
  const sockets = SOCKET_CANDIDATES.filter((p) => p !== currentSocket)
  return (
    <Alert variant="destructive" role="alert">
      <WrenchGlassIcon size="1.4em" aria-hidden />
      <AlertTitle>{t(`appImport.ssh.fix.${kind}.title`)}</AlertTitle>
      <AlertDescription className="space-y-2">
        <p>{t(`appImport.ssh.fix.${kind}.body`)}</p>
        {result.message ? (
          <p className="font-mono text-xs break-words">{result.message}</p>
        ) : null}
        <div className="flex flex-wrap gap-2">
          {kind === 'host_key_mismatch'
            ? (result.observed_host_keys ?? []).map((k) => (
                <Button
                  key={k.fingerprint}
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => onUseFingerprint(k.fingerprint)}
                >
                  {t('appImport.ssh.fix.host_key_mismatch.trust', {
                    fingerprint: k.fingerprint,
                  })}
                </Button>
              ))
            : null}
          {kind === 'socket_path'
            ? sockets.map((p) => (
                <Button
                  key={p}
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={busy}
                  onClick={() => onUseSocket(p)}
                >
                  {t('appImport.ssh.fix.socket_path.try', { path: p })}
                </Button>
              ))
            : null}
          {kind === 'blocked_address' ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={busy}
              onClick={onAllowPrivate}
            >
              {t('appImport.ssh.fix.blocked_address.allow')}
            </Button>
          ) : null}
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={busy}
            onClick={onRecheck}
          >
            {t('appImport.ssh.recheck')}
          </Button>
        </div>
        {kind === 'socket_permission' ? (
          <details>
            <summary className="cursor-pointer text-xs">
              {t('appImport.ssh.fix.viaCli')}
            </summary>
            <code className="mt-1 block rounded-md bg-muted px-2 py-1 text-xs text-foreground">
              {t('appImport.ssh.fix.socket_permission.command')}
            </code>
          </details>
        ) : null}
      </AlertDescription>
    </Alert>
  )
}
