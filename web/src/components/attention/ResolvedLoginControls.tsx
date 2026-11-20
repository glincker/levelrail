import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { CheckGlassIcon, CopyGlassIcon, XGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { FixCli } from '../fixit/FixCli'
import { useDismissAttentionItem } from '../../queries/deviceAuth'

export const DEVICE_LOGIN_COMMAND = 'levelrail-cli auth login --device'

// Start over, audit link and dismiss for an expired or denied CLI login.
// Rendered inside a wrapping flex row: the copyable command opens on its
// own full-width line.
export function ResolvedLoginControls({
  dismissKey,
  auditPath,
  compact = false,
}: {
  dismissKey: string
  auditPath: string
  // Compact rows swap the full-width fix row for an inline copy button.
  compact?: boolean
}) {
  const { t } = useTranslation('attention', { useSuspense: false })
  const dismiss = useDismissAttentionItem()
  const [copied, setCopied] = useState(false)
  return (
    <>
      <div className="flex items-center gap-1">
        <Button
          size="sm"
          variant="ghost"
          render={
            <Link to="/settings/audit-log" search={{ path: auditPath }} />
          }
        >
          {t('deviceLogin.viewAudit')}
        </Button>
        {compact ? (
          <Button
            type="button"
            size="icon-sm"
            variant="ghost"
            aria-label={t(copied ? 'deviceLogin.copied' : 'deviceLogin.copy')}
            title={DEVICE_LOGIN_COMMAND}
            onClick={() => {
              void navigator.clipboard
                .writeText(DEVICE_LOGIN_COMMAND)
                .then(() => {
                  setCopied(true)
                })
            }}
          >
            {copied ? (
              <CheckGlassIcon size="1.4em" />
            ) : (
              <CopyGlassIcon size="1.4em" />
            )}
          </Button>
        ) : null}
        <Button
          type="button"
          size="icon-sm"
          variant="ghost"
          aria-label={t('deviceLogin.dismiss')}
          disabled={dismiss.isPending}
          onClick={() => {
            dismiss.mutate(dismissKey, {
              onSuccess: () => {
                toast.add({
                  title: t('deviceLogin.dismissed'),
                  type: 'success',
                })
              },
              onError: (error) => {
                toast.add({ title: error.message, type: 'error' })
              },
            })
          }}
        >
          <XGlassIcon size="1.4em" />
        </Button>
      </div>
      {compact ? null : (
        <div className="w-full">
          <FixCli
            steps={[
              {
                label: t('deviceLogin.startOverHint'),
                command: DEVICE_LOGIN_COMMAND,
              },
            ]}
          />
        </div>
      )}
    </>
  )
}
