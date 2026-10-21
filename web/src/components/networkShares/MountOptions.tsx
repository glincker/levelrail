import { useTranslation } from 'react-i18next'
import { CaretDownIcon } from '@phosphor-icons/react/dist/ssr'
import { cn } from '@/lib/utils'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import type { NetworkShareProtocol } from '../../types/networkShare'
import { FieldHelp } from './Fields'
import { hasOption, toggleOption } from './logic'

const DEFAULT_ID = '1000'

type ChipKey = 'ro' | 'nfsvers4' | 'nfsvers3' | 'soft' | 'ids' | 'smbvers'

const CHIPS: Record<NetworkShareProtocol, { key: ChipKey; token: string }[]> = {
  nfs: [
    { key: 'ro', token: 'ro' },
    { key: 'nfsvers4', token: 'nfsvers=4' },
    { key: 'nfsvers3', token: 'nfsvers=3' },
    { key: 'soft', token: 'soft' },
  ],
  cifs: [
    { key: 'ro', token: 'ro' },
    { key: 'ids', token: `uid=${DEFAULT_ID},gid=${DEFAULT_ID}` },
    { key: 'smbvers', token: 'vers=3.0' },
  ],
}

export function MountOptions({
  protocol,
  value,
  onChange,
}: Readonly<{
  protocol: NetworkShareProtocol
  value: string
  onChange: (next: string) => void
}>) {
  const { t } = useTranslation('settings')
  return (
    <Collapsible>
      <CollapsibleTrigger className="flex items-center gap-1 text-sm font-medium">
        <CaretDownIcon aria-hidden="true" className="size-4" />
        {t('networkSharesPage.dialog.advanced')}
      </CollapsibleTrigger>
      <CollapsiblePanel>
        <div className="space-y-2 pt-3">
          <Label htmlFor="share-options">
            {t('networkSharesPage.dialog.options')}
            <span className="font-normal text-muted-foreground">
              {t('networkSharesPage.dialog.optionsOptional')}
            </span>
          </Label>
          <div
            role="group"
            aria-label={t('networkSharesPage.dialog.chips.label')}
            className="flex flex-wrap gap-1.5"
          >
            {CHIPS[protocol].map(({ key, token }) => {
              const on = hasOption(value, token)
              return (
                <button
                  key={key}
                  type="button"
                  aria-pressed={on}
                  onClick={() => {
                    onChange(toggleOption(value, token))
                  }}
                  className={cn(
                    'rounded-full border border-input px-2.5 py-1 text-xs transition-colors hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none',
                    on && 'border-primary bg-muted font-medium',
                  )}
                >
                  {t(`networkSharesPage.dialog.chips.${key}`)}
                </button>
              )
            })}
          </div>
          <Input
            id="share-options"
            className="font-mono"
            value={value}
            autoComplete="off"
            spellCheck={false}
            placeholder={t('networkSharesPage.dialog.optionsPlaceholder')}
            onChange={(e) => {
              onChange(e.target.value)
            }}
          />
          <FieldHelp>{t('networkSharesPage.dialog.optionsHint')}</FieldHelp>
        </div>
      </CollapsiblePanel>
    </Collapsible>
  )
}
