import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { SecretInput } from '@/components/ui/secret-input'
import { Badge } from '@/components/ui/badge'
import { ConfirmAction } from '@/components/ui/confirm-action'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  useDeleteSecretProvider,
  usePutSecretProvider,
  useSecretProviders,
  useTestSecretProvider,
  type SecretProvider,
  type SecretProviderType,
} from '../../queries/secretProviders'

// Mirrors the server's name rule so bad names fail before the round trip.
const NAME_RE = /^[a-z][a-z0-9_]{0,39}$/

const TYPE_OPTIONS: { value: SecretProviderType; label: string }[] = [
  { value: 'doppler', label: 'Doppler' },
  { value: 'infisical', label: 'Infisical' },
]

function ProviderForm({
  initial,
  onDone,
}: {
  initial: SecretProvider | null
  onDone: () => void
}) {
  const { t } = useTranslation('settings')
  const put = usePutSecretProvider()
  const [name, setName] = useState(initial?.name ?? '')
  const [type, setType] = useState<SecretProviderType>(
    initial?.type ?? 'doppler',
  )
  const [project, setProject] = useState(initial?.project ?? '')
  const [environment, setEnvironment] = useState(initial?.environment ?? '')
  const [path, setPath] = useState(initial?.path ?? '')
  const [baseUrl, setBaseUrl] = useState(initial?.base_url ?? '')
  const [token, setToken] = useState('')
  const [error, setError] = useState<string | null>(null)
  const isDoppler = type === 'doppler'

  function submit(e: FormEvent) {
    e.preventDefault()
    if (!NAME_RE.test(name)) {
      setError(t('secretProviders.errName'))
      return
    }
    if (!project.trim() || !environment.trim()) {
      setError(t('secretProviders.errScope'))
      return
    }
    if (!initial?.has_token && !token.trim()) {
      setError(t('secretProviders.errToken'))
      return
    }
    setError(null)
    put.mutate(
      {
        name,
        body: {
          type,
          project: project.trim(),
          environment: environment.trim(),
          path: path.trim() || undefined,
          base_url: !isDoppler ? baseUrl.trim() || undefined : undefined,
          token: token.trim() || undefined,
        },
      },
      {
        onSuccess: () => {
          setToken('')
          onDone()
        },
      },
    )
  }

  const message = error ?? (put.isError ? put.error.message : null)

  return (
    <form
      onSubmit={submit}
      className="space-y-3"
      aria-label={t('secretProviders.formLabel')}
    >
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <Field>
          <FieldLabel htmlFor="sp-name">{t('secretProviders.name')}</FieldLabel>
          <Input
            id="sp-name"
            aria-invalid={message ? true : undefined}
            aria-describedby={message ? 'sp-error' : undefined}
            value={name}
            disabled={initial !== null || put.isPending}
            autoComplete="off"
            placeholder={t('secretProviders.namePlaceholder')}
            onChange={(e) => {
              setName(e.target.value)
            }}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="sp-type">{t('secretProviders.type')}</FieldLabel>
          <Select
            value={type}
            disabled={put.isPending}
            onValueChange={(v) => {
              setType(v as SecretProviderType)
            }}
          >
            <SelectTrigger id="sp-type">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {TYPE_OPTIONS.map((o) => (
                <SelectItem key={o.value} value={o.value}>
                  {o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel htmlFor="sp-project">
            {isDoppler
              ? t('secretProviders.projectDoppler')
              : t('secretProviders.projectInfisical')}
          </FieldLabel>
          <Input
            id="sp-project"
            value={project}
            disabled={put.isPending}
            autoComplete="off"
            onChange={(e) => {
              setProject(e.target.value)
            }}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="sp-env">
            {isDoppler
              ? t('secretProviders.envDoppler')
              : t('secretProviders.envInfisical')}
          </FieldLabel>
          <Input
            id="sp-env"
            value={environment}
            disabled={put.isPending}
            autoComplete="off"
            onChange={(e) => {
              setEnvironment(e.target.value)
            }}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="sp-path">{t('secretProviders.path')}</FieldLabel>
          <Input
            id="sp-path"
            value={path}
            disabled={put.isPending}
            autoComplete="off"
            placeholder="/"
            onChange={(e) => {
              setPath(e.target.value)
            }}
          />
        </Field>
        {!isDoppler ? (
          <Field>
            <FieldLabel htmlFor="sp-base">
              {t('secretProviders.baseUrl')}
            </FieldLabel>
            <Input
              id="sp-base"
              value={baseUrl}
              disabled={put.isPending}
              autoComplete="off"
              placeholder="https://infisical.internal"
              onChange={(e) => {
                setBaseUrl(e.target.value)
              }}
            />
          </Field>
        ) : null}
        <Field className="sm:col-span-2">
          <FieldLabel htmlFor="sp-token">
            {t('secretProviders.token')}
          </FieldLabel>
          <SecretInput
            id="sp-token"
            value={token}
            hasExistingValue={initial?.has_token}
            disabled={put.isPending}
            onChange={(e) => {
              setToken(e.target.value)
            }}
          />
        </Field>
      </div>
      {message ? (
        <p id="sp-error" role="alert" className="text-destructive text-sm">
          {message}
        </p>
      ) : null}
      <div className="flex gap-2">
        <Button type="submit" size="sm" disabled={put.isPending}>
          {put.isPending
            ? t('secretProviders.saving')
            : t('secretProviders.save')}
        </Button>
        <Button type="button" size="sm" variant="ghost" onClick={onDone}>
          {t('secretProviders.cancel')}
        </Button>
      </div>
    </form>
  )
}

function ProviderRow({
  provider,
  onEdit,
}: {
  provider: SecretProvider
  onEdit: () => void
}) {
  const { t } = useTranslation('settings')
  const test = useTestSecretProvider()
  const del = useDeleteSecretProvider()
  const scheme = provider.type

  return (
    <li className="flex flex-wrap items-center gap-x-3 gap-y-2 py-2">
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-medium">{provider.name}</span>
          <Badge variant="muted">{provider.type}</Badge>
          {provider.self_hosted ? (
            <Badge variant="outline">{t('secretProviders.selfHosted')}</Badge>
          ) : null}
          {!provider.has_token ? (
            <Badge variant="destructive">{t('secretProviders.noToken')}</Badge>
          ) : null}
        </div>
        <p className="text-muted-foreground truncate text-xs">
          {provider.project} / {provider.environment}
          {provider.path ? ` ${provider.path}` : ''}
          {' · '}
          <code>{`${scheme}://${provider.name}/SECRET`}</code>
        </p>
        {test.data ? (
          <p
            role="status"
            className={
              test.data.ok
                ? 'text-xs text-emerald-600'
                : 'text-destructive text-xs'
            }
          >
            {test.data.ok
              ? t('secretProviders.testOk')
              : (test.data.reason ?? t('secretProviders.testFail'))}
          </p>
        ) : null}
        {test.isError ? (
          <p role="alert" className="text-destructive text-xs">
            {test.error.message}
          </p>
        ) : null}
      </div>
      <div className="flex gap-1">
        <Button
          size="xs"
          variant="outline"
          disabled={test.isPending}
          onClick={() => {
            test.mutate(provider.name)
          }}
        >
          {test.isPending
            ? t('secretProviders.testing')
            : t('secretProviders.test')}
        </Button>
        <Button size="xs" variant="outline" onClick={onEdit}>
          {t('secretProviders.edit')}
        </Button>
        <ConfirmAction
          trigger={(open) => (
            <Button size="xs" variant="destructive" onClick={open}>
              {t('secretProviders.remove')}
            </Button>
          )}
          title={t('secretProviders.removeTitle', { name: provider.name })}
          consequences={[t('secretProviders.removeConsequence')]}
          confirmLabel={t('secretProviders.remove')}
          pending={del.isPending}
          error={del.isError ? del.error.message : null}
          onConfirm={(close) => {
            del.mutate(provider.name, { onSuccess: close })
          }}
        />
      </div>
    </li>
  )
}

export default function SecretProvidersSection() {
  const { t } = useTranslation('settings')
  const { data, isPending, isError, error } = useSecretProviders()
  const [editing, setEditing] = useState<SecretProvider | 'new' | null>(null)

  return (
    <Card>
      <CardContent className="space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div>
            <h2 className="text-base font-semibold">
              {t('secretProviders.title')}
            </h2>
            <p className="text-muted-foreground text-sm">
              {t('secretProviders.hint')} <code>doppler://NAME/SECRET</code>{' '}
              <code>infisical://NAME/[dir/]SECRET</code>
            </p>
          </div>
          {editing === null ? (
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                setEditing('new')
              }}
            >
              {t('secretProviders.add')}
            </Button>
          ) : null}
        </div>
        {editing !== null ? (
          <ProviderForm
            key={editing === 'new' ? 'new' : editing.name}
            initial={editing === 'new' ? null : editing}
            onDone={() => {
              setEditing(null)
            }}
          />
        ) : null}
        {isPending ? (
          <p className="text-muted-foreground text-sm">
            {t('secretProviders.loading')}
          </p>
        ) : isError ? (
          <p role="alert" className="text-destructive text-sm">
            {error.message}
          </p>
        ) : data.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t('secretProviders.empty')}
          </p>
        ) : (
          <ul className="divide-border divide-y">
            {data.map((p) => (
              <ProviderRow
                key={p.name}
                provider={p}
                onEdit={() => {
                  setEditing(p)
                }}
              />
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
