import { useState } from 'react'
import { DownloadSimpleIcon } from '@phosphor-icons/react/dist/ssr'
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
  const exportIac = useExportIac()
  const [project, setProject] = useState('')
  const [app, setApp] = useState('')
  const [includeValues, setIncludeValues] = useState(true)

  const result = exportIac.data
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
              <DownloadSimpleIcon className="size-4" />
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
          />
        ) : null}
      </CardContent>
    </Card>
  )
}
