import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PlusIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  editsFromForm,
  isValidDomain,
  neededEnvKeys,
  validateAppName,
  type SimpleFormValues,
} from '../../../lib/repoImport'
import type { NewAppEdits } from '../../../lib/newApp'
import type { ImportAnalysis } from '../../../queries/importAnalyze'

const SECRET_KEY = /secret|token|password|passwd|key|credential/i
const BUILD_TYPES = ['railpack', 'dockerfile', 'static'] as const

interface ExtraVar {
  id: number
  key: string
  value: string
}

/** The short form: name, needed variables, address, and one Deploy button. */
export function RepoSimpleForm({
  analysis,
  branch,
  initialName,
  takenNames,
  deploying,
  failure,
  advancedOpen,
  onAdvancedOpen,
  onSubmit,
}: {
  analysis: ImportAnalysis
  branch: string
  initialName: string
  takenNames: ReadonlySet<string>
  deploying: boolean
  failure: string | null
  advancedOpen: boolean
  onAdvancedOpen: (open: boolean) => void
  onSubmit: (edits: NewAppEdits) => void
}) {
  const { t } = useTranslation('newApp')
  const needed = neededEnvKeys(analysis)
  const [name, setName] = useState(initialName)
  const [env, setEnv] = useState<Record<string, string>>({})
  const [extra, setExtra] = useState<ExtraVar[]>([])
  const [customOpen, setCustomOpen] = useState(false)
  const [customDomain, setCustomDomain] = useState('')
  const [advanced, setAdvanced] = useState<SimpleFormValues['advanced']>({})
  const [submitted, setSubmitted] = useState(false)

  const nameError = validateAppName(name, takenNames)
  const domainError =
    customOpen && customDomain.trim() !== '' && !isValidDomain(customDomain)
  const missing = needed.filter((k) => (env[k.key] ?? '').trim() === '')
  const valid = nameError === null && !domainError && missing.length === 0

  const builder = t(
    `import.builders.${advanced.buildType ?? analysis.build.type}`,
    {
      defaultValue: analysis.build.type,
    },
  )
  const target =
    customOpen && customDomain.trim() !== ''
      ? customDomain.trim()
      : t('import.form.freeAddress')

  function submit() {
    setSubmitted(true)
    if (!valid || deploying) return
    const allEnv = { ...env }
    for (const v of extra)
      if (v.key.trim() !== '') allEnv[v.key.trim()] = v.value
    onSubmit(
      editsFromForm(analysis, {
        name,
        env: allEnv,
        customDomain: customOpen ? customDomain : '',
        advanced,
      }),
    )
  }

  const num = (v: string) => {
    const n = Number(v)
    return v.trim() === '' || Number.isNaN(n) ? undefined : n
  }

  return (
    <form
      noValidate
      className="min-w-0 space-y-5"
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      <Field>
        <FieldLabel htmlFor="repo-app-name">{t('import.form.name')}</FieldLabel>
        <Input
          id="repo-app-name"
          value={name}
          autoComplete="off"
          spellCheck={false}
          aria-invalid={submitted && nameError !== null}
          aria-describedby="repo-app-name-msg"
          onChange={(e) => {
            setName(e.target.value.toLowerCase())
          }}
        />
        <FieldDescription id="repo-app-name-msg">
          {nameError !== null && (submitted || name !== '')
            ? t(`import.form.nameError.${nameError}`)
            : t('import.form.nameFree')}
        </FieldDescription>
      </Field>

      {needed.length > 0 || extra.length > 0 ? (
        <fieldset className="min-w-0 space-y-3">
          <legend className="text-sm font-medium text-foreground">
            {t('import.form.env')}
          </legend>
          {needed.map((k) => (
            <Field key={k.key}>
              <FieldLabel htmlFor={`repo-env-${k.key}`}>
                <code className="font-mono text-xs">{k.key}</code>
              </FieldLabel>
              <Input
                id={`repo-env-${k.key}`}
                type={k.secret || SECRET_KEY.test(k.key) ? 'password' : 'text'}
                autoComplete="off"
                value={env[k.key] ?? ''}
                aria-invalid={submitted && (env[k.key] ?? '').trim() === ''}
                onChange={(e) => {
                  setEnv({ ...env, [k.key]: e.target.value })
                }}
              />
              {submitted && (env[k.key] ?? '').trim() === '' ? (
                <FieldDescription>
                  {t('import.form.envRequired')}
                </FieldDescription>
              ) : null}
            </Field>
          ))}
          {extra.map((v) => (
            <div key={v.id} className="flex min-w-0 flex-col gap-2 sm:flex-row">
              <Input
                aria-label={t('import.form.varName')}
                placeholder="KEY"
                value={v.key}
                className="min-w-0 font-mono sm:w-56"
                onChange={(e) => {
                  setExtra(
                    extra.map((x) =>
                      x.id === v.id ? { ...x, key: e.target.value } : x,
                    ),
                  )
                }}
              />
              <Input
                aria-label={t('import.form.varValue')}
                type={SECRET_KEY.test(v.key) ? 'password' : 'text'}
                autoComplete="off"
                value={v.value}
                className="min-w-0 sm:flex-1"
                onChange={(e) => {
                  setExtra(
                    extra.map((x) =>
                      x.id === v.id ? { ...x, value: e.target.value } : x,
                    ),
                  )
                }}
              />
            </div>
          ))}
        </fieldset>
      ) : null}
      <Button
        type="button"
        size="sm"
        variant="outline"
        onClick={() => {
          setExtra([
            ...extra,
            { id: Date.now() + extra.length, key: '', value: '' },
          ])
        }}
      >
        <PlusIcon aria-hidden="true" className="size-4" />
        {t('import.form.addVar')}
      </Button>

      <Field>
        <FieldLabel>{t('import.form.domain')}</FieldLabel>
        {customOpen ? (
          <>
            <Input
              value={customDomain}
              placeholder={t('import.form.domainPlaceholder')}
              aria-label={t('import.form.domain')}
              aria-invalid={domainError}
              autoComplete="off"
              onChange={(e) => {
                setCustomDomain(e.target.value)
              }}
            />
            <FieldDescription>
              {domainError
                ? t('import.form.domainError')
                : t('import.form.domainHint')}
            </FieldDescription>
          </>
        ) : (
          <div className="flex min-w-0 flex-wrap items-center gap-2 text-sm">
            <span className="min-w-0 truncate text-foreground">
              {analysis.domain.value || t('import.form.freeAddress')}
            </span>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              onClick={() => {
                setCustomOpen(true)
              }}
            >
              {t('import.form.customDomain')}
            </Button>
          </div>
        )}
      </Field>

      <Collapsible open={advancedOpen} onOpenChange={onAdvancedOpen}>
        <CollapsibleTrigger className="text-sm text-muted-foreground underline underline-offset-2">
          {t('import.form.advanced')}
        </CollapsibleTrigger>
        <CollapsiblePanel className="grid gap-3 pt-3 sm:grid-cols-2">
          <Field>
            <FieldLabel htmlFor="repo-adv-port">
              {t('import.form.port')}
            </FieldLabel>
            <Input
              id="repo-adv-port"
              inputMode="numeric"
              placeholder={String(analysis.port.value)}
              onChange={(e) => {
                setAdvanced({ ...advanced, port: num(e.target.value) })
              }}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="repo-adv-health">
              {t('import.form.health')}
            </FieldLabel>
            <Input
              id="repo-adv-health"
              placeholder={analysis.health.value}
              onChange={(e) => {
                setAdvanced({
                  ...advanced,
                  healthPath: e.target.value || undefined,
                })
              }}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="repo-adv-mem">
              {t('import.form.memory')}
            </FieldLabel>
            <Input
              id="repo-adv-mem"
              inputMode="numeric"
              placeholder={String(
                Math.round(analysis.memory.bytes / 1_048_576),
              )}
              onChange={(e) => {
                setAdvanced({ ...advanced, memoryMb: num(e.target.value) })
              }}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="repo-adv-dir">
              {t('import.form.directory')}
            </FieldLabel>
            <Input
              id="repo-adv-dir"
              placeholder={analysis.build.base_directory ?? '/'}
              onChange={(e) => {
                setAdvanced({
                  ...advanced,
                  baseDirectory: e.target.value || undefined,
                })
              }}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="repo-adv-build">
              {t('import.form.buildType')}
            </FieldLabel>
            <Select
              value={advanced.buildType ?? analysis.build.type}
              onValueChange={(v) => {
                if (v) {
                  setAdvanced({
                    ...advanced,
                    buildType: v as NewAppEdits['buildType'],
                  })
                }
              }}
            >
              <SelectTrigger id="repo-adv-build" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {BUILD_TYPES.map((b) => (
                  <SelectItem key={b} value={b}>
                    {t(`findings.advanced.types.${b}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
        </CollapsiblePanel>
      </Collapsible>

      {failure ? (
        <Alert variant="destructive">
          <AlertDescription>{failure}</AlertDescription>
        </Alert>
      ) : null}

      <div className="flex min-w-0 flex-col gap-3 border-t border-border pt-4 sm:flex-row sm:items-center sm:justify-between">
        <p className="min-w-0 text-sm text-muted-foreground" aria-live="polite">
          {t('import.summary', { builder, branch, target })}
        </p>
        <Button type="submit" disabled={deploying} className="w-full sm:w-auto">
          {deploying ? t('import.form.deploying') : t('import.form.deploy')}
        </Button>
      </div>
    </form>
  )
}
