import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { PlusGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { SecretInput } from '@/components/ui/secret-input'
import { SegmentedControl } from '@/components/ui/segmented-control'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import { suggestClusterName } from '../../lib/clusterName'
import { useConnectCluster } from '../../queries/clusters'
import type { ConnectClusterInput } from '../../types/clusters'

type Mode = 'token' | 'kubeconfig'

const NAME_RE = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/

export function ConnectClusterForm({
  existing,
}: Readonly<{ existing: readonly string[] }>) {
  const { t } = useTranslation('clusters')
  const connect = useConnectCluster()
  const [mode, setMode] = useState<Mode>('token')
  const [name, setName] = useState('')
  const [nameTouched, setNameTouched] = useState(false)
  const [server, setServer] = useState('')
  const [token, setToken] = useState('')
  const [ca, setCa] = useState('')
  const [kubeconfig, setKubeconfig] = useState('')
  const [context, setContext] = useState('')
  const [allowAdmin, setAllowAdmin] = useState(false)

  const effectiveName = nameTouched ? name : suggestClusterName(server)
  const taken = existing.includes(effectiveName)
  const nameValid = NAME_RE.test(effectiveName) && !taken
  const credsReady =
    mode === 'token'
      ? server.trim() !== '' && token.trim() !== ''
      : kubeconfig.trim() !== ''
  const canSubmit = nameValid && credsReady && !connect.isPending

  // Secrets are cleared on success so a pasted kubeconfig is never shown again.
  function reset() {
    setName('')
    setNameTouched(false)
    setServer('')
    setToken('')
    setCa('')
    setKubeconfig('')
    setContext('')
    setAllowAdmin(false)
  }

  function submit(e: FormEvent) {
    e.preventDefault()
    if (!canSubmit) return
    const base: ConnectClusterInput =
      mode === 'token'
        ? {
            name: effectiveName,
            server: server.trim(),
            token: token.trim(),
            ...(ca.trim() ? { ca: ca.trim() } : {}),
          }
        : {
            name: effectiveName,
            kubeconfig,
            ...(context.trim() ? { context: context.trim() } : {}),
          }
    const input: ConnectClusterInput = allowAdmin
      ? { ...base, allow_cluster_admin: true }
      : base
    connect.mutate(input, {
      onSuccess: () => {
        toast.add({ title: t('connect.done'), type: 'success' })
        reset()
      },
    })
  }

  const errorText = connect.error?.message ?? ''
  const adminRefused = /cluster-admin/i.test(errorText)
  const blocked = !adminRefused && /refused|unusable/i.test(errorText)

  return (
    <form
      onSubmit={submit}
      aria-label={t('connect.title')}
      className="flex flex-col gap-3 rounded-lg border border-border bg-card p-3"
    >
      <div className="flex flex-wrap items-end gap-3">
        <h2 className="mr-auto text-sm font-semibold">{t('connect.title')}</h2>
        <SegmentedControl<Mode>
          aria-label={t('connect.mode')}
          value={mode}
          onValueChange={setMode}
          options={[
            { value: 'token', label: t('connect.modeToken') },
            { value: 'kubeconfig', label: t('connect.modeKubeconfig') },
          ]}
        />
      </div>

      <div className="grid gap-3 md:grid-cols-[minmax(0,12rem)_1fr_1fr]">
        <div className="flex flex-col gap-1">
          <Label htmlFor="cluster-name">{t('connect.name')}</Label>
          <Input
            id="cluster-name"
            value={effectiveName}
            autoComplete="off"
            aria-invalid={effectiveName !== '' && !nameValid}
            onChange={(e) => {
              setName(e.target.value)
              setNameTouched(true)
            }}
          />
          {taken ? (
            <p className="text-xs text-destructive">{t('connect.nameTaken')}</p>
          ) : null}
        </div>
        {mode === 'token' ? (
          <>
            <div className="flex flex-col gap-1">
              <Label htmlFor="cluster-server">{t('connect.server')}</Label>
              <Input
                id="cluster-server"
                value={server}
                placeholder={t('connect.serverPlaceholder')}
                autoComplete="off"
                inputMode="url"
                onChange={(e) => {
                  setServer(e.target.value)
                }}
              />
            </div>
            <div className="flex flex-col gap-1">
              <Label htmlFor="cluster-token">{t('connect.token')}</Label>
              <SecretInput
                id="cluster-token"
                value={token}
                onChange={(e) => {
                  setToken(e.target.value)
                }}
              />
            </div>
          </>
        ) : (
          <div className="flex flex-col gap-1 md:col-span-2">
            <Label htmlFor="cluster-context">{t('connect.context')}</Label>
            <Input
              id="cluster-context"
              value={context}
              placeholder={t('connect.contextPlaceholder')}
              autoComplete="off"
              onChange={(e) => {
                setContext(e.target.value)
              }}
            />
          </div>
        )}
      </div>

      {mode === 'token' ? (
        <div className="flex flex-col gap-1">
          <Label htmlFor="cluster-ca">{t('connect.ca')}</Label>
          <Textarea
            id="cluster-ca"
            value={ca}
            rows={2}
            spellCheck={false}
            autoComplete="off"
            className="min-h-10 font-mono text-xs"
            placeholder={t('connect.caPlaceholder')}
            onChange={(e) => {
              setCa(e.target.value)
            }}
          />
          <p className="text-xs text-muted-foreground">{t('connect.caHint')}</p>
        </div>
      ) : (
        <div className="flex flex-col gap-1">
          <Label htmlFor="cluster-kubeconfig">{t('connect.kubeconfig')}</Label>
          <Textarea
            id="cluster-kubeconfig"
            value={kubeconfig}
            rows={4}
            spellCheck={false}
            autoComplete="off"
            className="font-mono text-xs"
            placeholder={t('connect.kubeconfigPlaceholder')}
            onChange={(e) => {
              setKubeconfig(e.target.value)
            }}
          />
          <p className="text-xs text-muted-foreground">
            {t('connect.kubeconfigHint')}
          </p>
        </div>
      )}

      {connect.isError ? (
        <div role="alert" className="text-sm text-destructive">
          <p>{errorText}</p>
          {blocked ? (
            <p className="text-xs text-muted-foreground">
              {t('privateApiBlocked')}
            </p>
          ) : null}
        </div>
      ) : null}

      <div className="flex flex-col gap-1">
        <label
          htmlFor="cluster-allow-admin"
          className="flex items-center gap-2 text-sm"
        >
          <Checkbox
            id="cluster-allow-admin"
            checked={allowAdmin}
            onCheckedChange={(checked) => {
              setAllowAdmin(checked === true)
            }}
          />
          {t('connect.allowClusterAdmin')}
        </label>
        {allowAdmin ? (
          <p role="note" className="text-xs text-destructive">
            {t('connect.allowClusterAdminWarning')}
          </p>
        ) : null}
      </div>

      <div className="flex justify-end">
        <Button type="submit" size="sm" disabled={!canSubmit}>
          <PlusGlassIcon size="1.2em" aria-hidden="true" />
          {connect.isPending ? t('connect.pending') : t('connect.submit')}
        </Button>
      </div>
    </form>
  )
}
