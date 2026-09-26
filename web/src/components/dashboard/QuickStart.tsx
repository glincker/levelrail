import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import {
  DownloadSimpleIcon,
  GitBranchIcon,
  GlobeIcon,
  HardDrivesIcon,
  PlusIcon,
  SquaresFourIcon,
} from '@phosphor-icons/react/dist/ssr'
import { CreateResourceWizard } from '../CreateResourceWizard'

const CARD =
  'group flex flex-col gap-3 rounded-2xl border border-border bg-card p-4 text-left transition-colors duration-200 hover:border-primary/40 hover:bg-muted/40 focus-visible:outline-2 focus-visible:outline-ring'

function Body({
  icon,
  title,
  hint,
}: {
  icon: ReactNode
  title: string
  hint: string
}) {
  return (
    <>
      <span
        aria-hidden="true"
        className="flex size-9 items-center justify-center rounded-xl bg-primary/10 text-primary"
      >
        {icon}
      </span>
      <span>
        <span className="block text-sm font-semibold text-foreground">
          {title}
        </span>
        <span className="block text-xs text-muted-foreground">{hint}</span>
      </span>
    </>
  )
}

const ICON = 'size-5'

export function QuickStart({ hasApps }: { hasApps: boolean }) {
  const importCard = (
    <Link to="/settings/import-platform" className={CARD}>
      <Body
        icon={<DownloadSimpleIcon className={ICON} />}
        title="Import anything"
        hint="Coolify, Dokploy, CapRover, docker run"
      />
    </Link>
  )
  return (
    <section aria-label="Quick start" className="space-y-3">
      <h2 className="text-sm font-semibold text-foreground">Quick start</h2>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {hasApps ? (
          <>
            <CreateResourceWizard
              trigger={
                <button type="button" className={CARD}>
                  <Body
                    icon={<PlusIcon className={ICON} />}
                    title="New app"
                    hint="From git, an image or a template"
                  />
                </button>
              }
            />
            {importCard}
            <Link to="/domains" className={CARD}>
              <Body
                icon={<GlobeIcon className={ICON} />}
                title="Add a domain"
                hint="HTTPS with automatic certificates"
              />
            </Link>
            <Link to="/nodes" className={CARD}>
              <Body
                icon={<HardDrivesIcon className={ICON} />}
                title="Add a node"
                hint="Grow the fleet to another server"
              />
            </Link>
          </>
        ) : (
          <>
            {importCard}
            <CreateResourceWizard
              initialSelected="browse-templates"
              trigger={
                <button type="button" className={CARD}>
                  <Body
                    icon={<SquaresFourIcon className={ICON} />}
                    title="Deploy a template"
                    hint="Databases, tools and stacks in a click"
                  />
                </button>
              }
            />
            <Link to="/settings/github-app" className={CARD}>
              <Body
                icon={<GitBranchIcon className={ICON} />}
                title="Connect Git"
                hint="Deploy on every push"
              />
            </Link>
          </>
        )}
      </div>
    </section>
  )
}
