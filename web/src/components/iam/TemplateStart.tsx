import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { MagicWandIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Field, FieldLabel } from '@/components/ui/field'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  fetchPolicyTemplates,
  policyTemplateKeys,
} from '../../queries/iamPolicyTemplates'
import { renderTemplate } from '../../queries/iamBuilder'
import type { IamResources, RenderedTemplate } from '../../queries/iamBuilder'

const BLANK = '__blank'

function optionsFor(kind: string | undefined, res: IamResources) {
  switch (kind) {
    case 'app':
      return res.apps.map((a) => ({ value: a.name, label: a.name }))
    case 'database':
      return res.databases.map((d) => ({ value: d.name, label: d.name }))
    case 'project':
      return res.projects.map((p) => ({ value: p.id, label: p.name }))
    default:
      return res.environments.map((e) => ({
        value: e.id,
        label: `${e.name} (${e.kind})`,
      }))
  }
}

/** TemplateStart fills the builder from a ready-made policy; each parameter is picked from what exists instead of typed. */
export function TemplateStart({
  resources,
  onApply,
}: {
  resources: IamResources
  onApply: (rendered: RenderedTemplate) => void
}) {
  const { t } = useTranslation('iam')
  const [id, setId] = useState(BLANK)
  const [params, setParams] = useState<Record<string, string>>({})
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const list = useQuery({
    queryKey: policyTemplateKeys.all,
    queryFn: fetchPolicyTemplates,
  })
  const templates = list.data?.templates ?? []
  const current = templates.find((tpl) => tpl.id === id)
  const missing = (current?.params ?? []).some(
    (p) => p.required && !params[p.name],
  )

  const apply = async () => {
    if (!current) return
    setBusy(true)
    setError(null)
    try {
      onApply(await renderTemplate(current.id, params))
    } catch (e) {
      setError(e instanceof Error ? e.message : t('builder.template.failed'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <section
      aria-label={t('builder.template.title')}
      className="space-y-3 rounded-xl border border-border bg-muted/30 p-4"
    >
      <h3 className="flex items-center gap-2 text-sm font-medium text-foreground">
        <MagicWandIcon className="size-4 text-muted-foreground" />
        {t('builder.template.title')}
      </h3>
      <Field>
        <FieldLabel htmlFor="iam-template">
          {t('builder.template.pick')}
        </FieldLabel>
        <Select
          value={id}
          onValueChange={(v) => {
            setId(String(v))
            setParams({})
          }}
        >
          <SelectTrigger id="iam-template" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={BLANK}>{t('builder.template.none')}</SelectItem>
            {templates.map((tpl) => (
              <SelectItem key={tpl.id} value={tpl.id}>
                {tpl.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Field>
      {current ? (
        <>
          <p className="text-xs text-muted-foreground">{current.description}</p>
          {current.params.map((p) => (
            <Field key={p.name}>
              <FieldLabel>
                {t(`builder.template.param.${p.kind ?? 'environment'}`)}
              </FieldLabel>
              <Combobox
                options={optionsFor(p.kind, resources)}
                value={params[p.name] ?? ''}
                onValueChange={(v) => setParams((s) => ({ ...s, [p.name]: v }))}
                placeholder={t('builder.resources.value')}
                searchPlaceholder={t('builder.resources.search')}
                emptyMessage={t('builder.resources.noOptions')}
                triggerClassName="w-full"
              />
            </Field>
          ))}
          {error ? (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
          <Button
            type="button"
            size="sm"
            onClick={() => void apply()}
            disabled={busy || missing}
          >
            {t('builder.template.use')}
          </Button>
        </>
      ) : null}
    </section>
  )
}
