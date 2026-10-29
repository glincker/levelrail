import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ChevronDownGlassIcon,
  ChevronRightGlassIcon,
  DownloadGlassIcon,
  FileCodeGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { CodeBlock } from '@/components/ui/code-block'

function downloadYaml(compose: string, filename: string) {
  const url = URL.createObjectURL(new Blob([compose], { type: 'text/yaml' }))
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

/** Read-only preview of the compose file the template deploys, with copy and download. */
export function TemplateStackFile({
  compose,
  slug,
}: {
  compose: string
  slug: string
}) {
  const { t } = useTranslation('templates')
  const [open, setOpen] = useState(false)
  if (compose.trim() === '') return null
  const Caret = open ? ChevronDownGlassIcon : ChevronRightGlassIcon
  return (
    <section
      aria-labelledby="template-stackfile"
      className="rounded-xl border border-border bg-card p-3 sm:p-4"
    >
      <div className="flex flex-wrap items-center gap-2">
        <h2 id="template-stackfile" className="text-base font-semibold">
          <button
            type="button"
            aria-expanded={open}
            aria-controls="template-stackfile-body"
            onClick={() => {
              setOpen((v) => !v)
            }}
            className="inline-flex items-center gap-1.5 rounded-sm focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
          >
            <Caret className="size-4" aria-hidden="true" />
            <FileCodeGlassIcon className="size-4.5" aria-hidden="true" />
            {t('stackFile.title')}
          </button>
        </h2>
        <span className="text-xs text-muted-foreground">
          {t('stackFile.preview')}
        </span>
        {open ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="ml-auto"
            onClick={() => {
              downloadYaml(compose, `${slug}.compose.yaml`)
            }}
          >
            <DownloadGlassIcon size="1.2em" aria-hidden="true" />
            {t('stackFile.download')}
          </Button>
        ) : null}
      </div>
      {open ? (
        <div id="template-stackfile-body" className="mt-3">
          <CodeBlock
            code={compose}
            language="yaml"
            label={t('stackFile.label')}
            maxHeight="lg"
          />
        </div>
      ) : null}
    </section>
  )
}
