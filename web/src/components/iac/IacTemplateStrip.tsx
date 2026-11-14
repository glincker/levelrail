import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { CopyGlassIcon, FileCodeGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { joinTemplateFiles } from './iacTemplates'
import {
  stackTemplateRenderQueryOptions,
  stackTemplatesQueryOptions,
} from '../../queries/stackTemplates'

const MAX_CARDS = 4

export function IacTemplateStrip({
  onUse,
}: Readonly<{ onUse: (yaml: string) => void }>) {
  const { t } = useTranslation('settings')
  const queryClient = useQueryClient()
  const { data } = useQuery(stackTemplatesQueryOptions())
  const [note, setNote] = useState('')
  if (!data || data.length === 0) return null

  async function render(id: string): Promise<string> {
    const out = await queryClient.fetchQuery(
      stackTemplateRenderQueryOptions(id, {}),
    )
    return joinTemplateFiles(out.files)
  }

  async function copy(id: string) {
    try {
      await navigator.clipboard.writeText(await render(id))
      setNote(t('iacStudio.copied'))
    } catch {
      setNote(t('iacStudio.copyFailed'))
    }
  }

  return (
    <section aria-label={t('iacStudio.templates')} className="space-y-2">
      <h3 className="text-xs font-medium text-muted-foreground">
        {t('iacStudio.templates')}
      </h3>
      <p role="status" className="sr-only">
        {note}
      </p>
      <ul className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
        {data.slice(0, MAX_CARDS).map((tpl) => (
          <li
            key={tpl.id}
            className="flex flex-col gap-2 rounded-xl border bg-card p-3"
          >
            <span className="font-mono text-sm font-medium break-words">
              {tpl.title}
            </span>
            <span className="line-clamp-2 text-xs text-muted-foreground">
              {tpl.description}
            </span>
            <span className="flex flex-wrap gap-1">
              {tpl.resource_kinds.map((k) => (
                <Badge key={k} variant="muted">
                  {k}
                </Badge>
              ))}
            </span>
            <span className="mt-auto flex gap-2 pt-1">
              <Button
                type="button"
                size="sm"
                onClick={() =>
                  void render(tpl.id)
                    .then(onUse)
                    .catch(() => setNote(t('iacStudio.copyFailed')))
                }
                aria-label={t('iacStudio.useNamed', { name: tpl.title })}
              >
                <FileCodeGlassIcon size="1.4em" aria-hidden="true" />
                {t('iacStudio.use')}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => void copy(tpl.id)}
                aria-label={t('iacStudio.copyNamed', { name: tpl.title })}
              >
                <CopyGlassIcon size="1.4em" aria-hidden="true" />
                {t('iacStudio.copy')}
              </Button>
            </span>
          </li>
        ))}
      </ul>
    </section>
  )
}
