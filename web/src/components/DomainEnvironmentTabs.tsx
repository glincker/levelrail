import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { PlusIcon, XIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { toast } from '@/components/ui/toast'
import {
  useAppEnvironmentDomains,
  useEditAppDomains,
  type EnvironmentDomainSet,
} from '../queries/appEnvironmentDomains'
import { isValidDomain } from '../lib/domainWizard'

const DEFAULT_TAB = 'default'

function EnvironmentSet({
  appName,
  set,
}: {
  appName: string
  set: EnvironmentDomainSet
}) {
  const { t } = useTranslation('domains')
  const [value, setValue] = useState('')
  const edit = useEditAppDomains(appName)
  const next = value.trim().toLowerCase()

  function run(input: { add?: string[]; remove?: string[] }) {
    edit.mutate(
      { ...input, environment: set.environment_id },
      {
        onSuccess: () => {
          setValue('')
          toast.add({ title: t('environmentTabs.saved'), type: 'success' })
        },
      },
    )
  }

  return (
    <div className="space-y-3">
      <p className="text-xs text-muted-foreground">
        {set.active
          ? t('environmentTabs.activeHint')
          : t('environmentTabs.inactiveHint')}
      </p>
      {set.domains.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('environmentTabs.empty')}
        </p>
      ) : (
        <ul className="space-y-1.5">
          {set.domains.map((d) => (
            <li
              key={d}
              className="flex items-center gap-2 rounded-md border border-border px-2 py-1"
            >
              <span className="min-w-0 flex-1 truncate font-mono text-sm">
                {d}
              </span>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                disabled={edit.isPending}
                onClick={() => {
                  run({ remove: [d] })
                }}
              >
                <XIcon aria-hidden="true" />
                <span className="sr-only">
                  {t('environmentTabs.remove', { domain: d })}
                </span>
              </Button>
            </li>
          ))}
        </ul>
      )}
      <form
        className="flex items-center gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          if (isValidDomain(next)) run({ add: [next] })
        }}
      >
        <Input
          className="font-mono"
          value={value}
          placeholder="app.example.com"
          aria-label={t('environmentTabs.addLabel')}
          onChange={(e) => {
            setValue(e.target.value)
          }}
        />
        <Button
          type="submit"
          variant="outline"
          size="sm"
          disabled={!isValidDomain(next) || edit.isPending}
        >
          <PlusIcon aria-hidden="true" />
          {t('environmentTabs.add')}
        </Button>
      </form>
      {edit.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{edit.error.message}</AlertDescription>
        </Alert>
      ) : null}
    </div>
  )
}

// Wraps the app's default domain editor with one tab per environment, so
// dev, uat and production can each carry their own hostnames on one app.
// With no environments to choose from it renders the default editor alone.
export function DomainEnvironmentTabs({
  appName,
  children,
}: {
  appName: string
  children: ReactNode
}) {
  const { t } = useTranslation('domains')
  const { data, isError } = useAppEnvironmentDomains(appName)
  const [tab, setTab] = useState(DEFAULT_TAB)

  if (isError) {
    return (
      <div className="space-y-3">
        <Alert variant="destructive">
          <AlertDescription>{t('environmentTabs.loadFailed')}</AlertDescription>
        </Alert>
        {children}
      </div>
    )
  }
  if (!data || data.environments.length === 0) {
    return <>{children}</>
  }

  return (
    <Tabs value={tab} onValueChange={setTab}>
      <TabsList aria-label={t('environmentTabs.label')}>
        <TabsTrigger value={DEFAULT_TAB}>
          {t('environmentTabs.default')}
        </TabsTrigger>
        {data.environments.map((env) => (
          <TabsTrigger key={env.environment_id} value={env.environment_id}>
            {env.name}
            {env.active ? (
              <Badge variant="success" className="ml-1.5">
                {t('environmentTabs.active')}
              </Badge>
            ) : null}
          </TabsTrigger>
        ))}
      </TabsList>
      <TabsContent value={DEFAULT_TAB} className="space-y-3 pt-3">
        <p className="text-xs text-muted-foreground">
          {t('environmentTabs.defaultHint')} {t('environmentTabs.routingNow')}:{' '}
          <span className="font-mono">
            {data.routed_domains.join(', ') || '-'}
          </span>
        </p>
        {children}
      </TabsContent>
      {data.environments.map((env) => (
        <TabsContent
          key={env.environment_id}
          value={env.environment_id}
          className="pt-3"
        >
          <EnvironmentSet appName={appName} set={env} />
        </TabsContent>
      ))}
    </Tabs>
  )
}
