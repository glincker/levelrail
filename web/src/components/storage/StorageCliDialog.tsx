import { useState, type ReactElement } from 'react'
import { useTranslation } from 'react-i18next'
import { ShieldCheckGlassIcon } from '@thesvg/ui-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { TerminalBlock } from '@/components/ui/terminal-block'
import { useIsRoot } from '@/hooks/useIsRoot'
import { cn } from '@/lib/utils'
import {
  STORAGE_CLI_COMMANDS,
  STORAGE_CLI_TABS,
  commandFor,
  roleAllows,
  type StorageCliCommand,
  type StorageCliRole,
  type StorageCliTab,
} from './storageCliCommands'

interface StorageCliDialogProps {
  /** The trigger element, for example a header button. */
  children: ReactElement
  /** Defaults to admin for the root account and viewer otherwise. */
  role?: StorageCliRole
  /** Adds --node to the image commands. */
  node?: string
  /** Defaults to this dashboard's origin. */
  serverUrl?: string
}

function CommandBlock({
  entry,
  role,
  node,
}: {
  entry: StorageCliCommand
  role: StorageCliRole
  node?: string
}) {
  const { t } = useTranslation('storageCli')
  const allowed = roleAllows(role, entry.minRole)
  const command = commandFor(entry, node)
  return (
    <li className="min-w-0 space-y-1">
      <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs">
        <span className="rounded-sm border border-border px-1.5 py-0.5 font-medium text-muted-foreground">
          {t(`role.${entry.minRole}`)}
        </span>
        {!allowed ? (
          <span className="text-tone-warning">{t('needsAdmin')}</span>
        ) : null}
        {entry.sensitive ? (
          <span className="text-muted-foreground">{t('needsSensitive')}</span>
        ) : null}
      </div>
      <TerminalBlock
        className={cn(!allowed && 'opacity-60')}
        label={t('blockLabel', { command })}
        lines={[
          { kind: 'comment', text: t(`commands.${entry.description}`) },
          { kind: 'command', text: command },
        ]}
      />
    </li>
  )
}

export function StorageCliDialog({
  children,
  role,
  node,
  serverUrl,
}: StorageCliDialogProps) {
  const { t } = useTranslation('storageCli')
  const isRoot = useIsRoot()
  const effectiveRole: StorageCliRole = role ?? (isRoot ? 'admin' : 'viewer')
  const server = serverUrl ?? window.location.origin
  const [tab, setTab] = useState<StorageCliTab>('usage')
  return (
    <Dialog>
      <DialogTrigger render={children} />
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t('title')}</DialogTitle>
          <DialogDescription>{t('description')}</DialogDescription>
        </DialogHeader>
        <p className="text-xs text-muted-foreground">
          {t('server', { server })}{' '}
          {t('yourRole', { role: t(`role.${effectiveRole}`) })}
        </p>
        <p className="flex items-start gap-1.5 text-xs text-muted-foreground">
          <ShieldCheckGlassIcon
            size="1.4em"
            aria-hidden="true"
            className="mt-px shrink-0 text-tone-success"
          />
          <span>{t('dryRunFirst')}</span>
        </p>
        <Tabs
          value={tab}
          onValueChange={(v) => {
            setTab(v as StorageCliTab)
          }}
        >
          <TabsList variant="line" aria-label={t('tabsLabel')}>
            {STORAGE_CLI_TABS.map((id) => (
              <TabsTrigger key={id} value={id}>
                {t(`tabs.${id}`)}
              </TabsTrigger>
            ))}
          </TabsList>
          {STORAGE_CLI_TABS.map((id) => (
            <TabsContent key={id} value={id} className="pt-3">
              <ul className="max-h-[50vh] space-y-3 overflow-y-auto pr-1">
                {STORAGE_CLI_COMMANDS.filter((c) => c.tab === id).map((c) => (
                  <CommandBlock
                    key={c.id}
                    entry={c}
                    role={effectiveRole}
                    node={node}
                  />
                ))}
              </ul>
              {id === 'clean' ? (
                <p className="mt-3 text-xs text-muted-foreground">
                  {t('whyNoSystemPrune')}
                </p>
              ) : null}
            </TabsContent>
          ))}
        </Tabs>
      </DialogContent>
    </Dialog>
  )
}
