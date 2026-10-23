import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ProviderMark } from './ProviderMark'
import {
  EMAIL_PROVIDER_PRESETS,
  type EmailProviderPreset,
} from './emailProviderPresets'
import { PRESS } from './emailStyles'

interface Props {
  onPick: (preset: EmailProviderPreset) => void
  onCancel?: () => void
}

/** EmailProviderPicker lists provider templates; choosing one opens the edit panel already filled. */
export function EmailProviderPicker({ onPick, onCancel }: Readonly<Props>) {
  const { t } = useTranslation('settings')
  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center gap-2">
        <p className="min-w-0 flex-1 text-xs text-muted-foreground">
          {t('emailPresets.intro')}
        </p>
        {onCancel ? (
          <Button
            type="button"
            variant="ghost"
            size="xs"
            className={PRESS}
            onClick={onCancel}
          >
            {t('emailProviders.panel.cancel')}
          </Button>
        ) : null}
      </div>
      <ul className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-5">
        {EMAIL_PROVIDER_PRESETS.map((preset) => {
          const name = t(`emailPresets.items.${preset.id}.name`)
          return (
            <li key={preset.id}>
              <button
                type="button"
                onClick={() => {
                  onPick(preset)
                }}
                className={`flex h-full w-full items-start gap-2 rounded-lg border border-border bg-background/40 p-2 text-left transition-colors hover:border-primary hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none ${PRESS}`}
              >
                <ProviderMark
                  logoId={preset.logoId}
                  label={name}
                  generic={preset.id === 'smtp'}
                />
                <span className="min-w-0">
                  <span className="block text-sm font-medium">{name}</span>
                  <span className="block text-xs text-muted-foreground">
                    {t(`emailPresets.items.${preset.id}.bestFor`)}
                  </span>
                </span>
              </button>
            </li>
          )
        })}
      </ul>
    </div>
  )
}
