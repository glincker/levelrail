import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PlugGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { CopyField } from '../traffic/CopyField'
import { useDatabaseNetwork } from '../../queries/databaseAccess'
import { connectStrings, defaultConnectUser } from '@/lib/databaseConnect'
import type { DatabaseResource } from '../../types/databaseDetail'

/** Connect opens a drawer with ready connection strings for this engine and the tunnel how-to. */
export function ConnectDrawer({ database }: { database: DatabaseResource }) {
  const { t } = useTranslation('databases')
  const [open, setOpen] = useState(false)

  return (
    <>
      <Button
        size="sm"
        variant="outline"
        onClick={() => setOpen(true)}
        aria-haspopup="dialog"
      >
        <PlugGlassIcon size="1.4em" aria-hidden="true" />
        {t('connectDrawer.trigger')}
      </Button>
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent className="data-[side=right]:sm:max-w-xl">
          <SheetHeader className="border-b border-border pr-12">
            <SheetTitle>{t('connectDrawer.title')}</SheetTitle>
            <SheetDescription>
              {t('connectDrawer.description')}
            </SheetDescription>
          </SheetHeader>
          <div className="min-h-0 flex-1 space-y-5 overflow-y-auto px-4 pb-4">
            {open ? <DrawerBody database={database} /> : null}
          </div>
        </SheetContent>
      </Sheet>
    </>
  )
}

function DrawerBody({ database }: { database: DatabaseResource }) {
  const { t } = useTranslation('databases')
  const { data: network } = useDatabaseNetwork(database.name)
  const engine = database.engine
  const user = defaultConnectUser(engine)
  const dbName = engine === 'postgres' ? 'postgres' : ''
  const tls = network?.tls.enabled ?? database.tls_enabled ?? false

  const internal = network
    ? connectStrings({
        engine,
        host: network.internal.host,
        port: network.internal.port,
        user,
        database: dbName,
        tls,
      })
    : null
  const published = network?.published
    ? connectStrings({
        engine,
        host: window.location.hostname,
        port: network.published.host_port,
        user,
        database: dbName,
        tls,
      })
    : null

  return (
    <>
      <p className="text-sm text-muted-foreground">
        {t('connectDrawer.passwordNote')}
      </p>
      {internal ? (
        <Section title={t('connectDrawer.internal')}>
          <CopyField value={internal.uri} label={t('connectDrawer.copyUri')} />
          {internal.cli ? (
            <CopyField
              value={internal.cli}
              label={t('connectDrawer.copyCommand')}
            />
          ) : null}
        </Section>
      ) : null}
      {published ? (
        <Section title={t('connectDrawer.public')}>
          <CopyField value={published.uri} label={t('connectDrawer.copyUri')} />
          {published.cli ? (
            <CopyField
              value={published.cli}
              label={t('connectDrawer.copyCommand')}
            />
          ) : null}
        </Section>
      ) : null}
      <details className="rounded-md border border-border p-3">
        <summary className="cursor-pointer text-sm font-medium">
          {t('connectDrawer.tunnelTitle')}
        </summary>
        <div className="mt-3 space-y-2">
          <p className="text-sm text-muted-foreground">
            {t('connectDrawer.tunnelBody')}
          </p>
          <CopyField
            value={`levelrail-cli databases connect ${database.name} --launch`}
            label={t('connectDrawer.copyCommand')}
          />
          <p className="text-xs text-muted-foreground">
            {t('connectDrawer.tunnelAfter')}
          </p>
        </div>
      </details>
    </>
  )
}

function Section({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return (
    <section className="space-y-2">
      <h3 className="text-xs text-muted-foreground uppercase">{title}</h3>
      {children}
    </section>
  )
}
