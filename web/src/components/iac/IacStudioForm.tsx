import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { IacSchemaForm } from './IacSchemaForm'
import type { IacSchema } from './schemaToFields'
import { readDocument, YamlParseError } from './documentYaml'
import {
  firstDocument,
  replaceFirstDocument,
  splitStudioDocs,
} from './studioDocs'

function parseLine(doc: string): number | null {
  try {
    readDocument(doc)
    return null
  } catch (e) {
    return e instanceof YamlParseError ? e.line : 1
  }
}

// Lazy chunk: the schema JSON and the form stay out of the Studio's first load.
export default function IacStudioForm({
  text,
  onChange,
  onEditYaml,
}: Readonly<{
  text: string
  onChange: (text: string) => void
  onEditYaml: () => void
}>) {
  const { t } = useTranslation('settings')
  const [schema, setSchema] = useState<IacSchema | null>(null)
  useEffect(() => {
    void import('../../../../docs/schemas/resources.schema.json').then((m) =>
      setSchema(m.default as unknown as IacSchema),
    )
  }, [])

  const first = firstDocument(text)
  const multi = splitStudioDocs(text).count > 1
  const line = parseLine(first)

  if (!schema) {
    return (
      <p role="status" className="text-sm text-muted-foreground">
        {t('iacStudio.formLoading')}
      </p>
    )
  }
  return (
    <div className="space-y-2">
      {multi ? (
        <p role="status" className="text-xs text-muted-foreground">
          {t('iacStudio.multiDoc')}
        </p>
      ) : null}
      {line !== null ? (
        <div role="alert" className="flex flex-wrap items-center gap-2 text-sm">
          <span className="text-tone-danger">
            {t('iacStudio.parseLine', { line })}
          </span>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={onEditYaml}
          >
            {t('iacStudio.editAsYaml')}
          </Button>
        </div>
      ) : null}
      <fieldset
        disabled={multi || line !== null}
        className="min-w-0 border-0 p-0"
      >
        <IacSchemaForm
          schema={schema}
          value={first}
          onChange={(y) => onChange(replaceFirstDocument(text, y))}
        />
      </fieldset>
    </div>
  )
}
