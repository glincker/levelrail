import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { XIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { CopyCommand } from '../settings/CopyCommand'
import { useDismissAttentionItem } from '../../queries/deviceAuth'

export const DEVICE_LOGIN_COMMAND = 'levelrail-cli auth login --device'

// Start over, audit link and dismiss for an expired or denied CLI login.
// Rendered inside a wrapping flex row: the copyable command opens on its
// own full-width line.
export function ResolvedLoginControls({
  dismissKey,
  auditPath,
}: {
  dismissKey: string
  auditPath: string
}) {
  const { t } = useTranslation('attention', { useSuspense: false })
  const dismiss = useDismissAttentionItem()
  const [startOver, setStartOver] = useState(false)
  return (
    <>
      <div className="flex items-center gap-2">
        <Button
          type="button"
          size="sm"
          variant="outline"
          aria-expanded={startOver}
          onClick={() => {
            setStartOver((v) => !v)
          }}
        >
          {t('deviceLogin.startOver')}
        </Button>
        <Button
          size="sm"
          variant="ghost"
          render={
            <Link to="/settings/audit-log" search={{ path: auditPath }} />
          }
        >
          {t('deviceLogin.viewAudit')}
        </Button>
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
          <XIcon />
        </Button>
      </div>
      {startOver ? (
        <div className="w-full">
          <CopyCommand
            label={t('deviceLogin.startOverHint')}
            command={DEVICE_LOGIN_COMMAND}
          />
        </div>
      ) : null}
    </>
  )
}
