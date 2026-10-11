import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { GearSixIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { SkeletonLine } from './kit'
import { useGitSource } from '../queries/gitSources'
import { usePreviewPolicy, useSetPreviewPolicy } from '../queries/previewPolicy'
import type {
  PreviewDatabaseStrategy,
  PreviewPolicy,
  PreviewPolicyUpdate,
} from '../types/previewEnvironment'

const STRATEGIES: PreviewDatabaseStrategy[] = [
  'none',
  'fresh',
  'seed',
  'shared',
]

function isStrategy(value: string): value is PreviewDatabaseStrategy {
  return STRATEGIES.some((s) => s === value)
}

// PreviewLifecycleCard edits the preview policy fields that shape how a
// preview is sized, what data it reaches and who can see it
// (PUT /api/v1/apps/{name}/preview-policy).
export function PreviewLifecycleCard({ appName }: { appName: string }) {
  const { t } = useTranslation('previews')
  const gitSource = useGitSource(appName)
  const policy = usePreviewPolicy(appName)

  if (!gitSource.data) return null

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <GearSixIcon className="size-4 text-muted-foreground" />
          {t('lifecycle.title')}
        </CardTitle>
        <CardDescription>{t('lifecycle.description')}</CardDescription>
      </CardHeader>
      <CardContent>
        {policy.isPending ? (
          <div className="space-y-2" aria-label={t('lifecycle.loading')}>
            <SkeletonLine width="60%" />
            <SkeletonLine width="40%" />
          </div>
        ) : policy.isError ? (
          <p className="text-sm text-muted-foreground">
            {t('lifecycle.unavailable')}
          </p>
        ) : (
          <LifecycleForm appName={appName} policy={policy.data} />
        )}
      </CardContent>
    </Card>
  )
}

function Field({
  id,
  label,
  hint,
  children,
}: {
  id: string
  label: string
  hint?: string
  children: React.ReactNode
}) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      {children}
      {hint ? <p className="text-xs text-muted-foreground">{hint}</p> : null}
    </div>
  )
}

function SwitchRow({
  label,
  hint,
  checked,
  onChange,
}: {
  label: string
  hint: string
  checked: boolean
  onChange: (next: boolean) => void
}) {
  return (
    <div className="flex items-center justify-between gap-4">
      <div>
        <p className="text-sm font-medium text-foreground">{label}</p>
        <p className="text-sm text-muted-foreground">{hint}</p>
      </div>
      <Switch checked={checked} onCheckedChange={onChange} aria-label={label} />
    </div>
  )
}

function LifecycleForm({
  appName,
  policy,
}: {
  appName: string
  policy: PreviewPolicy
}) {
  const { t } = useTranslation('previews')
  const setPolicy = useSetPreviewPolicy(appName)
  const [maxPreviews, setMaxPreviews] = useState(String(policy.max_previews))
  const [memory, setMemory] = useState(policy.memory_limit)
  const [cpu, setCpu] = useState(String(policy.cpu_limit))
  const [idle, setIdle] = useState(String(policy.idle_sleep_minutes))
  const [strategy, setStrategy] = useState<PreviewDatabaseStrategy>(
    policy.database_strategy,
  )
  const [seed, setSeed] = useState(policy.seed_database)
  const [forkSecrets, setForkSecrets] = useState(policy.allow_fork_secrets)
  const [indexing, setIndexing] = useState(policy.allow_indexing)
  const [gate, setGate] = useState(policy.gate_basic_auth)
  const [gateUser, setGateUser] = useState(policy.gate_username)
  const [gatePass, setGatePass] = useState('')

  const numbers = [maxPreviews, cpu, idle].map(Number)
  const numbersValid = numbers.every((n) => Number.isFinite(n) && n >= 0)
  const gateReady =
    !gate ||
    (gateUser.trim() !== '' && (gatePass !== '' || policy.gate_password_set))

  function save() {
    const update: PreviewPolicyUpdate = {
      max_previews: Number(maxPreviews),
      memory_limit: memory.trim(),
      cpu_limit: Number(cpu),
      idle_sleep_minutes: Number(idle),
      database_strategy: strategy,
      seed_database: seed.trim(),
      allow_fork_secrets: forkSecrets,
      allow_indexing: indexing,
      gate_username: gateUser.trim(),
      gate_basic_auth: gate,
    }
    if (gatePass !== '') update.gate_password = gatePass
    setPolicy.mutate(update, {
      onSuccess: () => {
        setGatePass('')
        toast.add({ title: t('lifecycle.saved'), type: 'success' })
      },
      onError: (error) => {
        toast.add({
          title: t('lifecycle.saveError'),
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  return (
    <div className="space-y-6">
      <div className="grid gap-4 sm:grid-cols-2">
        <Field
          id="preview-max"
          label={t('lifecycle.maxPreviews')}
          hint={t('lifecycle.maxPreviewsHint', {
            cap: policy.max_per_app > 0 ? policy.max_per_app : '0',
          })}
        >
          <Input
            id="preview-max"
            inputMode="numeric"
            className="w-28"
            value={maxPreviews}
            onChange={(e) => {
              setMaxPreviews(e.target.value)
            }}
          />
        </Field>
        <Field
          id="preview-memory"
          label={t('lifecycle.memory')}
          hint={t('lifecycle.memoryHint', { value: policy.effective_memory })}
        >
          <Input
            id="preview-memory"
            className="w-28"
            placeholder={policy.effective_memory}
            value={memory}
            onChange={(e) => {
              setMemory(e.target.value)
            }}
          />
        </Field>
        <Field
          id="preview-cpu"
          label={t('lifecycle.cpu')}
          hint={t('lifecycle.cpuHint', { value: policy.effective_cpu })}
        >
          <Input
            id="preview-cpu"
            inputMode="decimal"
            className="w-28"
            value={cpu}
            onChange={(e) => {
              setCpu(e.target.value)
            }}
          />
        </Field>
        <Field
          id="preview-idle"
          label={t('lifecycle.idle')}
          hint={t('lifecycle.idleHint', {
            value: policy.effective_idle_sleep_minutes,
          })}
        >
          <Input
            id="preview-idle"
            inputMode="numeric"
            className="w-28"
            value={idle}
            onChange={(e) => {
              setIdle(e.target.value)
            }}
          />
        </Field>
      </div>

      <div className="space-y-3">
        <Field
          id="preview-db-strategy"
          label={t('lifecycle.database.label')}
          hint={t('lifecycle.database.hint')}
        >
          <Select
            value={strategy}
            onValueChange={(value) => {
              if (value && isStrategy(value)) setStrategy(value)
            }}
          >
            <SelectTrigger id="preview-db-strategy" className="w-full sm:w-96">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {STRATEGIES.map((key) => (
                <SelectItem key={key} value={key}>
                  {t(`lifecycle.database.${key}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        {strategy === 'seed' ? (
          <Field
            id="preview-seed"
            label={t('lifecycle.database.seedName')}
            hint={t('lifecycle.database.seedHint')}
          >
            <Input
              id="preview-seed"
              className="w-64"
              value={seed}
              onChange={(e) => {
                setSeed(e.target.value)
              }}
            />
          </Field>
        ) : null}
        {strategy === 'shared' ? (
          <Alert variant="destructive">
            <AlertDescription>
              {t('lifecycle.database.sharedWarning')}
            </AlertDescription>
          </Alert>
        ) : null}
      </div>

      <div className="space-y-4">
        <h3 className="text-sm font-medium text-foreground">
          {t('lifecycle.exposure.heading')}
        </h3>
        <SwitchRow
          label={t('lifecycle.exposure.forkSecrets')}
          hint={t('lifecycle.exposure.forkSecretsHint')}
          checked={forkSecrets}
          onChange={setForkSecrets}
        />
        {forkSecrets ? (
          <Alert variant="destructive">
            <AlertDescription>
              {t('lifecycle.exposure.forkSecretsWarning')}
            </AlertDescription>
          </Alert>
        ) : null}
        <SwitchRow
          label={t('lifecycle.exposure.indexing')}
          hint={t('lifecycle.exposure.indexingHint')}
          checked={indexing}
          onChange={setIndexing}
        />
        <SwitchRow
          label={t('lifecycle.exposure.gate')}
          hint={t('lifecycle.exposure.gateHint')}
          checked={gate}
          onChange={setGate}
        />
        {gate ? (
          <div className="grid gap-4 sm:grid-cols-2">
            <Field
              id="preview-gate-user"
              label={t('lifecycle.exposure.gateUsername')}
            >
              <Input
                id="preview-gate-user"
                autoComplete="off"
                value={gateUser}
                onChange={(e) => {
                  setGateUser(e.target.value)
                }}
              />
            </Field>
            <Field
              id="preview-gate-pass"
              label={t('lifecycle.exposure.gatePassword')}
              hint={
                policy.gate_password_set
                  ? t('lifecycle.exposure.gatePasswordSet')
                  : undefined
              }
            >
              <Input
                id="preview-gate-pass"
                type="password"
                autoComplete="new-password"
                value={gatePass}
                onChange={(e) => {
                  setGatePass(e.target.value)
                }}
              />
            </Field>
            {!gateReady ? (
              <p className="text-xs text-muted-foreground sm:col-span-2">
                {t('lifecycle.exposure.gateNeedsPassword')}
              </p>
            ) : null}
          </div>
        ) : null}
      </div>

      <Button
        type="button"
        size="sm"
        disabled={!numbersValid || !gateReady || setPolicy.isPending}
        onClick={save}
      >
        {t('lifecycle.save')}
      </Button>
    </div>
  )
}
