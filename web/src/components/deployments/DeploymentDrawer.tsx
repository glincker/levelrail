import { useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
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
  hasNote,
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
import { SupplyChainSection } from '../SupplyChainSection'
import { SecretScanSection } from '../SecretScanSection'
import { DrawerActions } from './DrawerActions'
import { DeploymentNote } from './DeploymentNote'
import { EnvPill, StatusCell } from './DeploymentRow'
import { ImageRefChip } from './ImageRefChip'
import { CodeBlock } from '@/components/ui/code-block'

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
  const { t } = useTranslation('deployDetail')
  const tail = useQuery(deploymentLogTailOptions(d.app, d.id))
  const step = d.steps?.failing_step
  return (
    <Section title={t('drawer.whatWentWrong')}>
      <div className="flex flex-col gap-2 rounded-lg border border-tone-danger-border bg-tone-danger-soft p-3">
        {step && (
          <p className="text-sm font-medium text-tone-danger">
            {t('drawer.failedAtStep', { step })}
          </p>
        )}
        <p className="text-sm break-words">
          {d.error_summary ?? t('drawer.noErrorRecorded')}
        </p>
        {tail.data && tail.data.length > 0 && (
          <CodeBlock
            code={tail.data.join('\n')}
            label={t('drawer.lastLogLines')}
            maxHeight="sm"
          />
        )}
        {tail.isError && (
          <p className="text-xs text-muted-foreground">
            {t('drawer.tailFailed')}
          </p>
        )}
        <div>
          <Button
            variant="outline"
            size="sm"
            nativeButton={false}
            render={<Link {...deployLogsPath(d)} />}
          >
            {t('drawer.viewFullLogs')}
          </Button>
        </div>
      </div>
    </Section>
  )
}

function DrawerBody({
  d,
  now,
  onAction,
  onOpenDeployment,
}: {
  d: Deployment
  now: number
  onAction: (kind: DeploymentActionKind, d: Deployment) => void
  onOpenDeployment: (id: string) => void
}) {
  const { t } = useTranslation('deployDetail')
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
          <span className="text-xs font-medium text-tone-success">
            {t('drawer.live')}
          </span>
        )}
      </div>
      {hasNote(d) ? (
        <p>
          <DeploymentNote d={d} onOpen={onOpenDeployment} />
        </p>
      ) : (
        sub &&
        d.status !== 'failed' && (
          <p className="text-sm text-muted-foreground">{sub}</p>
        )
      )}
      <DrawerActions
        d={d}
        hasProject={Boolean(projectId)}
        onAction={onAction}
        onPromote={() => {
          setPromoteOpen(true)
        }}
      />
      {d.status === 'failed' && <FailureCard d={d} />}
      <Section title={t('drawer.timing')}>
        <dl className="flex flex-col gap-1">
          <Row label={t('drawer.started')}>
            <RelativeTime at={d.started_at} live />
          </Row>
          {d.finished_at && (
            <Row label={t('drawer.finished')}>
              <RelativeTime at={d.finished_at} />
            </Row>
          )}
          <Row label={t('drawer.duration')}>
            {durationLabel(d, now) || t('drawer.notFinished')}
          </Row>
        </dl>
      </Section>
      <Section title={t('drawer.source')}>
        <dl className="flex flex-col gap-1">
          <Row label={t('drawer.trigger')}>{triggerLabel(d.trigger)}</Row>
          {rollback && d.rollback_of && (
            <Row label={t('drawer.rollbackTo')}>
              <span className="font-mono">{shortId(d.rollback_of)}</span>
            </Row>
          )}
          {d.branch && (
            <Row label={t('drawer.branch')}>
              <span className="inline-flex items-center gap-1 font-mono">
                <GitBranchIcon className="size-3.5" aria-hidden="true" />
                {d.branch}
              </span>
            </Row>
          )}
          {d.commit_sha && (
            <Row label={t('drawer.commit')}>
              <span className="inline-flex items-center gap-1 font-mono">
                <GitCommitIcon className="size-3.5" aria-hidden="true" />
                {shortSha(d.commit_sha)}
              </span>
            </Row>
          )}
          {d.commit_message && (
            <Row label={t('drawer.message')}>
              <span className="block max-h-40 overflow-y-auto text-left whitespace-pre-wrap">
                {d.commit_message}
              </span>
            </Row>
          )}
          {d.author && <Row label={t('drawer.author')}>{d.author}</Row>}
          {d.pr_number !== null && (
            <Row label={t('drawer.pullRequest')}>#{d.pr_number}</Row>
          )}
          <Row label={t('drawer.app')}>
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
      <Section title={t('drawer.image')}>
        <ImageRefChip d={d} />
      </Section>
      <Section title={t('drawer.domains')}>
        {domains.length > 0 ? (
          <ul className="flex flex-col gap-0.5 font-mono text-sm">
            {domains.map((dom) => (
              <li key={dom}>{dom}</li>
            ))}
          </ul>
        ) : (
          <p className="text-sm text-muted-foreground">
            {app.isPending ? t('drawer.loadingDomains') : t('drawer.noDomains')}
          </p>
        )}
      </Section>
      <Section title={t('drawer.preview')}>
        <DeployPreviewThumb
          appName={d.app}
          deploymentId={d.id}
          imageUrl={d.preview_image_url ?? undefined}
          canRecapture={d.is_live}
          size="md"
          className="w-full"
        />
      </Section>
      <Section title={t('drawer.supplyChain')}>
        <SupplyChainSection
          appName={d.app}
          deploymentId={d.id}
          sbomPackages={d.sbom_packages}
        />
      </Section>
      <SecretScanSection appName={d.app} deploymentId={d.id} />
      {d.status !== 'failed' && (
        <Button
          variant="link"
          size="sm"
          className="self-start px-0"
          nativeButton={false}
          render={<Link {...deployLogsPath(d)} />}
        >
          {t('drawer.viewFullLogs')}
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
  onClose: () => void
  onAction: (kind: DeploymentActionKind, d: Deployment) => void
  onOpenDeployment: (id: string) => void
}

export function DeploymentDrawer({
  id,
  deployment,
  searching,
  now,
  onClose,
  onAction,
  onOpenDeployment,
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
            onAction={onAction}
            onOpenDeployment={onOpenDeployment}
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
