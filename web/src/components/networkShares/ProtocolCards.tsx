import { useTranslation } from 'react-i18next'
import {
  FolderNotchOpenIcon,
  HardDrivesIcon,
} from '@phosphor-icons/react/dist/ssr'
import { cn } from '@/lib/utils'
import type { NetworkShareProtocol } from '../../types/networkShare'

const CARDS = [
  { id: 'nfs', key: 'nfs', Icon: HardDrivesIcon },
  { id: 'cifs', key: 'smb', Icon: FolderNotchOpenIcon },
] as const

export function ProtocolCards({
  value,
  onChange,
}: Readonly<{
  value: NetworkShareProtocol
  onChange: (next: NetworkShareProtocol) => void
}>) {
  const { t } = useTranslation('settings')
  return (
    <div
      role="radiogroup"
      aria-label={t('networkSharesPage.dialog.protocol')}
      className="grid min-w-0 grid-cols-1 gap-2 sm:grid-cols-2"
    >
      {CARDS.map(({ id, key, Icon }) => {
        const selected = value === id
        return (
          <button
            key={id}
            type="button"
            role="radio"
            aria-checked={selected}
            onClick={() => {
              onChange(id)
            }}
            className={cn(
              'flex min-w-0 items-start gap-3 rounded-lg border border-input p-3 text-left transition-colors hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none',
              selected && 'border-primary bg-muted',
            )}
          >
            <Icon
              aria-hidden="true"
              className="mt-0.5 size-5 shrink-0 text-muted-foreground"
            />
            <span className="min-w-0 space-y-0.5">
              <span className="block text-sm font-medium">
                {t(`networkSharesPage.dialog.${key}.title`)}
              </span>
              <span className="block text-xs text-muted-foreground">
                {t(`networkSharesPage.dialog.${key}.body`)}
              </span>
            </span>
          </button>
        )
      })}
    </div>
  )
}
