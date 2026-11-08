import { useEffect, useState } from 'react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { SegmentedControl } from '@/components/ui/segmented-control'
import { Button } from '@/components/ui/button'
import { CodeBlock } from '@/components/ui/code-block'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { previewMailTemplate, type MailPreviewDraft } from '../../queries/mail'

const DEBOUNCE_MS = 400

function useDebounced<T>(value: T, ms: number): T {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const id = setTimeout(() => setDebounced(value), ms)
    return () => clearTimeout(id)
  }, [value, ms])
  return debounced
}

/** TemplatePreview renders the draft with sample data at desktop or mobile width, light or dark. */
export function TemplatePreview({
  eventKey,
  draft,
}: {
  eventKey: string
  draft: MailPreviewDraft
}) {
  const { t } = useTranslation('mail')
  const [mobile, setMobile] = useState(false)
  const [dark, setDark] = useState(false)
  const [showPlain, setShowPlain] = useState(false)
  const settled = useDebounced(draft, DEBOUNCE_MS)
  const scheme = dark ? 'dark' : 'light'

  const preview = useQuery({
    queryKey: ['mail', 'preview', eventKey, settled, scheme],
    queryFn: () => previewMailTemplate(eventKey, { ...settled, scheme }),
    placeholderData: keepPreviousData,
    retry: false,
  })

  return (
    <section aria-label={t('templates.preview.title')} className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-sm font-medium">{t('templates.preview.title')}</h3>
        <div className="flex flex-wrap gap-2">
          <SegmentedControl
            aria-label={t('templates.preview.deviceLabel')}
            value={mobile ? 'mobile' : 'desktop'}
            onValueChange={(v) => setMobile(v === 'mobile')}
            options={[
              { value: 'desktop', label: t('templates.preview.desktop') },
              { value: 'mobile', label: t('templates.preview.mobile') },
            ]}
          />
          <SegmentedControl
            aria-label={t('templates.preview.schemeLabel')}
            value={dark ? 'dark' : 'light'}
            onValueChange={(v) => setDark(v === 'dark')}
            options={[
              { value: 'light', label: t('templates.preview.light') },
              { value: 'dark', label: t('templates.preview.dark') },
            ]}
          />
        </div>
      </div>
      {preview.isError ? (
        <Alert variant="destructive">
          <AlertDescription>
            {t('templates.preview.error', { message: preview.error.message })}
          </AlertDescription>
        </Alert>
      ) : null}
      {preview.data ? (
        <div className="overflow-x-auto rounded-lg border border-border bg-muted/40 p-3">
          <iframe
            sandbox=""
            title={t('templates.preview.frameTitle')}
            srcDoc={preview.data.html}
            className={cn(
              'mx-auto block h-[560px] rounded-md border-0 bg-white',
              mobile ? 'w-full max-w-[375px]' : 'w-full min-w-[620px]',
            )}
          />
        </div>
      ) : (
        <Skeleton className="h-[560px] w-full" />
      )}
      {preview.data ? (
        <div className="space-y-1.5">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            aria-expanded={showPlain}
            onClick={() => setShowPlain((v) => !v)}
          >
            {t('templates.preview.plain')}
          </Button>
          {showPlain ? (
            <CodeBlock code={preview.data.text} wrap maxHeight="sm" />
          ) : null}
        </div>
      ) : null}
    </section>
  )
}
