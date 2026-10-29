import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { LockGlassIcon } from '@thesvg/ui-react'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useEffectiveConfig } from '../queries/deployContexts'
import type { ConfigSource } from '../types/deployContext'

const CONTEXTS = ['production', 'deploy-preview', 'branch-deploy'] as const
type EffectiveContext = (typeof CONTEXTS)[number]

function SourceTag({ source }: { source: ConfigSource }) {
  const { t } = useTranslation('deployContexts')
  return (
    <span className="rounded-md border px-1.5 py-0.5 text-xs text-muted-foreground">
      {t(`effective.source.${source}`)}
    </span>
  )
}

function Row({
  label,
  value,
  source,
  secret,
}: {
  label: string
  value: string
  source: ConfigSource
  secret?: boolean
}) {
  const { t } = useTranslation('deployContexts')
  return (
    <li className="flex flex-wrap items-center justify-between gap-2 py-2">
      <span className="min-w-0 truncate font-mono text-sm">{label}</span>
      <span className="flex min-w-0 items-center gap-2">
        {secret ? (
          <LockGlassIcon
            className="size-5 text-muted-foreground"
            aria-label={t('effective.secret')}
          />
        ) : null}
        <span className="truncate font-mono text-sm text-muted-foreground">
          {value === '' ? t('effective.none') : value}
        </span>
        <SourceTag source={source} />
      </span>
    </li>
  )
}

/** Shows the env and build settings a deploy in the chosen context would use. */
export function EffectiveConfigPanel({
  appName,
  initialContext = 'production',
  initialBranch = '',
}: {
  appName: string
  initialContext?: EffectiveContext
  initialBranch?: string
}) {
  const { t } = useTranslation('deployContexts')
  const [context, setContext] = useState<EffectiveContext>(initialContext)
  const [branch, setBranch] = useState(initialBranch)
  const query = useEffectiveConfig(appName, context, branch)
  const eff = query.data
  const env = eff?.env ?? []
  const args = Object.entries(eff?.build_args ?? {})

  return (
    <section className="space-y-3" aria-labelledby="effective-config-title">
      <div>
        <h3 id="effective-config-title" className="text-sm font-medium">
          {t('effective.title')}
        </h3>
        <p className="text-xs text-muted-foreground">
          {t('effective.description')}
        </p>
      </div>
      <div className="flex flex-wrap gap-4">
        <Field>
          <FieldLabel htmlFor="effective-context">
            {t('effective.context')}
          </FieldLabel>
          <Select
            value={context}
            onValueChange={(v) => {
              if (v) setContext(v)
            }}
          >
            <SelectTrigger id="effective-context" className="w-56">
              <SelectValue>
                {context === 'production'
                  ? t('effective.production')
                  : t(`contexts.${context}`)}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              {CONTEXTS.map((c) => (
                <SelectItem key={c} value={c}>
                  {c === 'production'
                    ? t('effective.production')
                    : t(`contexts.${c}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        {context !== 'production' ? (
          <Field>
            <FieldLabel htmlFor="effective-branch">
              {t('effective.branch')}
            </FieldLabel>
            <Input
              id="effective-branch"
              className="w-56 font-mono"
              value={branch}
              placeholder={t('effective.branchPlaceholder')}
              onChange={(e) => {
                setBranch(e.target.value)
              }}
            />
          </Field>
        ) : null}
      </div>
      {query.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {t('effective.loadError')}
        </p>
      ) : eff ? (
        <ul className="divide-y rounded-lg border px-3">
          <Row
            label={t('effective.buildCommand')}
            value={eff.build_command.value}
            source={eff.build_command.source}
          />
          <Row
            label={t('effective.publishDir')}
            value={eff.publish_dir.value}
            source={eff.publish_dir.source}
          />
          {args.map(([name, s]) => (
            <Row
              key={name}
              label={t('effective.buildArg', { name })}
              value={s.value}
              source={s.source}
            />
          ))}
          {env.map((e) => (
            <Row
              key={e.key}
              label={e.key}
              value={e.value}
              source={e.source}
              secret={e.secret}
            />
          ))}
          {env.length === 0 ? (
            <li className="py-2 text-sm text-muted-foreground">
              {t('effective.empty')}
            </li>
          ) : null}
        </ul>
      ) : null}
    </section>
  )
}
