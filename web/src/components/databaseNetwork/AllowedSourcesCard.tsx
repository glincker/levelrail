import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { FireIcon, PlusIcon, TrashIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import type {
  DatabaseNetwork,
  DatabaseRule,
  RulesPlan,
} from '../../types/databaseAccess'
import {
  useApplyRules,
  usePreviewRules,
  useRemoveRules,
} from '../../queries/databaseAccess'

const LOCAL_SOURCE = '172.16.0.0/12'

interface DraftRule extends DatabaseRule {
  key: number
}

function RulesEditor({
  databaseName,
  rules,
}: {
  databaseName: string
  rules: DatabaseNetwork['rules']
}) {
  const { t } = useTranslation('databaseAccess')
  const preview = usePreviewRules(databaseName)
  const apply = useApplyRules(databaseName)
  const remove = useRemoveRules(databaseName)
  const [draft, setDraft] = useState<DraftRule[]>(() =>
    rules.allow
      .filter((r) => r.source !== LOCAL_SOURCE)
      .map((r, i) => ({ ...r, key: i })),
  )
  const [local, setLocal] = useState(
    rules.allow.some((r) => r.source === LOCAL_SOURCE),
  )
  const [counter, setCounter] = useState(draft.length)
  const [plan, setPlan] = useState<RulesPlan | null>(null)
  const [confirmed, setConfirmed] = useState(false)

  const filled = draft.filter((r) => r.source.trim() !== '')
  const input = {
    allow: filled.map((r) => ({
      source: r.source.trim(),
      description: r.description?.trim() ?? '',
    })),
    local_containers: local,
  }
  const canPreview = filled.length > 0 || local

  function edit(key: number, patch: Partial<DatabaseRule>) {
    setPlan(null)
    setDraft((d) => d.map((r) => (r.key === key ? { ...r, ...patch } : r)))
  }

  function fail(err: Error) {
    toast.add({
      title: t('rules.failedToast'),
      description: err.message,
      type: 'error',
    })
  }

  return (
    <div className="space-y-3">
      {rules.missing && rules.missing.length > 0 ? (
        <Alert variant="destructive">
          <AlertDescription>
            {t('rules.driftMissing', { sources: rules.missing.join(', ') })}
          </AlertDescription>
        </Alert>
      ) : null}
      {rules.extra && rules.extra.length > 0 ? (
        <Alert variant="destructive">
          <AlertDescription>
            {t('rules.driftExtra', { sources: rules.extra.join(', ') })}
          </AlertDescription>
        </Alert>
      ) : null}
      {!rules.active && draft.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t('rules.empty')}</p>
      ) : null}
      <div className="space-y-2">
        {draft.map((r) => (
          <div key={r.key} className="flex flex-wrap items-center gap-2">
            <Input
              aria-label={t('rules.source')}
              placeholder="203.0.113.7 or 10.0.0.0/8"
              className="w-56 font-mono"
              value={r.source}
              onChange={(e) => edit(r.key, { source: e.target.value })}
            />
            <Input
              aria-label={t('rules.description_')}
              placeholder={t('rules.descriptionPlaceholder')}
              className="w-64"
              value={r.description ?? ''}
              maxLength={120}
              onChange={(e) => edit(r.key, { description: e.target.value })}
            />
            <Button
              variant="ghost"
              size="sm"
              aria-label={t('rules.remove')}
              onClick={() => {
                setPlan(null)
                setDraft((d) => d.filter((x) => x.key !== r.key))
              }}
            >
              <TrashIcon aria-hidden="true" />
            </Button>
          </div>
        ))}
        <Button
          variant="outline"
          size="sm"
          onClick={() => {
            setPlan(null)
            setDraft((d) => [...d, { key: counter, source: '' }])
            setCounter((c) => c + 1)
          }}
        >
          <PlusIcon aria-hidden="true" />
          {t('rules.add')}
        </Button>
      </div>
      <label className="flex items-center gap-2 text-sm">
        <Checkbox
          checked={local}
          onCheckedChange={(v) => {
            setLocal(v === true)
            setPlan(null)
          }}
        />
        <span>{t('rules.localContainers')}</span>
      </label>
      {plan ? (
        <div className="space-y-2">
          <p className="text-xs font-medium">{t('rules.previewTitle')}</p>
          <pre className="overflow-x-auto rounded-md bg-muted p-2 font-mono text-xs">
            {plan.commands.join('\n')}
          </pre>
          <Alert>
            <AlertTitle>{t('rules.effect')}</AlertTitle>
            <AlertDescription>{plan.drops}</AlertDescription>
          </Alert>
          {plan.warnings.map((w) => (
            <Alert key={w} variant="destructive">
              <AlertDescription>{w}</AlertDescription>
            </Alert>
          ))}
          <label className="flex items-center gap-2 text-sm">
            <Checkbox
              checked={confirmed}
              onCheckedChange={(v) => setConfirmed(v === true)}
            />
            <span>{t('rules.confirm')}</span>
          </label>
        </div>
      ) : null}
      <div className="flex flex-wrap gap-2">
        <Button
          variant="outline"
          disabled={!canPreview || preview.isPending}
          onClick={() => {
            setConfirmed(false)
            preview.mutate(input, { onSuccess: setPlan, onError: fail })
          }}
        >
          {t('rules.preview')}
        </Button>
        <Button
          disabled={!plan || !confirmed || apply.isPending}
          onClick={() =>
            apply.mutate(input, {
              onSuccess: () => {
                toast.add({ title: t('rules.appliedToast'), type: 'success' })
                setPlan(null)
                setConfirmed(false)
              },
              onError: fail,
            })
          }
        >
          {t('rules.apply')}
        </Button>
        {rules.active ? (
          <Button
            variant="ghost"
            disabled={remove.isPending}
            onClick={() =>
              remove.mutate(undefined, {
                onSuccess: () => {
                  toast.add({ title: t('rules.clearedToast'), type: 'success' })
                  setDraft([])
                  setLocal(false)
                },
                onError: fail,
              })
            }
          >
            {t('rules.clear')}
          </Button>
        ) : null}
      </div>
    </div>
  )
}

/** AllowedSourcesCard is the security group: an inbound rule editor for the published port, applied through the host firewall. */
export function AllowedSourcesCard({
  databaseName,
  network,
}: {
  databaseName: string
  network: DatabaseNetwork
}) {
  const { t } = useTranslation('databaseAccess')
  const { rules } = network
  const noPort = !network.published

  return (
    <Card>
      <CardHeader className="space-y-1">
        <CardTitle className="flex items-center gap-1.5 text-sm">
          <FireIcon className="size-4" aria-hidden="true" />
          {t('rules.title')}
        </CardTitle>
        <p className="text-xs text-muted-foreground">
          {t('rules.description')}
        </p>
      </CardHeader>
      <CardContent>
        {noPort ? (
          <p className="text-sm text-muted-foreground">{t('rules.noPort')}</p>
        ) : !rules.can_restrict && !rules.active ? (
          <p className="text-sm text-muted-foreground">
            {t('rules.cannot', { reason: rules.cannot_restrict_reason ?? '' })}
          </p>
        ) : (
          <RulesEditor
            key={JSON.stringify(rules.allow)}
            databaseName={databaseName}
            rules={rules}
          />
        )}
      </CardContent>
    </Card>
  )
}
