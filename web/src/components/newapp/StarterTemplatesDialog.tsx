import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CopyIcon,
  RocketLaunchIcon,
  TerminalWindowIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { CommandBlock } from '@/components/ui/code-block'
import { toast } from '@/components/ui/toast'
import {
  ProjectTemplateValidationError,
  useProjectTemplates,
  useRenderProjectTemplate,
  type ProjectTemplate,
  type ProjectTemplateFile,
} from '../../queries/projectTemplates'

function defaultsFor(t: ProjectTemplate): Record<string, string> {
  return Object.fromEntries(t.params.map((p) => [p.name, p.default ?? '']))
}

/**
 * Starter project templates: pick one, fill its few parameters, and get the
 * validated app.yaml (plus a pipeline file for the build and test starter) to
 * commit to the repository. Nothing is created on the server.
 */
export function StarterTemplatesDialog({
  variant = 'button',
}: {
  /** `row` renders the trigger as a compact list row for the Other ways card. */
  variant?: 'button' | 'row'
}) {
  const { t } = useTranslation('newApp')
  const templates = useProjectTemplates()
  const render = useRenderProjectTemplate()
  const [selected, setSelected] = useState<ProjectTemplate | null>(null)
  const [values, setValues] = useState<Record<string, string>>({})
  const [files, setFiles] = useState<ProjectTemplateFile[] | null>(null)

  const fieldErrors =
    render.error instanceof ProjectTemplateValidationError
      ? Object.fromEntries(render.error.fields.map((f) => [f.param, f.message]))
      : {}

  function choose(template: ProjectTemplate) {
    setSelected(template)
    setValues(defaultsFor(template))
    setFiles(null)
    render.reset()
  }

  function submit() {
    if (!selected) return
    render.mutate({ id: selected.id, params: values }, { onSuccess: setFiles })
  }

  function copy(file: ProjectTemplateFile) {
    void navigator.clipboard.writeText(file.content).then(() => {
      toast.add({
        title: t('starter.copied', { path: file.path }),
        type: 'success',
      })
    })
  }

  const initCommand = selected
    ? `init --template ${selected.id}${Object.entries(values)
        .filter(([, v]) => v !== '')
        .map(([k, v]) => ` --param ${k}=${v}`)
        .join('')}`
    : ''

  return (
    <Dialog>
      {variant === 'row' ? (
        <DialogTrigger
          render={
            <button
              type="button"
              className="flex w-full items-start gap-2.5 rounded-lg border border-border p-2 text-left transition-colors hover:bg-muted/50 focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
            />
          }
        >
          <RocketLaunchIcon
            className="mt-0.5 size-5 shrink-0 text-muted-foreground"
            aria-hidden="true"
          />
          <span className="min-w-0">
            <span className="block text-sm font-medium text-foreground">
              {t('starter.open')}
            </span>
            <span className="block text-xs text-muted-foreground">
              {t('starter.openHint')}
            </span>
          </span>
        </DialogTrigger>
      ) : (
        <DialogTrigger render={<Button type="button" variant="outline" />}>
          <RocketLaunchIcon aria-hidden="true" />
          {t('starter.open')}
        </DialogTrigger>
      )}
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('starter.title')}</DialogTitle>
          <DialogDescription>{t('starter.description')}</DialogDescription>
        </DialogHeader>

        {templates.isError ? (
          <p className="text-sm text-tone-danger">{templates.error.message}</p>
        ) : (
          <ul className="grid gap-2 sm:grid-cols-2">
            {(templates.data ?? []).map((tpl) => (
              <li key={tpl.id}>
                <Button
                  type="button"
                  variant={selected?.id === tpl.id ? 'default' : 'outline'}
                  className="h-auto w-full flex-col items-start gap-1 whitespace-normal p-3 text-left"
                  aria-pressed={selected?.id === tpl.id}
                  onClick={() => choose(tpl)}
                >
                  <span className="font-medium">{tpl.title}</span>
                  <span className="text-xs font-normal opacity-80">
                    {tpl.summary}
                  </span>
                </Button>
              </li>
            ))}
          </ul>
        )}

        {selected ? (
          <form
            className="space-y-3"
            onSubmit={(e) => {
              e.preventDefault()
              submit()
            }}
          >
            {selected.params.map((p) => (
              <Field key={p.name}>
                <FieldLabel htmlFor={`starter-${p.name}`}>{p.label}</FieldLabel>
                {p.kind === 'enum' ? (
                  <Select
                    value={values[p.name] ?? ''}
                    onValueChange={(v) =>
                      setValues((prev) => ({ ...prev, [p.name]: v ?? '' }))
                    }
                  >
                    <SelectTrigger id={`starter-${p.name}`} className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {(p.options ?? []).map((o) => (
                        <SelectItem key={o} value={o}>
                          {o}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                ) : (
                  <Input
                    id={`starter-${p.name}`}
                    value={values[p.name] ?? ''}
                    placeholder={p.placeholder}
                    aria-invalid={Boolean(fieldErrors[p.name])}
                    onChange={(e) =>
                      setValues((prev) => ({
                        ...prev,
                        [p.name]: e.target.value,
                      }))
                    }
                  />
                )}
                {fieldErrors[p.name] ? (
                  <FieldError>{fieldErrors[p.name]}</FieldError>
                ) : null}
              </Field>
            ))}
            {render.error &&
            !(render.error instanceof ProjectTemplateValidationError) ? (
              <p className="text-sm text-tone-danger">{render.error.message}</p>
            ) : null}
            <Button type="submit" disabled={render.isPending}>
              {render.isPending
                ? t('starter.generating')
                : t('starter.generate')}
            </Button>
          </form>
        ) : null}

        {files ? (
          <div className="space-y-3">
            {files.map((f) => (
              <section key={f.path} className="rounded-lg border border-border">
                <div className="flex items-center justify-between gap-2 border-b border-border px-3 py-2">
                  <p className="font-mono text-xs text-foreground">{f.path}</p>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => copy(f)}
                  >
                    <CopyIcon aria-hidden="true" />
                    {t('starter.copy')}
                  </Button>
                </div>
                <pre className="overflow-x-auto p-3 font-mono text-xs text-foreground">
                  {f.content}
                </pre>
              </section>
            ))}
            <p className="text-sm text-muted-foreground">{t('starter.next')}</p>
            <Collapsible>
              <CollapsibleTrigger
                render={<Button type="button" variant="ghost" size="sm" />}
              >
                <TerminalWindowIcon aria-hidden="true" />
                {t('starter.viaCli')}
              </CollapsibleTrigger>
              <CollapsiblePanel>
                <CommandBlock className="mt-2" code={initCommand} />
              </CollapsiblePanel>
            </Collapsible>
          </div>
        ) : null}
      </DialogContent>
    </Dialog>
  )
}
