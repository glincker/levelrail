import { useContext, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { FlaskConicalGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { useDemoInfo } from '@/hooks/useDemo'
import { BrandContext } from '@/lib/brandContext'
import { openDemoInstall } from '@/lib/installPrompt'

/** DemoSampleFrame labels canned output as a sample and always offers the install call to action. */
export function DemoSampleFrame({
  hint,
  children,
}: {
  hint: string
  children: ReactNode
}) {
  const { t } = useTranslation('demo')
  const info = useDemoInfo()
  const brand = useContext(BrandContext)
  const brandName = brand?.ShortName || brand?.Name || 'Levelrail'
  return (
    <section
      aria-label={t('sample.badge')}
      className="flex min-h-0 flex-1 flex-col gap-3"
    >
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-md border border-border bg-muted px-3 py-2 text-xs text-muted-foreground">
        <span className="inline-flex items-center gap-1 rounded-sm bg-foreground px-1.5 py-0.5 font-medium text-background">
          <FlaskConicalGlassIcon size="1.2em" aria-hidden="true" />
          {t('sample.badge')}
        </span>
        <span className="min-w-0 flex-1">{hint}</span>
        <Button
          type="button"
          size="xs"
          variant="outline"
          onClick={() => {
            openDemoInstall(info.docsUrl)
          }}
        >
          {t('installState.install', { brand: brandName })}
        </Button>
      </div>
      {children}
    </section>
  )
}
