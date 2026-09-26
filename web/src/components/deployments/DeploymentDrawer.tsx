import { useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { GitBranchIcon, GitCommitIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Button } from '@/components/ui/button'
import { RelativeTime } from '@/components/kit'
import type { Deployment } from '../../types/deployment'
import {
  durationLabel,
  headline,
  isRollback,
  shortId,
  shortSha,
  subtitle,
  triggerLabel,
} from '../../lib/deploymentPresentation'
import {
  deployLogsPath,
  deploymentLogTailOptions,
} from '../../queries/deployments'
import { appDetailQueryOptions } from '../../queries/apps'
import type { DeploymentActionKind } from '../../hooks/useDeploymentActions'
import { PromoteAppDialog } from '../PromoteAppDialog'
import { DeployPreviewThumb } from '../DeployPreviewThumb'
import { DrawerActions } from './DrawerActions'
import { EnvPill, StatusCell } from './DeploymentRow'
import { ImageRefChip } from './ImageRefChip'

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-2">
      <h3 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
        {title}
      </h3>
      {children}
    </section>
  )
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-4 text-sm">
      <dt className="shrink-0 text-muted-foreground">{label}</dt>
      <dd className="min-w-0 text-right break-words">{children}</dd>
    </div>
  )
}

function FailureCard({ d }: { d: Deployment }) {
  const tail = useQuery(deploymentLogTailOptions(d.app, d.id))
  const step = d.steps?.failing_step
  return (
    <Section title="What went wrong">
      <div className="flex flex-col gap-2 rounded-lg border border-tone-danger-border bg-tone-danger-soft p-3">
        {step && (
          <p className="text-sm font-medium text-tone-danger">
            Failed at step: {step}
          </p>
        )}
        <p className="text-sm break-words">
          {d.error_summary ?? 'The deploy failed. No error was recorded.'}
        </p>
        {tail.data && tail.data.length > 0 && (
          <pre
            aria-label="Last log lines"
            className="max-h-48 overflow-auto rounded-md bg-background p-2 font-mono text-xs leading-relaxed"
          >
            {tail.data.join('\n')}
          </pre>
        )}
        {tail.isError && (
          <p className="text-xs text-muted-foreground">
            The log tail could not be loaded.
          </p>
        )}
        <div>
          <Button
            variant="outline"
            size="sm"
            nativeButton={false}
            render={<Link {...deployLogsPath(d)} />}
          >
            View full logs
          </Button>
        </div>
      </div>
    </Section>
  )
}

function DrawerBody({
  d,
  now,
  cancelSupported,
  onAction,
}: {
  d: Deployment
  now: number
  cancelSupported: boolean
  onAction: (kind: DeploymentActionKind, d: Deployment) => void
}) {
  const app = useQuery(appDetailQueryOptions(d.app))
  const [promoteOpen, setPromoteOpen] = useState(false)
  const projectId = app.data?.project_id
  const domains = app.data?.domains ?? []
  const rollback = isRollback(d)
  const sub = subtitle(d)
  return (
    <div className="flex flex-col gap-5 overflow-y-auto px-4 pb-6">
      <div className="flex flex-wrap items-center gap-2">
        <StatusCell d={d} now={now} />
        <EnvPill d={d} />
        {d.is_live && (
          <span className="text-xs font-medium text-tone-success">Live</span>
        )}
      </div>
      {sub && d.status !== 'failed' && (
        <p className="text-sm text-muted-foreground">{sub}</p>
      )}
      <DrawerActions
        d={d}
        cancelSupported={cancelSupported}
        hasProject={Boolean(projectId)}
        onAction={onAction}
        onPromote={() => {
          setPromoteOpen(true)
        }}
      />
      {d.status === 'failed' && <FailureCard d={d} />}
      <Section title="Timing">
        <dl className="flex flex-col gap-1">
          <Row label="Started">
            <RelativeTime at={d.started_at} live />
          </Row>
          {d.finished_at && (
            <Row label="Finished">
              <RelativeTime at={d.finished_at} />
            </Row>
          )}
          <Row label="Duration">{durationLabel(d, now) || 'Not finished'}</Row>
        </dl>
      </Section>
      <Section title="Source">
        <dl className="flex flex-col gap-1">
          <Row label="Trigger">{triggerLabel(d.trigger)}</Row>
          {rollback && d.rollback_of && (
            <Row label="Rollback to">
              <span className="font-mono">{shortId(d.rollback_of)}</span>
            </Row>
          )}
          {d.branch && (
            <Row label="Branch">
              <span className="inline-flex items-center gap-1 font-mono">
                <GitBranchIcon className="size-3.5" aria-hidden="true" />
                {d.branch}
              </span>
            </Row>
          )}
          {d.commit_sha && (
            <Row label="Commit">
              <span className="inline-flex items-center gap-1 font-mono">
                <GitCommitIcon className="size-3.5" aria-hidden="true" />
                {shortSha(d.commit_sha)}
              </span>
            </Row>
          )}
          {d.commit_message && <Row label="Message">{d.commit_message}</Row>}
          {d.author && <Row label="Author">{d.author}</Row>}
          {d.pr_number !== null && (
            <Row label="Pull request">#{d.pr_number}</Row>
          )}
          <Row label="App">
            <Link
              to="/apps/$name/overview"
              params={{ name: d.app }}
              className="underline-offset-4 hover:underline"
            >
              {d.app}
            </Link>
          </Row>
        </dl>
      </Section>
      <Section title="Image">
        <ImageRefChip d={d} />
      </Section>
      <Section title="Domains">
        {domains.length > 0 ? (
          <ul className="flex flex-col gap-0.5 font-mono text-sm">
            {domains.map((dom) => (
              <li key={dom}>{dom}</li>
            ))}
          </ul>
        ) : (
          <p className="text-sm text-muted-foreground">
            {app.isPending ? 'Loading domains' : 'No domains configured.'}
          </p>
        )}
      </Section>
      <Section title="Preview">
        <DeployPreviewThumb
          appName={d.app}
          deploymentId={d.id}
          imageUrl={d.preview_image_url ?? undefined}
          canRecapture={d.is_live}
          size="md"
          className="w-full"
        />
      </Section>
      {d.status !== 'failed' && (
        <Button
          variant="link"
          size="sm"
          className="self-start px-0"
          nativeButton={false}
          render={<Link {...deployLogsPath(d)} />}
        >
          View full logs
        </Button>
      )}
      {projectId && (
        <PromoteAppDialog
          appName={d.app}
          projectId={projectId}
          control={{
            open: promoteOpen,
            onOpenChange: setPromoteOpen,
            hideTrigger: true,
          }}
        />
      )}
    </div>
  )
}

export interface DeploymentDrawerProps {
  id: string
  deployment: Deployment | undefined
  searching: boolean
  now: number
  cancelSupported: boolean
  onClose: () => void
  onAction: (kind: DeploymentActionKind, d: Deployment) => void
}

export function DeploymentDrawer({
  id,
  deployment,
  searching,
  now,
  cancelSupported,
  onClose,
  onAction,
}: DeploymentDrawerProps) {
  return (
    <Sheet
      open={id !== ''}
      onOpenChange={(o) => {
        if (!o) onClose()
      }}
    >
      <SheetContent
        className="w-full data-[side=right]:sm:max-w-xl"
        data-deployment-drawer
      >
        <SheetHeader className="pr-12">
          <SheetTitle className="break-words">
            {deployment ? headline(deployment) : 'Deployment'}
          </SheetTitle>
          <SheetDescription>
            {deployment
              ? `${deployment.app} deployment ${shortId(deployment.id)}`
              : searching
                ? 'Looking for this deployment'
                : 'Deployment not found'}
          </SheetDescription>
        </SheetHeader>
        {deployment ? (
          <DrawerBody
            key={deployment.id}
            d={deployment}
            now={now}
            cancelSupported={cancelSupported}
            onAction={onAction}
          />
        ) : (
          !searching && (
            <p className="px-4 text-sm text-muted-foreground">
              Deployment not found. It may have been pruned by retention.
            </p>
          )
        )}
      </SheetContent>
    </Sheet>
  )
}
