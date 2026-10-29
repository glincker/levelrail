import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  PlugGlassIcon,
  PlusGlassIcon,
  TrashGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import { BrandTile } from '../BrandLogoBadge'
import { UrlDialog } from '../kit/UrlDialog'
import { useDialogParam, useUrlDialog } from '../kit/useUrlDialog'
import {
  useCreatePackageRegistry,
  useDeletePackageRegistry,
  usePackageRegistries,
  useTestPackageRegistry,
} from '../../queries/registryVendors'
import type {
  ConnectionTestResult,
  PackageEcosystem,
  PackageRegistry,
  PackageRegistryScope,
} from '../../types/registryVendor'

export const PACKAGE_DIALOG_VALUE = 'package-registry'
const DIALOG_PARAMS = ['ecosystem'] as const

const ECOSYSTEMS: readonly PackageEcosystem[] = [
  'npm',
  'pypi',
  'maven',
  'nuget',
  'cargo',
  'go',
  'rubygems',
]

const ECOSYSTEM_BRAND: Record<PackageEcosystem, string> = {
  npm: 'npm',
  pypi: 'pypi',
  maven: 'maven',
  nuget: 'nuget',
  cargo: 'cargo',
  go: 'go',
  rubygems: 'rubygems',
}

const ECOSYSTEM_LABEL_KEYS = {
  npm: 'packageRegistries.ecosystems.npm',
  pypi: 'packageRegistries.ecosystems.pypi',
  maven: 'packageRegistries.ecosystems.maven',
  nuget: 'packageRegistries.ecosystems.nuget',
  cargo: 'packageRegistries.ecosystems.cargo',
  go: 'packageRegistries.ecosystems.go',
  rubygems: 'packageRegistries.ecosystems.rubygems',
} as const

const SCOPE_LABEL_KEYS = {
  instance: 'packageRegistries.scopes.instance',
  project: 'packageRegistries.scopes.project',
  app: 'packageRegistries.scopes.app',
} as const

function isEcosystem(v: string | undefined): v is PackageEcosystem {
  return ECOSYSTEMS.some((e) => e === v)
}

function AddForm({ onDone }: { onDone: () => void }) {
  const { t } = useTranslation('settings')
  const [rawEcosystem, setEcosystem] = useDialogParam('ecosystem')
  const ecosystem: PackageEcosystem = isEcosystem(rawEcosystem)
    ? rawEcosystem
    : 'npm'
  const [name, setName] = useState('')
  const [url, setUrl] = useState('')
  const [username, setUsername] = useState('')
  const [secret, setSecret] = useState('')
  const [patterns, setPatterns] = useState('')
  const [scope, setScope] = useState<PackageRegistryScope>('instance')
  const [scopeRef, setScopeRef] = useState('')
  const create = useCreatePackageRegistry()
  const ready =
    name.trim() !== '' &&
    url.trim() !== '' &&
    secret !== '' &&
    (scope === 'instance' || scopeRef.trim() !== '')

  function submit(e: React.FormEvent) {
    e.preventDefault()
    if (!ready) return
    create.mutate(
      {
        name: name.trim(),
        ecosystem,
        url: url.trim(),
        username: username.trim() || undefined,
        secret,
        module_patterns: ecosystem === 'go' ? patterns.trim() : undefined,
        scope_kind: scope,
        scope_ref: scope === 'instance' ? undefined : scopeRef.trim(),
      },
      {
        onSuccess: (created) => {
          toast.add({
            title: t('packageRegistries.saved', { name: created.name }),
            type: 'success',
          })
          onDone()
        },
      },
    )
  }

  return (
    <form onSubmit={submit} className="space-y-4">
      <div role="radiogroup" className="flex flex-wrap gap-2">
        {ECOSYSTEMS.map((e) => (
          <button
            key={e}
            type="button"
            role="radio"
            aria-checked={e === ecosystem}
            onClick={() => setEcosystem(e)}
            className={cn(
              'flex items-center gap-2 rounded-lg border px-2.5 py-1.5 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring',
              e === ecosystem
                ? 'border-primary bg-primary/5'
                : 'border-border hover:bg-muted/50',
            )}
          >
            <BrandTile
              name={ECOSYSTEM_BRAND[e]}
              label={t(ECOSYSTEM_LABEL_KEYS[e])}
              size="xs"
            />
            {t(ECOSYSTEM_LABEL_KEYS[e])}
          </button>
        ))}
      </div>
      <FieldGroup className="gap-4">
        <Field>
          <FieldLabel htmlFor="pkg-name">
            {t('packageRegistries.name')}
          </FieldLabel>
          <Input
            id="pkg-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="pkg-url">
            {t('packageRegistries.url')}
          </FieldLabel>
          <Input
            id="pkg-url"
            className="font-mono"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            placeholder="https://registry.example.com/repo"
          />
          <FieldDescription>{t('packageRegistries.urlHelp')}</FieldDescription>
        </Field>
        <Field>
          <FieldLabel htmlFor="pkg-username">
            {t('packageRegistries.username')}
          </FieldLabel>
          <Input
            id="pkg-username"
            autoComplete="off"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
          />
          <FieldDescription>
            {t('packageRegistries.usernameHelp')}
          </FieldDescription>
        </Field>
        <Field>
          <FieldLabel htmlFor="pkg-secret">
            {t('packageRegistries.secret')}
          </FieldLabel>
          <Input
            id="pkg-secret"
            type="password"
            autoComplete="off"
            className="font-mono"
            value={secret}
            onChange={(e) => setSecret(e.target.value)}
          />
          <FieldDescription>
            {t('packageRegistries.secretHelp')}
          </FieldDescription>
        </Field>
        {ecosystem === 'go' ? (
          <Field>
            <FieldLabel htmlFor="pkg-patterns">
              {t('packageRegistries.patterns')}
            </FieldLabel>
            <Input
              id="pkg-patterns"
              className="font-mono"
              value={patterns}
              onChange={(e) => setPatterns(e.target.value)}
              placeholder="example.com/*"
            />
          </Field>
        ) : null}
        <Field>
          <FieldLabel>{t('packageRegistries.scope')}</FieldLabel>
          <div role="radiogroup" className="flex flex-wrap gap-2">
            {(['instance', 'project', 'app'] as const).map((s) => (
              <button
                key={s}
                type="button"
                role="radio"
                aria-checked={s === scope}
                onClick={() => setScope(s)}
                className={cn(
                  'rounded-lg border px-2.5 py-1.5 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring',
                  s === scope
                    ? 'border-primary bg-primary/5'
                    : 'border-border hover:bg-muted/50',
                )}
              >
                {t(SCOPE_LABEL_KEYS[s])}
              </button>
            ))}
          </div>
          {scope !== 'instance' ? (
            <Input
              aria-label={t('packageRegistries.scopeRef')}
              value={scopeRef}
              onChange={(e) => setScopeRef(e.target.value)}
              placeholder={t('packageRegistries.scopeRefPlaceholder')}
            />
          ) : null}
        </Field>
      </FieldGroup>
      <p className="rounded-lg border border-border bg-muted/30 p-3 text-xs text-muted-foreground">
        {t('packageRegistries.buildNote')}
      </p>
      {create.isError ? (
        <Alert variant="destructive">
          <TriangleAlertGlassIcon className="text-tone-warning" size="1.7em" />
          <AlertDescription>{create.error.message}</AlertDescription>
        </Alert>
      ) : null}
      <Button type="submit" disabled={!ready || create.isPending}>
        {create.isPending
          ? t('packageRegistries.saving')
          : t('packageRegistries.save')}
      </Button>
    </form>
  )
}

function Row({ registry }: { registry: PackageRegistry }) {
  const { t } = useTranslation('settings')
  const test = useTestPackageRegistry()
  const remove = useDeletePackageRegistry()
  const [result, setResult] = useState<ConnectionTestResult | null>(null)
  const eco = isEcosystem(registry.ecosystem) ? registry.ecosystem : 'npm'
  return (
    <li className="space-y-1 py-3">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <BrandTile
          name={ECOSYSTEM_BRAND[eco]}
          label={t(ECOSYSTEM_LABEL_KEYS[eco])}
          labelled
        />
        <div className="min-w-0 flex-1 basis-56">
          <p className="text-sm font-medium">{registry.name}</p>
          <p className="truncate font-mono text-xs text-muted-foreground">
            {registry.url}
          </p>
          <p className="text-xs text-muted-foreground">
            {t(SCOPE_LABEL_KEYS[registry.scope_kind])}
            {registry.scope_ref ? `: ${registry.scope_ref}` : ''}
            {' · '}
            {t('packageRegistries.mountAs', { id: registry.build_secret_id })}
          </p>
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={test.isPending}
          onClick={() => {
            setResult(null)
            test.mutate(registry.id, { onSuccess: setResult })
          }}
        >
          <PlugGlassIcon size="1.4em" aria-hidden="true" />
          {t('packageRegistries.test')}
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="text-destructive"
          disabled={remove.isPending}
          aria-label={t('packageRegistries.remove', { name: registry.name })}
          onClick={() => remove.mutate(registry.id)}
        >
          <TrashGlassIcon size="1.4em" aria-hidden="true" />
        </Button>
      </div>
      {result ? (
        <Alert variant={result.ok ? 'default' : 'destructive'}>
          {result.ok ? (
            <CircleCheckGlassIcon className="text-tone-success" size="1.4em" />
          ) : (
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.7em"
            />
          )}
          <AlertDescription>{result.message}</AlertDescription>
        </Alert>
      ) : null}
    </li>
  )
}

// Private package registries for builds: credentials mounted into
// Dockerfile builds as BuildKit secrets, never as build args or layers.
export function PackageRegistriesCard() {
  const { t } = useTranslation('settings')
  const list = usePackageRegistries()
  const triggerRef = useRef<HTMLButtonElement>(null)
  const dialog = useUrlDialog(PACKAGE_DIALOG_VALUE, 'connect', DIALOG_PARAMS)
  return (
    <section aria-labelledby="package-registries-heading" className="space-y-3">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <h2 id="package-registries-heading" className="text-sm font-medium">
            {t('packageRegistries.heading')}
          </h2>
          <p className="text-xs text-muted-foreground">
            {t('packageRegistries.subheading')}
          </p>
        </div>
        <Button
          ref={triggerRef}
          type="button"
          size="sm"
          onClick={() => dialog.open()}
        >
          <PlusGlassIcon size="1.4em" aria-hidden="true" />
          {t('packageRegistries.add')}
        </Button>
      </div>
      <Card>
        <CardContent>
          {list.isError ? (
            <Alert variant="destructive">
              <TriangleAlertGlassIcon
                className="text-tone-warning"
                size="1.7em"
              />
              <AlertDescription>{list.error.message}</AlertDescription>
            </Alert>
          ) : !list.data || list.data.length === 0 ? (
            <p className="py-6 text-center text-sm text-muted-foreground">
              {t('packageRegistries.empty')}
            </p>
          ) : (
            <ul className="divide-y divide-border">
              {list.data.map((r) => (
                <Row key={r.id} registry={r} />
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
      <UrlDialog
        value={PACKAGE_DIALOG_VALUE}
        ownedParams={DIALOG_PARAMS}
        returnFocusRef={triggerRef}
        title={t('packageRegistries.dialogTitle')}
        description={t('packageRegistries.dialogDescription')}
      >
        <AddForm onDone={dialog.close} />
      </UrlDialog>
    </section>
  )
}
