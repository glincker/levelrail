import { useTranslation } from 'react-i18next'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { cn } from '@/lib/utils'
import {
  PAGE_WIDTHS,
  setPageWidth,
  usePageWidth,
  type PageWidth,
} from '@/lib/pageWidth'

const PREVIEW_COLUMN: Record<PageWidth, string> = {
  comfortable: 'mx-auto w-3/5',
  wide: 'mx-auto w-4/5',
  full: 'w-full',
}

function Preview({ width }: { width: PageWidth }) {
  return (
    <span
      aria-hidden="true"
      className="flex h-14 w-full flex-col gap-1 rounded-md bg-muted p-1.5"
    >
      <span
        className={cn('h-2 rounded-sm bg-foreground/30', PREVIEW_COLUMN[width])}
      />
      <span
        className={cn(
          'flex-1 rounded-sm bg-foreground/15',
          PREVIEW_COLUMN[width],
        )}
      />
    </span>
  )
}

/** PageWidthCard picks how wide page content grows on large screens, stored in this browser only. */
export function PageWidthCard() {
  const { t } = useTranslation('settings')
  const value = usePageWidth()

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('generalPage.width.title')}</CardTitle>
        <CardDescription>{t('generalPage.width.description')}</CardDescription>
      </CardHeader>
      <CardContent>
        <div
          role="radiogroup"
          aria-label={t('generalPage.width.title')}
          className="grid gap-3 sm:grid-cols-3"
        >
          {PAGE_WIDTHS.map((option) => (
            <button
              key={option}
              type="button"
              role="radio"
              aria-checked={value === option}
              onClick={() => setPageWidth(option)}
              className={cn(
                'flex flex-col gap-2 rounded-lg p-2 text-left ring-1 ring-border transition-colors hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none',
                value === option && 'bg-muted/50 ring-2 ring-primary',
              )}
            >
              <Preview width={option} />
              <span className="px-1 pb-1">
                <span className="block text-sm font-medium">
                  {t(`generalPage.width.${option}`)}
                </span>
                <span className="block text-xs text-muted-foreground">
                  {t(`generalPage.width.${option}Hint`)}
                </span>
              </span>
            </button>
          ))}
        </div>
      </CardContent>
    </Card>
  )
}
