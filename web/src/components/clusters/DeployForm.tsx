import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { RocketGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import { parseEnvLines } from '../../lib/envLines'
import { imageRegistryHost } from '../../lib/registryHost'
import { useDeployApp, usePullSecrets } from '../../queries/clusters'
import { useRuntimeSettings } from '../../queries/runtimeSettings'
import type { DeployK8sInput } from '../../types/clusters'

// The API refuses with this phrase when the namespace is missing and cannot be created.
const NS_CREATE_REFUSAL = 'cannot create namespaces'
const APP_RE = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/

export function DeployForm({ cluster }: Readonly<{ cluster: string }>) {
  const { t } = useTranslation('clusters')
  const deploy = useDeployApp(cluster)
  const settings = useRuntimeSettings()
  const pullSecrets = usePullSecrets(cluster)
  const [app, setApp] = useState('')
  const [image, setImage] = useState('')
  const [port, setPort] = useState('80')
  const [replicas, setReplicas] = useState('1')
  const [host, setHost] = useState('')
  const [namespace, setNamespace] = useState<string | null>(null)
  const [env, setEnv] = useState('')
  const [allowRoot, setAllowRoot] = useState(false)
  const [pullSecret, setPullSecret] = useState('')

  // The prefill is the live default namespace setting; empty lets the server decide.
  const defaultNs = settings.data?.settings.find(
    (s) => s.key === 'k8s.default_namespace',
  )?.value
  const nsValue = namespace ?? (typeof defaultNs === 'string' ? defaultNs : '')

  const secrets = pullSecrets.data ?? []
  const chosen = secrets.find((x) => x.name === pullSecret)
  const imageHost = imageRegistryHost(image.trim())
  const mismatch =
    chosen !== undefined && image.trim() !== '' && imageHost !== chosen.registry

  const parsed = parseEnvLines(env)
  const portNum = Number(port)
  const replicasNum = Number(replicas)
  const valid =
    APP_RE.test(app) &&
    image.trim() !== '' &&
    Number.isInteger(portNum) &&
    portNum >= 1 &&
    portNum <= 65535 &&
    Number.isInteger(replicasNum) &&
    replicasNum >= 0 &&
    parsed.ok
  const canSubmit = valid && !deploy.isPending

  function submit(e: FormEvent) {
    e.preventDefault()
    if (!canSubmit || !parsed.ok) return
    const input: DeployK8sInput = {
      app,
      image: image.trim(),
      port: portNum,
      replicas: replicasNum,
      ...(nsValue.trim() ? { namespace: nsValue.trim() } : {}),
      ...(host.trim() ? { host: host.trim() } : {}),
      ...(Object.keys(parsed.env).length > 0 ? { env: parsed.env } : {}),
      ...(allowRoot ? { allow_root: true } : {}),
      ...(pullSecret ? { pull_secret: pullSecret } : {}),
    }
    deploy.mutate(input, {
      onSuccess: () => {
        toast.add({ title: t('deploy.done', { app }), type: 'success' })
        // Env is write-only: cleared so the values are never shown again.
        setApp('')
        setImage('')
        setHost('')
        setEnv('')
        setAllowRoot(false)
      },
    })
  }

  return (
    <form
      onSubmit={submit}
      aria-label={t('deploy.title')}
      className="flex flex-col gap-3 rounded-lg border border-border bg-card p-3"
    >
      <h2 className="text-sm font-semibold">{t('deploy.title')}</h2>
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-[1fr_2fr_6rem_6rem]">
        <div className="flex flex-col gap-1">
          <Label htmlFor="k8s-app">{t('deploy.app')}</Label>
          <Input
            id="k8s-app"
            value={app}
            autoComplete="off"
            aria-invalid={app !== '' && !APP_RE.test(app)}
            onChange={(e) => {
              setApp(e.target.value)
            }}
          />
        </div>
        <div className="flex flex-col gap-1">
          <Label htmlFor="k8s-image">{t('deploy.image')}</Label>
          <Input
            id="k8s-image"
            value={image}
            autoComplete="off"
            placeholder={t('deploy.imagePlaceholder')}
            onChange={(e) => {
              setImage(e.target.value)
            }}
          />
        </div>
        <div className="flex flex-col gap-1">
          <Label htmlFor="k8s-port">{t('deploy.port')}</Label>
          <Input
            id="k8s-port"
            type="number"
            min={1}
            max={65535}
            value={port}
            onChange={(e) => {
              setPort(e.target.value)
            }}
          />
        </div>
        <div className="flex flex-col gap-1">
          <Label htmlFor="k8s-replicas">{t('deploy.replicas')}</Label>
          <Input
            id="k8s-replicas"
            type="number"
            min={0}
            value={replicas}
            onChange={(e) => {
              setReplicas(e.target.value)
            }}
          />
        </div>
      </div>
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        <div className="flex flex-col gap-1">
          <Label htmlFor="k8s-namespace">{t('deploy.namespace')}</Label>
          <Input
            id="k8s-namespace"
            value={nsValue}
            autoComplete="off"
            onChange={(e) => {
              setNamespace(e.target.value)
            }}
          />
        </div>
        <div className="flex flex-col gap-1">
          <Label htmlFor="k8s-host">{t('deploy.host')}</Label>
          <Input
            id="k8s-host"
            value={host}
            autoComplete="off"
            placeholder={t('deploy.hostPlaceholder')}
            onChange={(e) => {
              setHost(e.target.value)
            }}
          />
        </div>
        <div className="flex flex-col gap-1">
          <Label htmlFor="k8s-pull-secret">{t('deploy.pullSecret')}</Label>
          <NativeSelect
            id="k8s-pull-secret"
            value={pullSecret}
            aria-describedby="k8s-pull-secret-hint"
            onChange={(e) => {
              setPullSecret(e.target.value)
            }}
          >
            <option value="">{t('deploy.pullSecretNone')}</option>
            {secrets.map((x) => (
              <option key={x.name} value={x.name}>
                {x.name} ({x.registry})
              </option>
            ))}
          </NativeSelect>
        </div>
      </div>
      <div className="flex flex-col gap-1">
        <Label htmlFor="k8s-env">{t('deploy.env')}</Label>
        <Textarea
          id="k8s-env"
          value={env}
          rows={2}
          spellCheck={false}
          autoComplete="off"
          className="min-h-10 font-mono text-xs"
          placeholder={t('deploy.envPlaceholder')}
          aria-invalid={!parsed.ok}
          onChange={(e) => {
            setEnv(e.target.value)
          }}
        />
        <p
          className={
            parsed.ok
              ? 'text-xs text-muted-foreground'
              : 'text-xs text-destructive'
          }
        >
          {parsed.ok
            ? t('deploy.envHint')
            : t('deploy.envInvalid', { line: parsed.line })}
        </p>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <div className="flex min-w-0 flex-1 basis-64 items-start gap-2">
          <Checkbox
            id="k8s-allow-root"
            checked={allowRoot}
            onCheckedChange={(v) => {
              setAllowRoot(v)
            }}
          />
          <div className="flex flex-col gap-0.5">
            <Label htmlFor="k8s-allow-root">{t('deploy.allowRoot')}</Label>
            <p className="flex items-center gap-1 text-xs text-tone-warning">
              <TriangleAlertGlassIcon aria-hidden="true" className="size-4" />
              {t('deploy.allowRootWarning')}
            </p>
          </div>
        </div>
        <Button type="submit" size="sm" disabled={!canSubmit}>
          <RocketGlassIcon size="1.2em" aria-hidden="true" />
          {deploy.isPending ? t('deploy.pending') : t('deploy.submit')}
        </Button>
      </div>
      {mismatch || (secrets.length === 0 && pullSecrets.isSuccess) ? (
        <p
          id="k8s-pull-secret-hint"
          className={
            mismatch
              ? 'flex items-center gap-1 text-xs text-tone-warning'
              : 'text-xs text-muted-foreground'
          }
        >
          {mismatch ? (
            <>
              <TriangleAlertGlassIcon aria-hidden="true" className="size-4" />
              {t('deploy.pullSecretMismatch', {
                image: imageHost,
                name: chosen.name,
                secret: chosen.registry,
              })}
            </>
          ) : (
            t('deploy.pullSecretNoneStored')
          )}
        </p>
      ) : null}
      {deploy.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {deploy.error.message}
        </p>
      ) : null}
      {deploy.isError && deploy.error.message.includes(NS_CREATE_REFUSAL) ? (
        <p id="k8s-namespace-hint" className="text-xs text-muted-foreground">
          {t('deploy.namespaceCreateHint')}
        </p>
      ) : null}
    </form>
  )
}
