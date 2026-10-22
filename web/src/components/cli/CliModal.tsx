import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { CheckIcon, CopyIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { toast } from '@/components/ui/toast'
import { CopyCommand } from '../settings/CopyCommand'
import { useCliPairing, formatLeft } from '../../hooks/useCliPairing'
import { CLI_INSTALL_COMMAND } from '../../lib/cliInstall'
import { setCliModalOpen, useCliModalOpen } from '../../lib/cliModalStore'
import { formatAbsoluteTime, formatRelativeTime } from '../../lib/relativeTime'
import { myCredentialsKey, useCliTokens } from '../../queries/myCredentials'
import { useRevokeToken } from '../../queries/tokens'
import type { TokenResource } from '../../types/token'

function ConnectSection() {
  const { t } = useTranslation('signIn', { useSuspense: false })
  const p = useCliPairing()
  return (
    <section aria-labelledby="cli-connect-title" className="space-y-2">
      <h3 id="cli-connect-title" className="text-sm font-semibold">
        {t('cliModal.connect.title')}
      </h3>
      <p className="text-sm text-muted-foreground">
        {t('cliModal.connect.lead')}
      </p>
      <div aria-live="polite" className="space-y-2">
        {p.live ? (
          <>
            <code
              aria-label={t('connectCli.commandLabel')}
              data-testid="connect-cli-command"
              className="block select-all overflow-x-auto rounded-md bg-muted px-3 py-2 font-mono text-xs"
            >
              {p.command}
            </code>
            <p className="text-xs text-muted-foreground">
              {t('connectCli.expiresIn', { time: formatLeft(p.left) })}
            </p>
          </>
        ) : p.expired ? (
          <p className="text-sm text-muted-foreground">
            {t('connectCli.expired')}
          </p>
        ) : null}
      </div>
      <div className="flex flex-wrap gap-2">
        {p.live ? (
          <Button type="button" size="sm" onClick={p.copy}>
            {p.copied ? (
              <CheckIcon aria-hidden="true" />
            ) : (
              <CopyIcon aria-hidden="true" />
            )}
            {t('connectCli.copy')}
          </Button>
        ) : null}
        <Button
          type="button"
          size="sm"
          variant={p.live ? 'outline' : 'default'}
          disabled={p.pending}
          onClick={p.generate}
        >
          {p.pairing ? t('connectCli.regenerate') : t('connectCli.generate')}
        </Button>
      </div>
    </section>
  )
}

function ManageSection({ onFullPage }: { onFullPage: () => void }) {
  const { t } = useTranslation('signIn', { useSuspense: false })
  const qc = useQueryClient()
  const { data: tokens = [], isPending } = useCliTokens()
  const revoke = useRevokeToken()
  const [target, setTarget] = useState<TokenResource | null>(null)
  return (
    <section aria-labelledby="cli-manage-title" className="space-y-2">
      <h3 id="cli-manage-title" className="text-sm font-semibold">
        {t('cliModal.manage.title')}
      </h3>
      {isPending ? (
        <p className="text-sm text-muted-foreground">
          {t('cliModal.manage.loading')}
        </p>
      ) : tokens.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('cliModal.manage.empty')}
        </p>
      ) : (
        <ul className="divide-y rounded-md border">
          {tokens.map((tok) => (
            <li
              key={tok.id}
              className="flex items-center justify-between gap-3 px-3 py-2"
            >
              <div className="min-w-0 space-y-0.5">
                <p className="truncate text-sm font-medium">{tok.name}</p>
                <p className="text-xs text-muted-foreground">
                  {tok.last_used_at
                    ? t('cliModal.manage.lastUsed', {
                        when: formatRelativeTime(tok.last_used_at),
                      })
                    : t('cliModal.manage.neverUsed')}
                  {tok.last_ip
                    ? ` · ${t('cliModal.manage.lastIp', { ip: tok.last_ip })}`
                    : ''}
                  {' · '}
                  {tok.expires_at
                    ? t('cliModal.manage.expires', {
                        when: formatAbsoluteTime(tok.expires_at),
                      })
                    : t('cliModal.manage.noExpiry')}
                </p>
              </div>
              <Button
                type="button"
                size="sm"
                variant="outline"
                aria-label={t('cliModal.manage.revokeLabel', {
                  name: tok.name,
                })}
                onClick={() => {
                  setTarget(tok)
                }}
              >
                {t('cliModal.manage.revoke')}
              </Button>
            </li>
          ))}
        </ul>
      )}
      <Link
        to="/settings/cli-access"
        onClick={onFullPage}
        className="inline-block text-sm text-primary underline-offset-4 hover:underline"
      >
        {t('cliModal.manage.fullPage')}
      </Link>
      <ConfirmDialog
        open={target !== null}
        onOpenChange={(next) => {
          if (!next) {
            setTarget(null)
          }
        }}
        title={t('cliModal.manage.revokeTitle')}
        description={t('cliModal.manage.revokeBody', {
          name: target?.name ?? '',
        })}
        tone="destructive"
        confirmLabel={t('cliModal.manage.revokeConfirm')}
        pendingLabel={t('cliModal.manage.revoking')}
        cancelLabel={t('cliModal.manage.revokeCancel')}
        pending={revoke.isPending}
        onConfirm={() => {
          if (!target) {
            return
          }
          revoke.mutate(target.id, {
            onSuccess: () => {
              void qc.invalidateQueries({ queryKey: myCredentialsKey })
              toast.add({
                title: t('cliModal.manage.revoked'),
                type: 'success',
              })
              setTarget(null)
            },
            onError: () => {
              toast.add({
                title: t('cliModal.manage.revokeFailed'),
                type: 'error',
              })
            },
          })
        }}
      />
    </section>
  )
}

// Mounted once in the app header so the shortcut and palette entry can open
// it even while the header badge is hidden.
export function CliModal() {
  const { t } = useTranslation('signIn', { useSuspense: false })
  const open = useCliModalOpen()
  return (
    <Dialog open={open} onOpenChange={setCliModalOpen}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('cliModal.title')}</DialogTitle>
          <DialogDescription>{t('cliModal.description')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-5">
          <section aria-labelledby="cli-install-title" className="space-y-2">
            <h3 id="cli-install-title" className="text-sm font-semibold">
              {t('cliModal.install.title')}
            </h3>
            <p className="text-sm text-muted-foreground">
              {t('cliModal.install.lead')}
            </p>
            <CopyCommand
              label={t('cliModal.install.label')}
              command={CLI_INSTALL_COMMAND}
            />
          </section>
          <ConnectSection />
          <ManageSection
            onFullPage={() => {
              setCliModalOpen(false)
            }}
          />
        </div>
      </DialogContent>
    </Dialog>
  )
}
