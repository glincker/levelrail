import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

export function CliFallback({ children }: { children: ReactNode }) {
  const { t } = useTranslation('updates')
  return (
    <details className="text-sm">
      <summary className="cursor-pointer text-xs text-muted-foreground underline-offset-2 hover:underline">
        {t('cli.summary')}
      </summary>
      <div className="mt-3 space-y-3">{children}</div>
    </details>
  )
}
