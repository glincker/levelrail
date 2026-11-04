import { useState } from 'react'
import { useTranslation } from 'react-i18next'
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
import { useExportIac } from '../queries/iac'
import { IacExportView } from './IacExportView'

// Reads live state and renders it as resource files. Secret values are never
// in the output: secret env vars come back as secretRef.
export function IacExportCard() {
  const { t } = useTranslation('settings')
  const exportIac = useExportIac()
  const [project, setProject] = useState('')
  const [app, setApp] = useState('')
  const [includeValues, setIncludeValues] = useState(true)

  const result = exportIac.data

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('iacExport.title')}</CardTitle>
        <CardDescription>{t('iacExport.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <Field>
            <FieldLabel htmlFor="iac-export-project">
              {t('iacExport.project')}
            </FieldLabel>
            <Input
              id="iac-export-project"
              value={project}
              onChange={(e) => setProject(e.target.value)}
              placeholder={t('iacExport.projectPlaceholder')}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="iac-export-app">
              {t('iacExport.app')}
            </FieldLabel>
            <Input
              id="iac-export-app"
              value={app}
              onChange={(e) => setApp(e.target.value)}
              placeholder={t('iacExport.appPlaceholder')}
            />
          </Field>
        </div>
        <label className="flex items-center gap-2 text-sm text-foreground">
          <Checkbox
            checked={includeValues}
            onCheckedChange={(v) => setIncludeValues(v === true)}
          />
          {t('iacExport.includeValues')}
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
            {exportIac.isPending ? t('iacExport.running') : t('iacExport.run')}
          </Button>
        </div>
        {exportIac.error ? (
          <p className="text-sm text-red-700 dark:text-red-400">
            {exportIac.error.message}
          </p>
        ) : null}
        {result ? (
          <IacExportView
            result={result}
            fileName={app || project || 'resources'}
          />
        ) : null}
      </CardContent>
    </Card>
  )
}
