import { useState } from 'react'
import {
  ArrowLeftIcon,
  ArrowsInIcon,
  ArrowsOutIcon,
  DatabaseIcon,
  GitBranchIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { BrandIcon, BRAND_ICON_NAMES, type BrandIconName } from './BrandIcon'
import { CreateAppFields } from './CreateAppFields'
import { CreateAppFromGitFields } from './CreateAppFromGitFields'
import { CreateDatabaseFields } from './CreateDatabaseFields'
import { useDatabaseEnginesOptional } from '../queries/databaseEngines'

// The two non-database starting points step 1 always offers: a small,
// fixed grid, not a template catalog (this project's own stated
// non-goal: "Not chasing Coolify's 280 one-click templates. Ten good
// ones beat 280 stale ones"). Database engine cards
// are appended dynamically below, from GET /api/v1/database-engines
// (queries/databaseEngines.ts), rather than hardcoded here: this used
// to list postgres/redis/mysql by hand, the exact "hardcode a name, add
// a new one, hunt down every place it's copied" problem
// internal/store/database_engines.yaml exists to close, only half-closed
// when this file still had its own hardcoded copy of the same list.
type FixedWizardOption = 'docker-image' | 'dockerfile-git'

interface WizardOptionDef {
  value: string
  label: string
  description: string
  icon: React.ReactNode
}

const FIXED_OPTIONS: WizardOptionDef[] = [
  {
    value: 'docker-image',
    label: 'Docker image',
    description:
      "You've already built an image and pushed it somewhere Docker can pull it from.",
    icon: <BrandIcon name="docker" className="size-6" />,
  },
  {
    value: 'dockerfile-git',
    label: 'Deploy from git',
    description: "Point us at your repository, we'll build and deploy it for you.",
    // No brand mark for this one: git itself isn't one of the four
    // vendored @thesvg/react logos (BrandIcon.tsx's own doc comment),
    // so this uses the same Phosphor icon DeployTriggerForm.tsx's
    // "Build from source" tab already uses for the same concept, for
    // visual/semantic consistency with the one other git-related
    // control in this app rather than picking a fresh icon cold. This
    // is a deliberate mixed-icon-source picker (brand-logo cards use
    // thesvg.org marks, this one uses UI chrome iconography); see
    // BrandIcon.tsx's own doc comment for why that's two different
    // concerns that can coexist in one picker without reopening the
    // Phosphor-only rule.
    icon: <GitBranchIcon className="size-6" />,
  },
]

// Type guard narrowing an arbitrary engine id string to BrandIconName
// only when a real vendored logo exists for it (BrandIcon.tsx's own
// BRAND_ICON_NAMES), so a future engine the registry knows about but
// thesvg.org hasn't been asked to vendor a mark for yet still renders a
// real card instead of crashing BrandIcon's exhaustive lookup.
function brandIconNameFor(engineId: string): BrandIconName | null {
  return (BRAND_ICON_NAMES as readonly string[]).includes(engineId)
    ? (engineId as BrandIconName)
    : null
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
  const [selected, setSelected] = useState<string | null>(null)
  // Presentation only, independent of `selected`: toggling this must
  // never remount step 2's form (that's what `key={selected}` below is
  // actually keyed on), so it lives as its own piece of state rather
  // than being folded into some combined "step" value. Always starts
  // compact on a fresh open, same as `selected` starting at null: see
  // handleOpenChange.
  const [fullscreen, setFullscreen] = useState(false)
  // Optional convenience, see useDatabaseEnginesOptional's own doc
  // comment: a slow/failed fetch just means step 1 shows the two fixed
  // options without any database cards yet, never a blocked dialog.
  const engineList = useDatabaseEnginesOptional()
  const engines = engineList.data ?? []
  const isFixedOption = (value: string): value is FixedWizardOption =>
    value === 'docker-image' || value === 'dockerfile-git'

  const options: WizardOptionDef[] = [
    ...FIXED_OPTIONS,
    ...engines.map((engine) => {
      const brandName = brandIconNameFor(engine.id)
      return {
        value: engine.id,
        label: engine.label,
        description: `A managed ${engine.label} database, set up and ready to use automatically.`,
        icon: brandName ? (
          <BrandIcon name={brandName} className="size-6" />
        ) : (
          <DatabaseIcon className="size-6" />
        ),
      }
    }),
  ]

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      // A fresh step 1 next time the dialog opens; the field
      // components below already reset their own local form state off
      // this same `open` prop.
      setSelected(null)
      setFullscreen(false)
    }
  }

  function handleCreated() {
    handleOpenChange(false)
  }

  const selectedTitle = selected
    ? isFixedOption(selected)
      ? selected === 'docker-image'
        ? 'New app from an image'
        : 'New app from a git repo'
      : `New ${engines.find((e) => e.id === selected)?.label ?? selected} database`
    : ''

  // One-sentence, plain-language framing of what's about to happen,
  // specific to the picked path: a first-time user shouldn't have to
  // infer it from the fields alone. The app.yaml aside only applies to
  // the two app paths, since a database has no spec-file equivalent.
  const appYamlAside =
    "Already deploying this app from an app.yaml in a connected repo? That happens automatically, you don't need to add it here."
  const selectedDescription = selected
    ? selected === 'docker-image'
      ? `Point this at an image you've already built and pushed to a registry Docker can pull from, like Docker Hub or GHCR. ${appYamlAside}`
      : selected === 'dockerfile-git'
        ? `We'll create your app, then build and deploy it straight from your repository. This usually takes a minute or two. ${appYamlAside}`
        : "We'll set this database up on your server and connect it automatically, no manual configuration needed."
    : ''

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={trigger} />
      <DialogContent
        size={fullscreen ? 'fullscreen' : 'default'}
        // sm:max-w-lg only means something for the centered `default`
        // popup; passing it while `fullscreen` is active would fight
        // that variant's own `max-w-none` at the sm breakpoint and up
        // (twMerge treats `max-w-none` and `sm:max-w-lg` as different
        // groups, so both would otherwise apply, quietly reimposing a
        // width cap on what's supposed to fill the viewport).
        className={fullscreen ? undefined : 'sm:max-w-lg'}
      >
        {/* Absolutely positioned against DialogContent itself (same
            trick dialog.tsx already uses for its own close button), so
            this sits in the popup's top-right corner in both
            presentations without needing to duplicate it into both the
            step 1 and step 2 header markup below. Placed to the close
            button's left (it occupies right-2 at icon-sm width) rather
            than replacing it: collapsing back to compact must stay a
            distinct action from closing the wizard entirely. */}
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          className="absolute top-2 right-11"
          onClick={() => {
            setFullscreen((prev) => !prev)
          }}
          aria-label={
            fullscreen ? 'Collapse to compact view' : 'Expand to fullscreen'
          }
        >
          {fullscreen ? <ArrowsInIcon /> : <ArrowsOutIcon />}
        </Button>
        {selected === null ? (
          <>
            <DialogHeader>
              <DialogTitle className={fullscreen ? 'text-lg' : undefined}>
                New resource
              </DialogTitle>
              <DialogDescription>Pick a starting point.</DialogDescription>
            </DialogHeader>
            {/* Compact stays the existing fixed two-column grid. Full
                screen earns real breakpoints instead of just letting
                grid-cols-2 stretch two huge cards across the viewport:
                the same sm/md/xl breakpoint vocabulary this codebase
                already uses elsewhere (dl grids in the node/database
                overview routes), stepped up one column at a time so the
                grid never jumps straight from 2 to 5 at a single
                breakpoint. */}
            <div
              className={
                fullscreen
                  ? 'grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 xl:grid-cols-5'
                  : 'grid grid-cols-2 gap-3'
              }
            >
              {options.map((option) => (
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
          // `display: contents` when compact makes this wrapper
          // invisible to layout, so DialogHeader and the field
          // component below participate directly in DialogContent's own
          // `grid gap-4` exactly as they did before this wrapper
          // existed: compact stays byte-for-byte unchanged. Full screen
          // swaps it for a real flex column that caps line length to a
          // sane reading width and centers it in the extra space,
          // because a form's labels and inputs stretched across a whole
          // viewport are the "narrow content stretched into empty
          // space" failure mode this is explicitly meant to avoid.
          <div
            className={
              fullscreen
                ? 'mx-auto flex w-full max-w-2xl flex-col gap-4 py-4'
                : 'contents'
            }
          >
            <DialogHeader>
              <DialogTitle
                className={
                  fullscreen
                    ? 'flex items-center gap-2 text-lg'
                    : 'flex items-center gap-2'
                }
              >
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
                {selectedTitle}
              </DialogTitle>
              <DialogDescription>{selectedDescription}</DialogDescription>
            </DialogHeader>
            {/* key={selected} forces a fresh mount (and so a blank
                form) whenever the picked option changes, including
                toggling between two different database engines which
                both render CreateDatabaseFields: switching the starting
                point is explicitly allowed to drop whatever was typed
                for the previous one rather than trying to carry values
                across two different field sets. Toggling `fullscreen`
                does not touch `selected`, so it never forces this
                remount: whatever's been typed survives the presentation
                switch. */}
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
            {!isFixedOption(selected) ? (
              <CreateDatabaseFields
                key={selected}
                open={open}
                onCreated={handleCreated}
                engine={selected}
              />
            ) : null}
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
