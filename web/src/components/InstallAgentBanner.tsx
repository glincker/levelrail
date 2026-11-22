import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import {
  CircleCheckGlassIcon,
  LoaderCircleGlassIcon,
  ServerGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { BrandIcon } from './BrandIcon'
import { CommandBlock } from './nodes/CommandBlock'
import { dismissBanner, isBannerDismissed } from '../lib/bannerDismissal'
import { pollMs } from '../lib/pollTier'
import {
  nodeListQueryOptions,
  useCreateNodeJoinToken,
  useNodeListOptional,
} from '../queries/nodes'
import { useMeshSettings } from '../queries/meshSettings'
import { findNewNode, looksLikeMac } from '../lib/installAgent'
import type { NodeResource } from '../types/nodeDetail'

type OsTab = 'linux' | 'macos'

const DISMISS_ID = 'install-agent'

function WaitingStatus({ joined }: Readonly<{ joined?: NodeResource }>) {
  const { t } = useTranslation('nodes')
  return (
    <p
      role="status"
      className="flex items-center gap-2 rounded-lg border border-border px-3 py-2 text-sm"
    >
      {joined ? (
        <>
          <CircleCheckGlassIcon
            aria-hidden="true"
            size="1.4em"
            className="text-tone-success"
          />
          {t('installAgent.connected', { name: joined.name })}
        </>
      ) : (
        <>
          <LoaderCircleGlassIcon
            aria-hidden="true"
            size="1.4em"
            className="animate-spin motion-reduce:animate-none"
          />
          {t('installAgent.waiting')}
        </>
      )}
    </p>
  )
}

// Mounted only while the dialog is open, so a join token exists only when
// the user asked for one.
function InstallAgentBody() {
  const { t } = useTranslation('nodes')
  const mint = useCreateNodeJoinToken()
  const mesh = useMeshSettings()
  const nodes = useNodeListOptional()
  const [tab, setTab] = useState<OsTab | null>(null)
  const [baseline, setBaseline] = useState<string[] | null>(null)
  const started = useRef(false)
  const meshReady = mesh.isSuccess || mesh.isError
  const baselineReady = nodes.isSuccess || nodes.isError

  useEffect(() => {
    if (started.current || !meshReady || !baselineReady) return
    started.current = true
    setBaseline((nodes.data ?? []).map((n) => n.id))
    // The agent certificate only covers the advertised name, so prefer it.
    const addr = mesh.data?.agent_addr ?? `${window.location.hostname}:9443`
    mint.mutate({ control_plane_addr: addr })
  }, [meshReady, baselineReady, mesh.data, nodes.data, mint])

  const live = useQuery({
    ...nodeListQueryOptions(),
    enabled: mint.isSuccess,
    refetchInterval: pollMs('live'),
  })
  const joined =
    baseline === null ? undefined : findNewNode(live.data ?? [], baseline)

  const commands = mint.data?.join_commands
  const macos = commands?.macos
  const activeTab: OsTab =
    tab ?? (macos && looksLikeMac(navigator.userAgent) ? 'macos' : 'linux')
  const expires = mint.data
    ? new Date(mint.data.expires_at).toLocaleTimeString([], {
        hour: '2-digit',
        minute: '2-digit',
      })
    : ''

  return (
    <div className="space-y-3">
      {mint.isError ? (
        <Alert variant="destructive">
          <AlertDescription>
            {t('installAgent.mintFailed', { message: mint.error.message })}
          </AlertDescription>
        </Alert>
      ) : null}
      {mint.isSuccess && commands ? (
        <>
          <Tabs value={activeTab} onValueChange={(v) => setTab(v as OsTab)}>
            <TabsList aria-label={t('installAgent.tabsLabel')}>
              <TabsTrigger value="linux">
                <BrandIcon name="linux" className="size-4" />
                {t('installAgent.linux')}
              </TabsTrigger>
              {macos ? (
                <TabsTrigger value="macos">
                  <BrandIcon name="apple" mono className="size-4" />
                  {t('installAgent.macos')}
                </TabsTrigger>
              ) : null}
            </TabsList>
            <TabsContent value="linux" className="pt-2">
              <CommandBlock
                id="install-agent-linux"
                label={t('installAgent.linuxLabel')}
                hint={t('installAgent.linuxHint', { time: expires })}
                value={commands.docker}
              />
            </TabsContent>
            {macos ? (
              <TabsContent value="macos" className="space-y-2 pt-2">
                <CommandBlock
                  id="install-agent-macos"
                  label={t('installAgent.macosLabel')}
                  hint={t('installAgent.macosHint', { time: expires })}
                  value={macos}
                />
                <p className="text-xs text-muted-foreground">
                  {t('installAgent.macosNote')}
                </p>
              </TabsContent>
            ) : null}
          </Tabs>
          <WaitingStatus joined={joined} />
        </>
      ) : mint.isError ? null : (
        <p role="status" className="text-sm text-muted-foreground">
          {t('installAgent.minting')}
        </p>
      )}
    </div>
  )
}

/** Banner that points a single-node install at adding a server or a Mac. */
export function InstallAgentBanner() {
  const { t } = useTranslation('nodes')
  const nodes = useNodeListOptional()
  const [open, setOpen] = useState(false)
  const [dismissed, setDismissed] = useState(() =>
    isBannerDismissed(DISMISS_ID),
  )

  // Unknown count (loading or no access) stays hidden rather than guessing.
  // An open dialog stays mounted: the node it is waiting for raises the count.
  if (!open && (dismissed || !nodes.data || nodes.data.length > 1)) return null

  return (
    <section
      aria-labelledby="install-agent-title"
      className="relative space-y-3 rounded-container border border-border bg-card p-4"
    >
      <Button
        type="button"
        size="icon-sm"
        variant="ghost"
        className="absolute top-2 right-2"
        aria-label={t('installAgent.dismiss')}
        onClick={() => {
          dismissBanner(DISMISS_ID)
          setDismissed(true)
        }}
      >
        <XGlassIcon size="1.2em" />
      </Button>
      <div className="flex items-center gap-2 text-muted-foreground">
        <BrandIcon name="linux" className="size-6" />
        <BrandIcon name="apple" mono className="size-6" />
      </div>
      <div className="space-y-1 pr-6">
        <h2
          id="install-agent-title"
          className="text-sm font-semibold text-foreground"
        >
          {t('installAgent.title')}
        </h2>
        <p className="text-sm text-muted-foreground">
          {t('installAgent.description')}
        </p>
      </div>
      <Button size="sm" onClick={() => setOpen(true)}>
        <ServerGlassIcon size="1.4em" />
        {t('installAgent.cta')}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t('installAgent.dialogTitle')}</DialogTitle>
            <DialogDescription>
              {t('installAgent.dialogDescription')}
            </DialogDescription>
          </DialogHeader>
          <InstallAgentBody />
        </DialogContent>
      </Dialog>
    </section>
  )
}
