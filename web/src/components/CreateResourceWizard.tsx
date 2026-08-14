import { useState } from 'react'
import { ArrowLeftIcon, GitBranchIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { BrandIcon } from './BrandIcon'
import { CreateAppFields } from './CreateAppFields'
import { CreateAppFromGitFields } from './CreateAppFromGitFields'
import { CreateDatabaseFields } from './CreateDatabaseFields'

// The four starting points step 1 offers, per
// docs/superpowers/specs/2026-08-14-creation-wizard-and-sidebar-design.md
// section 2: a small, fixed grid, not a template catalog (CLAUDE.md
// section 2's own non-goal: "Not chasing Coolify's 280 one-click
// templates. Ten good ones beat 280 stale ones"). MySQL was the
// original fifth pick but internal/reconcile/database/controller.go
// only handles postgres/redis today (verified by grep, no MySQL case
// anywhere in that controller), so it's left out of this first cut per
// the spec's own note, a fast-follow once the reconciler grows a third
// engine rather than a picker option backed by nothing.
type WizardOption = 'docker-image' | 'dockerfile-git' | 'postgres' | 'redis'

interface WizardOptionDef {
  value: WizardOption
  label: string
  description: string
  icon: React.ReactNode
}

const WIZARD_OPTIONS: WizardOptionDef[] = [
  {
    value: 'docker-image',
    label: 'Docker image',
    description: 'Already have a built image ready to run.',
    icon: <BrandIcon name="docker" className="size-6" />,
  },
  {
    value: 'dockerfile-git',
    label: 'Dockerfile from git',
    description: 'Build an image from a Dockerfile in a repo.',
    // No brand mark for this one: git itself isn't one of the four
    // vendored @thesvg/react logos (BrandIcon.tsx's own doc comment),
    // so this uses the same Phosphor icon DeployTriggerForm.tsx's
    // "Build from source" tab already uses for the same concept, for
    // visual/semantic consistency with the one other git-related
    // control in this app rather than picking a fresh icon cold. This
    // is a deliberate mixed-icon-source picker (three cards use brand
    // logos, one uses UI chrome iconography); see BrandIcon.tsx's own
    // doc comment for why that's two different concerns that can
    // coexist in one picker without reopening the Phosphor-only rule.
    icon: <GitBranchIcon className="size-6" />,
  },
  {
    value: 'postgres',
    label: 'Postgres',
    description: 'A managed Postgres database.',
    icon: <BrandIcon name="postgres" className="size-6" />,
  },
  {
    value: 'redis',
    label: 'Redis',
    description: 'A managed Redis database.',
    icon: <BrandIcon name="redis" className="size-6" />,
  },
]

const OPTION_TITLES: Record<WizardOption, string> = {
  'docker-image': 'New app from an image',
  'dockerfile-git': 'New app from a git repo',
  postgres: 'New Postgres database',
  redis: 'New Redis database',
}

// Two-step creation flow replacing the old single-step CreateAppDialog/
// CreateDatabaseDialog entry points, per the design spec's section 2:
// step 1 picks a starting point from a small fixed grid, step 2 shows
// the minimal config for whichever was picked, reusing the exact same
// field sets and mutation hooks CreateAppFields/CreateDatabaseFields/
// CreateAppFromGitFields already back (no validation or submission
// logic is duplicated here, this component only owns which step and
// which option is showing).
//
// Both the apps and databases list routes render this same component
// as their "New app"/"New database" trigger: there's no single step-1
// option each button could sensibly pre-select (apps has two starting
// points, docker-image and dockerfile-git; databases has two, postgres
// and redis), so pre-scrolling to "the relevant option" doesn't reduce
// clicks in either direction, it would just be an arbitrary pick
// between two equally-valid ones. Both buttons open the same wizard
// fresh on step 1 instead, which also matches the actual point of this
// redesign: replacing "New app" and "New database" as two separate
// mental models with one unified "pick a starting point" flow, the way
// Coolify's own New Resource flow works.
export function CreateResourceWizard({
  trigger,
}: {
  trigger: React.ReactElement
}) {
  const [open, setOpen] = useState(false)
  const [selected, setSelected] = useState<WizardOption | null>(null)

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      // A fresh step 1 next time the dialog opens; the field
      // components below already reset their own local form state off
      // this same `open` prop.
      setSelected(null)
    }
  }

  function handleCreated() {
    handleOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={trigger} />
      <DialogContent className="sm:max-w-lg">
        {selected === null ? (
          <>
            <DialogHeader>
              <DialogTitle>New resource</DialogTitle>
              <DialogDescription>Pick a starting point.</DialogDescription>
            </DialogHeader>
            <div className="grid grid-cols-2 gap-3">
              {WIZARD_OPTIONS.map((option) => (
                <button
                  key={option.value}
                  type="button"
                  onClick={() => {
                    setSelected(option.value)
                  }}
                  className="flex flex-col items-start gap-2 rounded-lg border border-border bg-card p-3 text-left transition-colors hover:border-primary/40 hover:bg-muted focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
                >
                  {option.icon}
                  <span className="text-sm font-medium text-foreground">
                    {option.label}
                  </span>
                  <span className="text-xs text-muted-foreground">
                    {option.description}
                  </span>
                </button>
              ))}
            </div>
          </>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  onClick={() => {
                    setSelected(null)
                  }}
                  aria-label="Back to starting points"
                >
                  <ArrowLeftIcon />
                </Button>
                {OPTION_TITLES[selected]}
              </DialogTitle>
              <DialogDescription>
                {selected === 'docker-image' || selected === 'dockerfile-git'
                  ? 'Apps deployed from an app.yaml spec in a connected repo show up automatically instead.'
                  : 'The reconciler provisions it on the target node.'}
              </DialogDescription>
            </DialogHeader>
            {/* key={selected} forces a fresh mount (and so a blank
                form) whenever the picked option changes, including
                toggling between postgres and redis which both render
                CreateDatabaseFields: switching the starting point is
                explicitly allowed to drop whatever was typed for the
                previous one rather than trying to carry values across
                two different field sets. */}
            {selected === 'docker-image' ? (
              <CreateAppFields
                key={selected}
                open={open}
                onCreated={handleCreated}
              />
            ) : null}
            {selected === 'dockerfile-git' ? (
              <CreateAppFromGitFields
                key={selected}
                open={open}
                onCreated={handleCreated}
              />
            ) : null}
            {selected === 'postgres' ? (
              <CreateDatabaseFields
                key={selected}
                open={open}
                onCreated={handleCreated}
                engine="postgres"
              />
            ) : null}
            {selected === 'redis' ? (
              <CreateDatabaseFields
                key={selected}
                open={open}
                onCreated={handleCreated}
                engine="redis"
              />
            ) : null}
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
