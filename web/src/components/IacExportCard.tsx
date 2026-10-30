import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { DownloadGlassIcon } from '@thesvg/ui-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { useExportIac } from '../queries/iac'
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

// Reads live state and renders it as resource files. Secret values are never
// in the output: secret env vars come back as secretRef.
export function IacExportCard() {
  const { t } = useTranslation('settings')
  const exportIac = useExportIac()
  const [schema, setSchema] = useState<SchemaRoot | null>(null)
  const [help, setHelp] = useState<(FieldHelp & { key: string }) | null>(null)
  const [project, setProject] = useState('')
  const [app, setApp] = useState('')
  const [includeValues, setIncludeValues] = useState(true)

  const result = exportIac.data

  // Lazy so the schema stays out of the page chunk until an export exists.
  useEffect(() => {
    if (!result || schema) return
    void import('../../../docs/schemas/resources.schema.json').then((m) =>
      setSchema(m.default as SchemaRoot),
    )
  }, [result, schema])

  function showHelp(el: HTMLTextAreaElement) {
    if (!schema) return
    const lines = el.value.split('\n')
    const line = el.value.slice(0, el.selectionStart).split('\n').length - 1
    const { kind, path } = yamlPathAtLine(lines, line)
    const found = path.length ? resolveFieldHelp(schema, kind, path) : null
    setHelp(found ? { ...found, key: path[path.length - 1] ?? '' } : null)
  }
  const text = result ? joinFiles(result) : ''
  const fileName = app || project || 'resources'

  return (
    <Card>
      <CardHeader>
        <CardTitle>Export</CardTitle>
        <CardDescription>
          Write live state as resource files. Leave both fields empty for
          everything you can read.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <Field>
            <FieldLabel htmlFor="iac-export-project">Project</FieldLabel>
            <Input
              id="iac-export-project"
              value={project}
              onChange={(e) => setProject(e.target.value)}
              placeholder="shop"
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="iac-export-app">App</FieldLabel>
            <Input
              id="iac-export-app"
              value={app}
              onChange={(e) => setApp(e.target.value)}
              placeholder="web"
            />
          </Field>
        </div>
        <label className="flex items-center gap-2 text-sm text-foreground">
          <Checkbox
            checked={includeValues}
            onCheckedChange={(v) => setIncludeValues(v === true)}
          />
          Include plain env values
        </label>
        <div className="flex gap-2">
          <Button
            type="button"
            disabled={exportIac.isPending}
            onClick={() =>
              exportIac.mutate({
                project: project.trim() || undefined,
                app: app.trim() || undefined,
                includeEnvValues: includeValues,
              })
            }
          >
            {exportIac.isPending ? 'Exporting...' : 'Export'}
          </Button>
          {result && result.files.length > 0 ? (
            <Button
              type="button"
              variant="outline"
              onClick={() => download(text, `${fileName}.yaml`)}
            >
              <DownloadGlassIcon className="size-5" />
              Download
            </Button>
          ) : null}
        </div>
        {exportIac.error ? (
          <p className="text-sm text-red-700 dark:text-red-400">
            {exportIac.error.message}
          </p>
        ) : null}
        {result?.warnings?.map((w) => (
          <p key={w} className="text-xs text-amber-700 dark:text-amber-400">
            warning: {w}
          </p>
        ))}
        {result ? (
          <Textarea
            readOnly
            aria-label="Exported resource files"
            className="min-h-48 font-mono text-xs"
            value={text}
            onClick={(e) => showHelp(e.currentTarget)}
            onKeyUp={(e) => showHelp(e.currentTarget)}
          />
        ) : null}
        {result ? (
          <p className="text-xs text-muted-foreground" aria-live="polite">
            {help ? (
              <>
                <code className="font-mono">{help.key}</code>:{' '}
                {help.description}
                {help.defaultValue
                  ? ` ${t('iacHelp.default', { value: help.defaultValue })}`
                  : ''}
              </>
            ) : (
              t('iacHelp.hint')
            )}
          </p>
        ) : null}
      </CardContent>
    </Card>
  )
}
