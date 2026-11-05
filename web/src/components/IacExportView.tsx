import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ClipboardCopyGlassIcon, DownloadGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import type { IacExportResult } from '../queries/iac'
import { resolveFieldHelp, yamlPathAtLine } from '@/lib/schemaHelp'
import type { FieldHelp } from '@/lib/schemaHelp'

type SchemaRoot = Parameters<typeof resolveFieldHelp>[0]

function joinFiles(res: IacExportResult): string {
  return res.files.map((f) => f.content).join('---\n')
}

function download(text: string, name: string) {
  const url = URL.createObjectURL(new Blob([text], { type: 'text/yaml' }))
  const a = document.createElement('a')
  a.href = url
  a.download = name
  a.click()
  URL.revokeObjectURL(url)
}

interface IacExportViewProps {
  result: IacExportResult
  fileName: string
}

// Presentational export output: text, copy, download, warnings and schema help.
export function IacExportView({ result, fileName }: IacExportViewProps) {
  const { t } = useTranslation('settings')
  const [schema, setSchema] = useState<SchemaRoot | null>(null)
  const [help, setHelp] = useState<(FieldHelp & { key: string }) | null>(null)
  const [copied, setCopied] = useState(false)
  const text = joinFiles(result)
  const hasFiles = result.files.length > 0

  // Lazy so the schema stays out of the page chunk until an export exists.
  useEffect(() => {
    if (schema) return
    void import('../../../docs/schemas/resources.schema.json').then((m) =>
      setSchema(m.default as SchemaRoot),
    )
  }, [schema])

  function showHelp(el: HTMLTextAreaElement) {
    if (!schema) return
    const lines = el.value.split('\n')
    const line = el.value.slice(0, el.selectionStart).split('\n').length - 1
    const { kind, path } = yamlPathAtLine(lines, line)
    const found = path.length ? resolveFieldHelp(schema, kind, path) : null
    setHelp(found ? { ...found, key: path[path.length - 1] ?? '' } : null)
  }

  async function copy() {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    } catch {
      setCopied(false)
    }
  }

  return (
    <div className="space-y-2">
      {hasFiles ? (
        <div className="flex gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => void copy()}
          >
            <ClipboardCopyGlassIcon className="size-4" />
            {copied ? t('iacExport.copied') : t('iacExport.copy')}
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => download(text, `${fileName}.yaml`)}
          >
            <DownloadGlassIcon className="size-4" />
            {t('iacExport.download')}
          </Button>
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">{t('iacExport.empty')}</p>
      )}
      {result.warnings?.map((w) => (
        <p key={w} className="text-xs text-amber-700 dark:text-amber-400">
          {t('iacExport.warning', { message: w })}
        </p>
      ))}
      {hasFiles ? (
        <Textarea
          readOnly
          aria-label={t('iacExport.textLabel')}
          className="min-h-48 font-mono text-xs"
          value={text}
          onClick={(e) => showHelp(e.currentTarget)}
          onKeyUp={(e) => showHelp(e.currentTarget)}
        />
      ) : null}
      {hasFiles ? (
        <p className="text-xs text-muted-foreground" aria-live="polite">
          {help ? (
            <>
              <code className="font-mono">{help.key}</code>: {help.description}
              {help.defaultValue
                ? ` ${t('iacHelp.default', { value: help.defaultValue })}`
                : ''}
            </>
          ) : (
            t('iacHelp.hint')
          )}
        </p>
      ) : null}
    </div>
  )
}
