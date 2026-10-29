import { useState } from 'react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  CopyGlassIcon,
  DownloadGlassIcon,
  KeyGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { CodeBlock } from '@/components/ui/code-block'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useBrand } from '../../../hooks/useBrand'
import { useCopyToClipboard } from '../../../hooks/useCopyToClipboard'
import { useDebouncedValue } from '../../../hooks/useDebouncedValue'
import {
  stackTemplateQueryOptions,
  stackTemplateRenderQueryOptions,
} from '../../../queries/stackTemplates'
import type { StackTemplateDetail } from '../../../types/stackTemplates'
import {
  downloadStackTemplateFiles,
  joinStackTemplateFiles,
  stackTemplateCommands,
} from './stackTemplateCommands'

const RENDER_DEBOUNCE_MS = 300

function languageOf(name: string): string {
  return name.endsWith('.md') ? 'markdown' : 'yaml'
}

function DrawerBody({ template }: Readonly<{ template: StackTemplateDetail }>) {
  const { t } = useTranslation('stacks')
  const brand = useBrand()
  const { copied, copy } = useCopyToClipboard()
  const [values, setValues] = useState<Record<string, string>>(() =>
    Object.fromEntries(template.variables.map((v) => [v.name, v.default])),
  )
  // Empty fields fall back to the template default on the server.
  const sent = useDebouncedValue(
    Object.fromEntries(Object.entries(values).filter(([, v]) => v.trim())),
    RENDER_DEBOUNCE_MS,
  )
  const { data: rendered } = useQuery({
    ...stackTemplateRenderQueryOptions(template.id, sent),
    placeholderData: keepPreviousData,
  })
  const files = rendered?.files ?? template.files
  const issues = rendered?.issues ?? []
  const effective = {
    ...Object.fromEntries(template.variables.map((v) => [v.name, v.default])),
    ...sent,
  }

  return (
    <div className="flex flex-col gap-4 overflow-y-auto px-4 pb-4">
      <form
        className="grid gap-2 sm:grid-cols-3"
        onSubmit={(e) => {
          e.preventDefault()
        }}
      >
        {template.variables.map((v) => (
          <Field key={v.name}>
            <FieldLabel htmlFor={`tpl-var-${v.name}`}>
              {t(`templates.var.${v.name}`, { defaultValue: v.name })}
            </FieldLabel>
            <Input
              id={`tpl-var-${v.name}`}
              className="font-mono"
              value={values[v.name] ?? ''}
              placeholder={v.default}
              autoComplete="off"
              spellCheck={false}
              onChange={(e) => {
                setValues((prev) => ({ ...prev, [v.name]: e.target.value }))
              }}
            />
          </Field>
        ))}
      </form>

      {issues.length > 0 ? (
        <Alert variant="destructive">
          <TriangleAlertGlassIcon
            className="text-tone-warning"
            size="1.7em"
            aria-hidden="true"
          />
          <AlertDescription>
            <p className="font-medium">{t('templates.issues')}</p>
            <ul className="list-disc pl-4">
              {issues.map((i) => (
                <li key={`${i.file ?? ''}:${String(i.line)}:${i.message}`}>
                  {i.file ? `${i.file}: ` : ''}
                  {i.message}
                </li>
              ))}
            </ul>
          </AlertDescription>
        </Alert>
      ) : null}

      <section aria-labelledby="tpl-files" className="flex flex-col gap-2">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h3 id="tpl-files" className="text-sm font-medium">
            {t('templates.files')}
          </h3>
          <div className="flex gap-1">
            <Button
              type="button"
              size="xs"
              variant="outline"
              onClick={() => {
                copy(joinStackTemplateFiles(files))
              }}
            >
              {copied ? (
                <CheckGlassIcon size="1.4em" aria-hidden="true" />
              ) : (
                <CopyGlassIcon size="1.4em" aria-hidden="true" />
              )}
              {copied ? t('templates.copied') : t('templates.copyAll')}
            </Button>
            <Button
              type="button"
              size="xs"
              variant="outline"
              onClick={() => {
                downloadStackTemplateFiles(files)
              }}
            >
              <DownloadGlassIcon size="1.4em" aria-hidden="true" />
              {t('templates.downloadAll')}
            </Button>
          </div>
        </div>
        <Tabs
          key={files.map((f) => f.name).join('|')}
          defaultValue={files[0]?.name}
        >
          <TabsList>
            {files.map((f) => (
              <TabsTrigger key={f.name} value={f.name} className="font-mono">
                {f.name}
              </TabsTrigger>
            ))}
          </TabsList>
          {files.map((f) => (
            <TabsContent key={f.name} value={f.name}>
              <CodeBlock
                code={f.content}
                language={languageOf(f.name)}
                label={f.name}
                maxHeight="lg"
              />
            </TabsContent>
          ))}
        </Tabs>
      </section>

      <section aria-labelledby="tpl-secrets" className="flex flex-col gap-1">
        <h3 id="tpl-secrets" className="text-sm font-medium">
          {t('templates.secrets.title')}
        </h3>
        {template.secret_names.length === 0 ? (
          <p className="text-xs text-muted-foreground">
            {t('templates.secrets.none')}
          </p>
        ) : (
          <>
            <p className="text-xs text-muted-foreground">
              {t('templates.secrets.hint')}
            </p>
            <ul className="flex flex-wrap gap-1">
              {template.secret_names.map((n) => (
                <li key={n}>
                  <Badge variant="warning" className="font-mono">
                    <KeyGlassIcon size="1.4em" aria-hidden="true" />
                    {n}
                  </Badge>
                </li>
              ))}
            </ul>
          </>
        )}
      </section>

      <section aria-labelledby="tpl-cli" className="flex flex-col gap-1">
        <h3 id="tpl-cli" className="text-sm font-medium">
          {t('templates.cli.title')}
        </h3>
        <p className="text-xs text-muted-foreground">
          {t('templates.cli.hint')}
        </p>
        <CodeBlock
          code={stackTemplateCommands(
            brand.BinaryName,
            template.id,
            effective,
            template.secret_names,
          )}
          language="bash"
          label={t('templates.cli.title')}
          prompt
          wrap
        />
      </section>
    </div>
  )
}

export function StackTemplateDrawer({
  id,
  onClose,
}: Readonly<{ id: string | null; onClose: () => void }>) {
  const { t } = useTranslation('stacks')
  const { data, isError } = useQuery({
    ...stackTemplateQueryOptions(id ?? ''),
    enabled: id !== null,
  })
  return (
    <Sheet
      open={id !== null}
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      <SheetContent className="data-[side=right]:sm:max-w-3xl">
        <SheetHeader>
          <SheetTitle className="font-mono">{id ?? ''}</SheetTitle>
          <SheetDescription>
            {data?.description ?? t('templates.drawerDescription')}
          </SheetDescription>
        </SheetHeader>
        {isError ? (
          <p className="px-4 text-sm text-destructive">
            {t('templates.loadFailed')}
          </p>
        ) : data ? (
          <DrawerBody key={data.id} template={data} />
        ) : (
          <div className="px-4">
            <Skeleton className="h-40 w-full" />
          </div>
        )}
      </SheetContent>
    </Sheet>
  )
}
