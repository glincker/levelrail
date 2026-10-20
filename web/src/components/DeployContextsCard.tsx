import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldHint, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import {
  useClearContextConfig,
  useContextConfigs,
  useSaveContextConfig,
} from '../queries/deployContexts'
import {
  BRANCH_CONTEXT_PREFIX,
  formatKeyValueLines,
  parseKeyValueLines,
} from '../lib/deployContextText'
import type { ContextConfig } from '../types/deployContext'
import { EffectiveConfigPanel } from './EffectiveConfigPanel'

type Kind = 'deploy-preview' | 'branch-deploy' | 'branch'
const KINDS: Kind[] = ['deploy-preview', 'branch-deploy', 'branch']

interface Draft {
  kind: Kind
  pattern: string
  env: string
  buildCommand: string
  publishDir: string
  buildArgs: string
}

const EMPTY: Draft = {
  kind: 'deploy-preview',
  pattern: '',
  env: '',
  buildCommand: '',
  publishDir: '',
  buildArgs: '',
}

function contextName(d: Draft): string {
  return d.kind === 'branch' ? BRANCH_CONTEXT_PREFIX + d.pattern.trim() : d.kind
}

function draftFrom(c: ContextConfig): Draft {
  const isBranch = c.context.startsWith(BRANCH_CONTEXT_PREFIX)
  return {
    kind: isBranch ? 'branch' : (c.context as Kind),
    pattern: isBranch ? c.context.slice(BRANCH_CONTEXT_PREFIX.length) : '',
    env: formatKeyValueLines(c.env),
    buildCommand: c.build_command,
    publishDir: c.publish_dir,
    buildArgs: formatKeyValueLines(c.build_args),
  }
}

export function DeployContextsCard({ appName }: { appName: string }) {
  const { t } = useTranslation('deployContexts')
  const list = useContextConfigs(appName)
  const save = useSaveContextConfig(appName)
  const clear = useClearContextConfig(appName)
  const [draft, setDraft] = useState<Draft>(EMPTY)
  const set = (patch: Partial<Draft>) => {
    setDraft((d) => ({ ...d, ...patch }))
  }

  function onSave() {
    const env = parseKeyValueLines(draft.env)
    const args = parseKeyValueLines(draft.buildArgs)
    const bad = !env.ok ? env.line : !args.ok ? args.line : null
    if (bad !== null || !env.ok || !args.ok) {
      toast.add({
        title: t('editor.saveError'),
        description: t('editor.invalidLine', { line: bad ?? '' }),
        type: 'error',
      })
      return
    }
    save.mutate(
      {
        context: contextName(draft),
        env: env.values,
        build_command: draft.buildCommand.trim(),
        publish_dir: draft.publishDir.trim(),
        build_args: args.values,
      },
      {
        onSuccess: () => {
          toast.add({ title: t('editor.saved'), type: 'success' })
        },
        onError: (err) => {
          toast.add({
            title: t('editor.saveError'),
            description: err.message,
            type: 'error',
          })
        },
      },
    )
  }

  function onClear(context: string) {
    clear.mutate(context, {
      onSuccess: () => {
        toast.add({ title: t('editor.cleared'), type: 'success' })
      },
      onError: (err) => {
        toast.add({
          title: t('editor.clearError'),
          description: err.message,
          type: 'error',
        })
      },
    })
  }

  const saved = list.data ?? []
  const canSave =
    !save.isPending && (draft.kind !== 'branch' || draft.pattern.trim() !== '')

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
        <p className="text-sm text-muted-foreground">{t('description')}</p>
      </CardHeader>
      <CardContent className="space-y-6">
        <div className="space-y-4">
          <Field>
            <FieldLabel htmlFor="ctx-kind">{t('editor.context')}</FieldLabel>
            <Select
              value={draft.kind}
              onValueChange={(v) => {
                if (v) set({ kind: v })
              }}
            >
              <SelectTrigger id="ctx-kind" className="w-full max-w-xs">
                <SelectValue>{t(`contexts.${draft.kind}`)}</SelectValue>
              </SelectTrigger>
              <SelectContent>
                {KINDS.map((k) => (
                  <SelectItem key={k} value={k}>
                    {t(`contexts.${k}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          {draft.kind === 'branch' ? (
            <Field>
              <FieldLabel htmlFor="ctx-pattern">
                {t('editor.branchPattern')}
              </FieldLabel>
              <Input
                id="ctx-pattern"
                className="max-w-xs font-mono"
                value={draft.pattern}
                onChange={(e) => {
                  set({ pattern: e.target.value })
                }}
              />
              <FieldHint>{t('editor.branchPatternHint')}</FieldHint>
            </Field>
          ) : null}
          <Field>
            <FieldLabel htmlFor="ctx-env">{t('editor.env')}</FieldLabel>
            <Textarea
              id="ctx-env"
              className="min-h-24 font-mono"
              value={draft.env}
              onChange={(e) => {
                set({ env: e.target.value })
              }}
            />
            <FieldHint>{t('editor.envHint')}</FieldHint>
          </Field>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field>
              <FieldLabel htmlFor="ctx-command">
                {t('editor.buildCommand')}
              </FieldLabel>
              <Input
                id="ctx-command"
                className="font-mono"
                value={draft.buildCommand}
                onChange={(e) => {
                  set({ buildCommand: e.target.value })
                }}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="ctx-publish">
                {t('editor.publishDir')}
              </FieldLabel>
              <Input
                id="ctx-publish"
                className="font-mono"
                value={draft.publishDir}
                onChange={(e) => {
                  set({ publishDir: e.target.value })
                }}
              />
            </Field>
          </div>
          <Field>
            <FieldLabel htmlFor="ctx-args">{t('editor.buildArgs')}</FieldLabel>
            <Textarea
              id="ctx-args"
              className="min-h-16 font-mono"
              value={draft.buildArgs}
              onChange={(e) => {
                set({ buildArgs: e.target.value })
              }}
            />
            <FieldHint>{t('editor.buildArgsHint')}</FieldHint>
          </Field>
          <Button onClick={onSave} disabled={!canSave}>
            {save.isPending ? t('editor.saving') : t('editor.save')}
          </Button>
        </div>

        <div>
          <h3 className="text-sm font-medium">{t('saved.title')}</h3>
          {list.isError ? (
            <p role="alert" className="py-3 text-sm text-destructive">
              {t('editor.loadError')}
            </p>
          ) : saved.length === 0 && !list.isPending ? (
            <p className="py-3 text-sm text-muted-foreground">
              {t('saved.empty')}
            </p>
          ) : (
            <ul className="divide-y">
              {saved.map((c) => (
                <li
                  key={c.context}
                  className="flex flex-wrap items-center justify-between gap-2 py-2"
                >
                  <div className="min-w-0">
                    <p className="truncate font-mono text-sm">{c.context}</p>
                    <p className="text-xs text-muted-foreground">
                      {t('saved.summary', {
                        env: Object.keys(c.env).length,
                        args: Object.keys(c.build_args).length,
                      })}
                    </p>
                  </div>
                  <div className="flex gap-2">
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => {
                        setDraft(draftFrom(c))
                      }}
                    >
                      {t('saved.edit')}
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={clear.isPending}
                      onClick={() => {
                        onClear(c.context)
                      }}
                    >
                      {t('editor.clear')}
                    </Button>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>

        <EffectiveConfigPanel
          appName={appName}
          initialContext="deploy-preview"
        />
      </CardContent>
    </Card>
  )
}
