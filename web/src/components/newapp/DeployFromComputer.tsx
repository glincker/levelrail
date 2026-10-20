import { useTranslation } from 'react-i18next'
import { LaptopIcon } from '@phosphor-icons/react/dist/ssr'
import { useBrand } from '../../hooks/useBrand'
import { cliCommand } from '../../lib/cliCommand'
import { CliCommandBox } from './CliCommandBox'

/** New-app option for a directory with no git remote: shows the exact command with this control plane's URL. */
export function DeployFromComputer() {
  const { t } = useTranslation('newApp')
  const brand = useBrand()
  const command = cliCommand(
    brand.BinaryName,
    `deploy --api-url ${window.location.origin}`,
  )

  return (
    <section aria-labelledby="new-app-upload" className="space-y-3">
      <div className="flex items-start gap-2.5">
        <LaptopIcon
          className="mt-0.5 size-5 shrink-0 text-muted-foreground"
          aria-hidden="true"
        />
        <div>
          <h2
            id="new-app-upload"
            className="text-base font-medium text-foreground"
          >
            {t('upload.title')}
          </h2>
          <p className="text-sm text-muted-foreground">
            {t('upload.description')}
          </p>
        </div>
      </div>
      <CliCommandBox command={command} />
      <p className="text-xs text-muted-foreground">{t('upload.note')}</p>
    </section>
  )
}
