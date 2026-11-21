import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { messageOf } from '../../lib/cloudAwsServices'
import {
  useAwsSsmInstances,
  useAwsSsmRefresh,
  useAwsSsmRun,
  useAwsSsmSession,
} from '../../queries/cloudCfSsm'
import { CopyButton, StateLine } from './AwsServiceShared'

function RunPanel({
  id,
  region,
  instances,
}: Readonly<{ id: string; region: string; instances: string[] }>) {
  const { t } = useTranslation('cloudAccounts')
  const run = useAwsSsmRun(id)
  const refresh = useAwsSsmRefresh(id)
  const session = useAwsSsmSession(id)
  const [instance, setInstance] = useState(instances[0] ?? '')
  const [command, setCommand] = useState('')
  const result = refresh.data ?? run.data?.invocation
  const err = run.error ?? refresh.error ?? session.error
  return (
    <div className="space-y-2 border-t border-border pt-2">
      <h4 className="text-xs font-medium text-muted-foreground">
        {t('services.ssm.runTitle')}
      </h4>
      <form
        className="flex flex-wrap items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          if (instance === '' || command.trim() === '') return
          refresh.reset()
          run.mutate({ region, instance, command: command.trim() })
        }}
      >
        <label className="text-xs text-muted-foreground">
          {t('services.ssm.instance')}
          <select
            value={instance}
            className="mt-1 block h-8 rounded-md border border-input bg-background px-2 font-mono text-xs"
            onChange={(e) => {
              setInstance(e.target.value)
            }}
          >
            {instances.map((i) => (
              <option key={i} value={i}>
                {i}
              </option>
            ))}
          </select>
        </label>
        <label className="min-w-40 flex-1 text-xs text-muted-foreground">
          {t('services.ssm.command')}
          <Input
            value={command}
            className="mt-1 font-mono"
            onChange={(e) => {
              setCommand(e.target.value)
            }}
          />
        </label>
        <Button
          type="submit"
          size="sm"
          disabled={run.isPending || instance === '' || command.trim() === ''}
        >
          {t('services.ssm.run')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={session.isPending || instance === ''}
          onClick={() => {
            session.mutate({ region, instance })
          }}
        >
          {t('services.ssm.session')}
        </Button>
        {session.data ? (
          <CopyButton
            label={t('services.ssm.copyToken')}
            text={session.data.token}
          />
        ) : null}
      </form>
      {err ? (
        <p role="alert" className="text-sm text-destructive">
          {messageOf(err)}
        </p>
      ) : null}
      {result ? (
        <div className="space-y-1 text-xs">
          <div className="flex items-center gap-2">
            <span>
              {t('services.ssm.status')}: {result.status}
            </span>
            <span>
              {t('services.ssm.exit')}: {String(result.exit_code)}
            </span>
            {run.data && result.status === 'InProgress' ? (
              <Button
                size="sm"
                variant="outline"
                disabled={refresh.isPending}
                onClick={() => {
                  refresh.mutate({
                    cid: run.data.commandId,
                    region,
                    instance,
                  })
                }}
              >
                {t('services.ssm.refresh')}
              </Button>
            ) : null}
          </div>
          <pre className="max-h-40 overflow-auto rounded-md bg-muted p-2 font-mono">
            {result.stdout || result.stderr || result.detail}
          </pre>
        </div>
      ) : null}
    </div>
  )
}

// Run Command needs root and its own flag; a 403 or a disabled flag hides it.
export function SsmCard({
  id,
  isRoot,
  runEnabled,
}: Readonly<{ id: string; isRoot: boolean; runEnabled: boolean }>) {
  const { t } = useTranslation('cloudAccounts')
  const list = useAwsSsmInstances(id)
  const rows = list.data?.instances ?? []
  const empty =
    list.data && rows.length === 0 ? t('services.ssm.empty') : undefined
  return (
    <div className="space-y-2">
      <StateLine pending={list.isPending} error={list.error} empty={empty} />
      <ul className="space-y-1">
        {rows.map((i) => (
          <li key={i.id} className="flex flex-wrap items-center gap-2 text-sm">
            <span className="font-mono text-xs">{i.id}</span>
            <span className="text-xs text-muted-foreground">
              {i.computer_name || i.platform_name} · {i.ping_status}
            </span>
          </li>
        ))}
      </ul>
      {isRoot && runEnabled && rows.length > 0 ? (
        <RunPanel
          id={id}
          region={list.data?.region ?? ''}
          instances={rows.map((r) => r.id)}
        />
      ) : null}
    </div>
  )
}
