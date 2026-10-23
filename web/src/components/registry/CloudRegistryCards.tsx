import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowsClockwiseIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { BrandIcon, type BrandIconName } from '../BrandIcon'
import {
  useCreateRegistryCredential,
  useRefreshRegistryCredential,
  useTestRegistryPreset,
} from '../../queries/registryCredentials'
import type {
  RegistryCredential,
  RegistryPreset,
} from '../../types/registryCredential'

interface PresetSpec {
  preset: RegistryPreset
  brand: BrandIconName
  defaultName: string
  defaultRegion: string
  needsHost: boolean
}

const PRESETS: PresetSpec[] = [
  {
    preset: 'ecr',
    brand: 'ecr',
    defaultName: 'ecr',
    defaultRegion: 'us-east-1',
    needsHost: false,
  },
  {
    preset: 'artifact_registry',
    brand: 'google-artifact-registry',
    defaultName: 'artifact-registry',
    defaultRegion: 'us',
    needsHost: false,
  },
  {
    preset: 'acr',
    brand: 'azure-container-registry',
    defaultName: 'acr',
    defaultRegion: '',
    needsHost: true,
  },
]

function formatTime(value?: string): string | null {
  if (!value) return null
  return new Date(value).toLocaleString()
}

function ConnectedRow({ credential }: { credential: RegistryCredential }) {
  const { t } = useTranslation('settings')
  const refresh = useRefreshRegistryCredential()
  const failed = credential.last_refresh_error
  return (
    <div className="space-y-1 border-t pt-3 text-xs">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium">{credential.name}</span>
        <Badge variant={failed ? 'destructive' : 'success'}>
          {failed
            ? t('registryPage.cloud.refreshFailing')
            : t('registryPage.cloud.refreshHealthy')}
        </Badge>
        <Button
          size="sm"
          variant={failed ? 'default' : 'outline'}
          disabled={refresh.isPending}
          onClick={() => refresh.mutate(credential.id)}
        >
          <ArrowsClockwiseIcon aria-hidden="true" className="size-4" />
          {t('registryPage.cloud.refreshNow')}
        </Button>
      </div>
      <p className="text-muted-foreground">
        {t('registryPage.cloud.expires', {
          when: formatTime(credential.expires_at) ?? '-',
        })}
        {' · '}
        {t('registryPage.cloud.lastRefresh', {
          when: formatTime(credential.last_refreshed_at) ?? '-',
        })}
        {' · '}
        {t('registryPage.cloud.nextRefresh', {
          when: formatTime(credential.next_refresh_at) ?? '-',
        })}
      </p>
      {failed && <p className="text-destructive">{failed}</p>}
      {refresh.error && (
        <p className="text-destructive">{refresh.error.message}</p>
      )}
    </div>
  )
}

function PresetCard({
  spec,
  connected,
}: {
  spec: PresetSpec
  connected: RegistryCredential[]
}) {
  const { t } = useTranslation('settings')
  const [name, setName] = useState(spec.defaultName)
  const [region, setRegion] = useState(spec.defaultRegion)
  const [host, setHost] = useState('')
  const test = useTestRegistryPreset()
  const create = useCreateRegistryCredential()

  const request = {
    preset: spec.preset,
    region: region || undefined,
    registry_host: host || undefined,
  }
  const missingHost = spec.needsHost && host.trim() === ''
  const regionLabel = spec.needsHost
    ? t('registryPage.cloud.hostLabel')
    : t('registryPage.cloud.regionLabel')

  return (
    <Card>
      <CardContent className="space-y-3">
        <div className="flex items-center gap-3">
          <BrandIcon name={spec.brand} aria-hidden="true" className="size-6" />
          <div className="min-w-0 flex-1">
            <h3 className="text-sm font-medium">
              {t(`registryPage.cloud.${spec.preset}.title`)}
            </h3>
            <p className="text-xs text-muted-foreground">
              {t(`registryPage.cloud.${spec.preset}.body`)}
            </p>
          </div>
        </div>
        <div className="grid gap-2 sm:grid-cols-2">
          <label className="space-y-1 text-xs">
            <span>{t('registryPage.cloud.nameLabel')}</span>
            <Input value={name} onChange={(e) => setName(e.target.value)} />
          </label>
          {spec.needsHost ? (
            <label className="space-y-1 text-xs">
              <span>{regionLabel}</span>
              <Input
                value={host}
                placeholder={t('registryPage.cloud.hostPlaceholder')}
                onChange={(e) => setHost(e.target.value)}
              />
            </label>
          ) : (
            <label className="space-y-1 text-xs">
              <span>{regionLabel}</span>
              <Input
                value={region}
                onChange={(e) => setRegion(e.target.value)}
              />
            </label>
          )}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button
            size="sm"
            variant="outline"
            disabled={test.isPending || missingHost}
            onClick={() => test.mutate(request)}
          >
            {t('registryPage.cloud.test')}
          </Button>
          <Button
            size="sm"
            disabled={create.isPending || missingHost || name.trim() === ''}
            onClick={() => create.mutate({ name: name.trim(), ...request })}
          >
            {t('registryPage.cloud.connect')}
          </Button>
          {test.data && (
            <span className="text-xs text-muted-foreground">
              {t('registryPage.cloud.testOk', {
                host: test.data.registry_host,
                when: formatTime(test.data.expires_at) ?? '-',
              })}
            </span>
          )}
        </div>
        {(test.error ?? create.error) && (
          <p className="text-xs text-destructive">
            {(test.error ?? create.error)?.message}
          </p>
        )}
        {connected.map((c) => (
          <ConnectedRow key={c.id} credential={c} />
        ))}
      </CardContent>
    </Card>
  )
}

export function CloudRegistryCards({
  credentials,
}: {
  credentials: RegistryCredential[]
}) {
  const { t } = useTranslation('settings')
  return (
    <section className="space-y-3">
      <div>
        <h2 className="text-sm font-medium">{t('registryPage.cloud.title')}</h2>
        <p className="text-xs text-muted-foreground">
          {t('registryPage.cloud.description')}
        </p>
      </div>
      <div className="grid gap-4 lg:grid-cols-3">
        {PRESETS.map((spec) => (
          <PresetCard
            key={spec.preset}
            spec={spec}
            connected={credentials.filter((c) => c.preset === spec.preset)}
          />
        ))}
      </div>
    </section>
  )
}

export default CloudRegistryCards
