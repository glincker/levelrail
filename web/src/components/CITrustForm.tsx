import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { useIsRoot } from '../hooks/useIsRoot'
import { CI_PROVIDERS, type CIProvider } from '../lib/ciProviders'
import { useAddCITrust, type CITrustInput } from '../queries/ciDeploy'

const DEFAULT_REF = 'refs/heads/main'
const FIELD = 'pointer-coarse:min-h-11'

interface Fields {
  repository: string
  refPattern: string
  environment: string
  orgId: string
  issuer: string
  protectedOnly: boolean
}

const EMPTY: Fields = {
  repository: '',
  refPattern: DEFAULT_REF,
  environment: '',
  orgId: '',
  issuer: '',
  protectedOnly: true,
}

function buildInput(
  provider: CIProvider,
  f: Fields,
  isRoot: boolean,
): CITrustInput {
  const base = {
    provider,
    repository: f.repository.trim(),
    ref_pattern: f.refPattern.trim(),
    environment: f.environment.trim(),
  }
  if (provider === 'circleci') return { ...base, org_id: f.orgId.trim() }
  if (provider !== 'gitlab') return base
  const issuer = isRoot ? f.issuer.trim() : ''
  return {
    ...base,
    protected_ref_only: f.protectedOnly,
    ...(issuer === '' ? {} : { issuer }),
  }
}

function canSubmit(provider: CIProvider, f: Fields): boolean {
  if (f.repository.trim() === '' || f.refPattern.trim() === '') return false
  return provider !== 'circleci' || f.orgId.trim() !== ''
}

export function CIProviderPicker({
  provider,
  onChange,
}: {
  provider: CIProvider
  onChange: (next: CIProvider) => void
}) {
  const { t } = useTranslation('deploys')
  return (
    <div
      role="group"
      aria-label={t('ciDeploy.providerLabel')}
      className="flex flex-wrap gap-1.5"
    >
      {CI_PROVIDERS.map((p) => (
        <Button
          key={p}
          type="button"
          size="sm"
          variant={p === provider ? 'default' : 'outline'}
          aria-pressed={p === provider}
          onClick={() => {
            onChange(p)
          }}
        >
          {t(`ciDeploy.providers.${p}`)}
        </Button>
      ))}
    </div>
  )
}

function FieldBox({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="grid gap-1">
      <Label className="text-xs text-muted-foreground">{label}</Label>
      {children}
    </div>
  )
}

export function CITrustForm({
  appName,
  provider,
}: {
  appName: string
  provider: CIProvider
}) {
  const { t } = useTranslation('deploys')
  const add = useAddCITrust(appName)
  const isRoot = useIsRoot()
  const [fields, setFields] = useState<Fields>(EMPTY)

  function set<K extends keyof Fields>(key: K, value: Fields[K]) {
    setFields((prev) => ({ ...prev, [key]: value }))
  }

  function submit() {
    add.mutate(buildInput(provider, fields, isRoot), {
      onSuccess: (trust) => {
        setFields((prev) => ({ ...prev, repository: '' }))
        if (trust.warning) {
          toast.add({
            title: t('ciDeploy.toast.trustedLoose'),
            description: trust.warning,
            type: 'warning',
          })
          return
        }
        toast.add({ title: t('ciDeploy.toast.trusted'), type: 'success' })
      },
      onError: (error) => {
        toast.add({
          title: t('ciDeploy.toast.trustError'),
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  return (
    <div className="space-y-2">
      <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
        {provider === 'circleci' && (
          <FieldBox label={t('ciDeploy.orgIdLabel')}>
            <Input
              className={FIELD}
              value={fields.orgId}
              placeholder={t('ciDeploy.orgIdPlaceholder')}
              onChange={(e) => {
                set('orgId', e.target.value)
              }}
            />
          </FieldBox>
        )}
        <FieldBox label={t(`ciDeploy.repositoryLabel.${provider}`)}>
          <Input
            className={FIELD}
            value={fields.repository}
            placeholder={t(`ciDeploy.repositoryPlaceholder.${provider}`)}
            onChange={(e) => {
              set('repository', e.target.value)
            }}
          />
        </FieldBox>
        <FieldBox label={t('ciDeploy.refLabel')}>
          <Input
            className={FIELD}
            value={fields.refPattern}
            onChange={(e) => {
              set('refPattern', e.target.value)
            }}
          />
        </FieldBox>
        <FieldBox label={t(`ciDeploy.environmentLabel.${provider}`)}>
          <Input
            className={FIELD}
            value={fields.environment}
            placeholder={t(`ciDeploy.environmentPlaceholder.${provider}`)}
            onChange={(e) => {
              set('environment', e.target.value)
            }}
          />
        </FieldBox>
        {provider === 'gitlab' && isRoot && (
          <FieldBox label={t('ciDeploy.issuerLabel')}>
            <Input
              className={FIELD}
              value={fields.issuer}
              placeholder={t('ciDeploy.issuerPlaceholder')}
              onChange={(e) => {
                set('issuer', e.target.value)
              }}
            />
          </FieldBox>
        )}
      </div>
      {provider === 'gitlab' && (
        <div className="space-y-2">
          {isRoot && (
            <p className="text-xs text-muted-foreground">
              {t('ciDeploy.issuerNote')}
            </p>
          )}
          <Label className="flex items-center gap-2 pointer-coarse:min-h-11">
            <Switch
              checked={fields.protectedOnly}
              onCheckedChange={(checked) => {
                set('protectedOnly', checked)
              }}
            />
            {t('ciDeploy.protectedOnly')}
          </Label>
          {!fields.protectedOnly && (
            <Alert variant="destructive">
              <TriangleAlertGlassIcon
                className="text-tone-warning"
                size="1.7em"
              />
              <AlertDescription>
                {t('ciDeploy.unprotectedWarning')}
              </AlertDescription>
            </Alert>
          )}
        </div>
      )}
      <Button
        type="button"
        onClick={submit}
        className="pointer-coarse:min-h-11"
        disabled={!canSubmit(provider, fields) || add.isPending}
      >
        {t('ciDeploy.trust')}
      </Button>
    </div>
  )
}
