import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { CaretDownIcon, TrashIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import { RetentionDaysSelect } from './settings/RetentionDaysSelect'
import { RelativeTime } from './kit/RelativeTime'
import { useIsRoot } from '../hooks/useIsRoot'
import {
  useRuntimeSettings,
  useSetRuntimeSetting,
} from '../queries/runtimeSettings'
import { useCleanupTokens } from '../queries/tokens'
import type { TokenResource } from '../types/token'

export const REVOKED_TOKEN_SETTING = 'retention.revoked_token_days'

export function RevokedTokensCard({ tokens }: { tokens: TokenResource[] }) {
  const { t } = useTranslation('settings')
  const isRoot = useIsRoot()
  const { data } = useRuntimeSettings()
  const save = useSetRuntimeSetting()
  const cleanup = useCleanupTokens()
  const [confirming, setConfirming] = useState(false)
  const setting = data?.settings.find((s) => s.key === REVOKED_TOKEN_SETTING)
  const revoked = tokens.filter((tok) => tok.revoked_at)

  function onCleanup() {
    cleanup.mutate(0, {
      onSuccess: (res) => {
        setConfirming(false)
        toast.add({
          type: 'success',
          title: t('tokensPage.revoked.cleaned', { count: res.deleted }),
        })
      },
    })
  }

  return (
    <Collapsible className="rounded-lg border border-border px-4 py-3">
      <CollapsibleTrigger className="flex items-center justify-between gap-2 text-sm font-medium">
        <span className="flex items-center gap-2">
          {t('tokensPage.revoked.title')}
          <Badge variant="muted">{revoked.length}</Badge>
        </span>
        <CaretDownIcon className="size-4" aria-hidden="true" />
      </CollapsibleTrigger>
      <CollapsiblePanel className="space-y-3 pt-3">
        <div className="flex flex-wrap items-center justify-between gap-3">
          {setting ? (
            <div className="flex flex-wrap items-center gap-2 text-sm">
              <span>{t('tokensPage.revoked.autoDelete')}</span>
              <RetentionDaysSelect
                value={Number(setting.value)}
                allowed={setting.allowed_values ?? []}
                disabled={!isRoot || save.isPending}
                ariaLabel={t('tokensPage.revoked.autoDelete')}
                onChange={(days) => {
                  save.mutate(
                    { key: setting.key, value: String(days) },
                    {
                      onError: (err) => {
                        toast.add({ type: 'error', title: err.message })
                      },
                    },
                  )
                }}
              />
            </div>
          ) : null}
          {isRoot ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={revoked.length === 0}
              onClick={() => {
                setConfirming(true)
              }}
            >
              <TrashIcon aria-hidden="true" />
              {t('tokensPage.revoked.cleanNow')}
            </Button>
          ) : null}
        </div>
        <p className="text-xs text-muted-foreground">
          {t('tokensPage.revoked.schedule')}{' '}
          <Link
            to="/settings/janitor"
            className="underline underline-offset-4 hover:no-underline"
          >
            {t('tokensPage.revoked.janitorLink')}
          </Link>
        </p>
        {revoked.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('tokensPage.revoked.empty')}
          </p>
        ) : (
          <ul className="divide-y divide-border text-sm">
            {revoked.map((tok) => (
              <li
                key={tok.id}
                className="flex items-center justify-between gap-3 py-1.5"
              >
                <span className="truncate">{tok.name}</span>
                {tok.revoked_at ? (
                  <span className="shrink-0 text-xs text-muted-foreground">
                    {t('tokensPage.revoked.revokedAt')}{' '}
                    <RelativeTime at={tok.revoked_at} />
                  </span>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </CollapsiblePanel>
      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        title={t('tokensPage.revoked.confirmTitle')}
        description={t('tokensPage.revoked.confirmBody', {
          count: revoked.length,
        })}
        consequences={[t('tokensPage.revoked.confirmConsequence')]}
        confirmLabel={t('tokensPage.revoked.confirmAction')}
        tone="destructive"
        pending={cleanup.isPending}
        error={cleanup.error?.message ?? null}
        onConfirm={onCleanup}
      />
    </Collapsible>
  )
}
