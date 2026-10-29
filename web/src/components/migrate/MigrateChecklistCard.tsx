import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CircleCheckGlassIcon, CircleGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import {
  fetchCutoverVerdict,
  StepBlockedError,
  useStepChecklist,
  type ChecklistStep,
  type MigrateChecklist,
  type VolumeStat,
} from '../../queries/migrateWizard'

function parsePair(raw: string): [number, number] | null {
  const parts = raw.trim().split(/[\s:,]+/)
  if (parts.length !== 2) return null
  const a = Number(parts[0])
  const b = Number(parts[1])
  if (!Number.isInteger(a) || !Number.isInteger(b) || a < 0 || b < 0) {
    return null
  }
  return [a, b]
}

function StepRow({
  step,
  current,
  app,
  checklist,
}: {
  step: ChecklistStep
  current: boolean
  app: string
  checklist: MigrateChecklist
}) {
  const { t } = useTranslation('migrate')
  const mutation = useStepChecklist()
  const [databases, setDatabases] = useState('')
  const [pairs, setPairs] = useState<Record<string, [string, string]>>({})
  const [problems, setProblems] = useState<string[]>([])
  const [dns, setDns] = useState<string | null>(null)
  const [checking, setChecking] = useState(false)
  const done = step.status === 'done'

  function send(
    action: 'complete' | 'reset',
    extra: { confirm?: boolean; volumes?: VolumeStat[] } = {},
  ) {
    setProblems([])
    mutation.mutate(
      {
        app,
        step: step.id,
        req: {
          action,
          confirm: extra.confirm,
          volumes: extra.volumes,
          databases:
            step.id === 'data_verified'
              ? databases
                  .split(',')
                  .map((d) => d.trim())
                  .filter(Boolean)
              : undefined,
        },
      },
      {
        onError: (e) => {
          if (e instanceof StepBlockedError) {
            setProblems(e.problems.length > 0 ? e.problems : [e.message])
            return
          }
          toast.add({
            title: e instanceof Error ? e.message : t('cutover.stepError'),
            type: 'error',
          })
        },
      },
    )
  }

  function complete() {
    if (step.id === 'old_stopped') {
      send('complete', { confirm: true })
      return
    }
    if (step.id === 'data_verified') {
      const stats: VolumeStat[] = []
      for (const v of checklist.volumes ?? []) {
        const [src, dst] = pairs[v.name] ?? ['', '']
        const s = parsePair(src)
        const d = parsePair(dst)
        if (s && d) {
          stats.push({
            name: v.name,
            source_files: s[0],
            source_bytes: s[1],
            target_files: d[0],
            target_bytes: d[1],
          })
        }
      }
      send('complete', { volumes: stats })
      return
    }
    send('complete')
  }

  async function recheck() {
    setChecking(true)
    try {
      const r = await fetchCutoverVerdict(app)
      setDns(r.verdict)
    } catch (e) {
      toast.add({
        title: e instanceof Error ? e.message : t('cutover.stepError'),
        type: 'error',
      })
    } finally {
      setChecking(false)
    }
  }

  return (
    <li className="space-y-2 py-3">
      <div className="flex flex-wrap items-center gap-2">
        {done ? (
          <CircleCheckGlassIcon className="size-6 text-tone-success" />
        ) : (
          <CircleGlassIcon className="size-6 text-muted-foreground" />
        )}
        <span className="font-medium">{t(`steps.${step.id}`)}</span>
        <Badge variant={done ? 'success' : 'muted'}>
          {done ? t('cutover.done') : t('cutover.pending')}
        </Badge>
        {done && step.actor ? (
          <span className="text-xs text-muted-foreground">
            {t('cutover.by', { actor: step.actor })}
          </span>
        ) : null}
      </div>
      <p className="pl-7 text-sm text-muted-foreground">
        {t(`stepHelp.${step.id}`)}
      </p>
      {done && (step.evidence ?? []).length > 0 ? (
        <ul className="list-disc pl-12 text-xs text-muted-foreground">
          {step.evidence?.map((e) => (
            <li key={e}>{e}</li>
          ))}
        </ul>
      ) : null}

      {current && step.id === 'data_verified' ? (
        <div className="space-y-3 pl-7">
          <div className="space-y-1">
            <Label htmlFor={`${app}-dbs`}>{t('verify.databases')}</Label>
            <Input
              id={`${app}-dbs`}
              value={databases}
              onChange={(e) => setDatabases(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              {t('verify.databasesHint')}
            </p>
          </div>
          {(checklist.volumes ?? []).length > 0 ? (
            <div className="space-y-3">
              <p className="text-xs text-muted-foreground">
                {t('verify.volumesHint')}
              </p>
              {checklist.volumes?.map((v) => (
                <div key={v.name} className="space-y-1 rounded-md border p-2">
                  <p className="text-sm font-medium">{v.name}</p>
                  {v.source_command ? (
                    <p className="text-xs">
                      {t('verify.sourceCommand')}:{' '}
                      <code className="break-all">{v.source_command}</code>
                    </p>
                  ) : null}
                  <p className="text-xs">
                    {t('verify.targetCommand')}:{' '}
                    <code className="break-all">{v.target_command}</code>
                  </p>
                  <div className="grid gap-2 sm:grid-cols-2">
                    <Input
                      aria-label={`${v.name} ${t('verify.sourceResult')}`}
                      placeholder={t('verify.resultPlaceholder')}
                      value={pairs[v.name]?.[0] ?? ''}
                      onChange={(e) =>
                        setPairs((p) => ({
                          ...p,
                          [v.name]: [e.target.value, p[v.name]?.[1] ?? ''],
                        }))
                      }
                    />
                    <Input
                      aria-label={`${v.name} ${t('verify.targetResult')}`}
                      placeholder={t('verify.resultPlaceholder')}
                      value={pairs[v.name]?.[1] ?? ''}
                      onChange={(e) =>
                        setPairs((p) => ({
                          ...p,
                          [v.name]: [p[v.name]?.[0] ?? '', e.target.value],
                        }))
                      }
                    />
                  </div>
                </div>
              ))}
            </div>
          ) : null}
        </div>
      ) : null}

      {current && step.id === 'dns_switched' ? (
        <div className="flex flex-wrap items-center gap-2 pl-7">
          <Button
            size="sm"
            variant="outline"
            disabled={checking}
            onClick={() => void recheck()}
          >
            {checking ? t('cutover.checkingDns') : t('cutover.recheck')}
          </Button>
          {dns ? (
            <span className="text-sm">
              {t('cutover.dnsResult', { verdict: dns })}
            </span>
          ) : null}
        </div>
      ) : null}

      {problems.length > 0 ? (
        <div className="pl-7">
          <Banner variant="warning" title={t('cutover.blocked')}>
            <ul className="list-disc pl-5 text-sm">
              {problems.map((p) => (
                <li key={p}>{p}</li>
              ))}
            </ul>
            {step.id === 'dns_switched' ? (
              <Button
                size="sm"
                variant="outline"
                className="mt-2"
                disabled={mutation.isPending}
                onClick={() => send('complete', { confirm: true })}
              >
                {t('cutover.confirm')}
              </Button>
            ) : null}
          </Banner>
        </div>
      ) : null}

      <div className="pl-7">
        {done ? (
          <Button
            size="sm"
            variant="outline"
            disabled={mutation.isPending}
            onClick={() => send('reset')}
          >
            {step.actor === 'migration-agent'
              ? t('cutover.undoAgent')
              : t('cutover.reset')}
          </Button>
        ) : current ? (
          <Button size="sm" disabled={mutation.isPending} onClick={complete}>
            {mutation.isPending ? t('cutover.working') : t('cutover.complete')}
          </Button>
        ) : null}
      </div>
    </li>
  )
}

export function MigrateChecklistCard({
  checklist,
}: {
  checklist: MigrateChecklist
}) {
  const { t } = useTranslation('migrate')
  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-center gap-2">
          <CardTitle>{checklist.app}</CardTitle>
          <Badge variant={checklist.current === '' ? 'success' : 'info'}>
            {t('cutover.progress', {
              done: checklist.progress,
              total: checklist.total,
            })}
          </Badge>
          <span className="text-sm text-muted-foreground">
            {checklist.current === ''
              ? t('cutover.finished')
              : t('cutover.next', { step: t(`steps.${checklist.current}`) })}
          </span>
        </div>
      </CardHeader>
      <CardContent>
        <ul className="divide-y">
          {checklist.steps.map((s) => (
            <StepRow
              key={s.id}
              step={s}
              current={s.id === checklist.current}
              app={checklist.app}
              checklist={checklist}
            />
          ))}
        </ul>
      </CardContent>
    </Card>
  )
}
