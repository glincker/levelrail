import { useState } from 'react'
import {
  ArrowLeftIcon,
  ArrowsInIcon,
  ArrowsOutIcon,
  DatabaseIcon,
  GitBranchIcon,
  MagnifyingGlassIcon,
  StackIcon,
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
import { Input } from '@/components/ui/input'
import { BrandIcon, BRAND_ICON_NAMES, type BrandIconName } from './BrandIcon'
import { CreateAppFields } from './CreateAppFields'
import { CreateAppFromGitFields } from './CreateAppFromGitFields'
import { CreateComposeFields } from './CreateComposeFields'
import { CreateDatabaseFields } from './CreateDatabaseFields'
import { useDatabaseEnginesOptional } from '../queries/databaseEngines'

// Step 1's fixed application starting points: a small set, not a
// template catalog (see this repo's root CLAUDE.md non-goals section).
// Database cards are appended dynamically below, from
// GET /api/v1/database-engines (queries/databaseEngines.ts), rather than
// hardcoded here.
type FixedWizardOption = 'docker-image' | 'dockerfile-git' | 'docker-compose'

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
    description:
      "Point us at your repository, we'll build and deploy it for you.",
    // No brand mark for this one: git itself isn't one of the vendored
    // @thesvg/react logos, so this reuses the same Phosphor icon
    // DeployTriggerForm.tsx's "Build from source" tab already uses for
    // the same concept.
    icon: <GitBranchIcon className="size-6" />,
  },
  {
    value: 'docker-compose',
    label: 'Docker Compose',
    description:
      'Paste or upload a compose.yaml and deploy every service it defines at once.',
    icon: <StackIcon className="size-6" />,
  },
]

// Type guard narrowing an arbitrary engine id string to BrandIconName
// only when a real vendored logo exists for it, so a future engine the
// registry knows about but thesvg.org hasn't vendored a mark for yet
// still renders a real card instead of crashing BrandIcon's exhaustive
// lookup.
function brandIconNameFor(engineId: string): BrandIconName | null {
  return (BRAND_ICON_NAMES as readonly string[]).includes(engineId)
    ? (engineId as BrandIconName)
    : null
}

function matchesSearch(option: WizardOptionDef, query: string): boolean {
  if (!query) return true
  return (
    option.label.toLowerCase().includes(query) ||
    option.description.toLowerCase().includes(query)
  )
}

// A single step-1 card, shared between the Applications and Databases
// sections below so both groups render identically.
function OptionCard({
  option,
  onSelect,
}: {
  option: WizardOptionDef
  onSelect: (value: string) => void
}) {
  return (
    <button
      type="button"
      onClick={() => {
        onSelect(option.value)
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
  )
}

// Two-step creation flow: step 1 picks a starting point from a
// categorized, searchable picker, step 2 shows the minimal config for
// whichever was picked, reusing the exact same field sets and mutation
// hooks CreateAppFields/CreateDatabaseFields/CreateAppFromGitFields/
// CreateComposeFields already back (no validation or submission logic
// is duplicated here, this component only owns which step and which
// option is showing).
//
// Both the apps and databases list routes render this same component as
// their "New app"/"New database" trigger, always opening fresh on step 1:
// see handleOpenChange.
export function CreateResourceWizard({
  trigger,
}: {
  trigger: React.ReactElement
}) {
  const [open, setOpen] = useState(false)
  const [selected, setSelected] = useState<string | null>(null)
  // Presentation only, independent of `selected`: toggling this must
  // never remount step 2's form (that's what `key={selected}` below is
  // actually keyed on), so it lives as its own piece of state. Always
  // starts compact on a fresh open, same as `selected` starting at null.
  const [fullscreen, setFullscreen] = useState(false)
  const [search, setSearch] = useState('')
  // Optional convenience, see useDatabaseEnginesOptional's own doc
  // comment: a slow/failed fetch just means step 1 shows the fixed
  // options without any database cards yet, never a blocked dialog.
  const engineList = useDatabaseEnginesOptional()
  const engines = engineList.data ?? []
  const isFixedOption = (value: string): value is FixedWizardOption =>
    value === 'docker-image' ||
    value === 'dockerfile-git' ||
    value === 'docker-compose'

  const applicationOptions = FIXED_OPTIONS
  const databaseOptions: WizardOptionDef[] = engines.map((engine) => {
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
  })
  const options = [...applicationOptions, ...databaseOptions]

  const normalizedSearch = search.trim().toLowerCase()
  const filteredApplications = applicationOptions.filter((option) =>
    matchesSearch(option, normalizedSearch),
  )
  const filteredDatabases = databaseOptions.filter((option) =>
    matchesSearch(option, normalizedSearch),
  )
  const hasResults =
    filteredApplications.length > 0 || filteredDatabases.length > 0

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      // A fresh step 1 next time the dialog opens; the field components
      // below already reset their own local form state off this same
      // `open` prop.
      setSelected(null)
      setFullscreen(false)
      setSearch('')
    }
  }

  function handleCreated() {
    handleOpenChange(false)
  }

  const selectedTitle = selected
    ? isFixedOption(selected)
      ? selected === 'docker-image'
        ? 'New app from an image'
        : selected === 'dockerfile-git'
          ? 'New app from a git repo'
          : 'New app from Docker Compose'
      : `New ${engines.find((e) => e.id === selected)?.label ?? selected} database`
    : ''

  // One-sentence, plain-language framing of what's about to happen,
  // specific to the picked path.
  const appYamlAside =
    "Already deploying this app from an app.yaml in a connected repo? That happens automatically, you don't need to add it here."
  const selectedDescription = selected
    ? selected === 'docker-image'
      ? `Point this at an image you've already built and pushed to a registry Docker can pull from, like Docker Hub or GHCR. ${appYamlAside}`
      : selected === 'dockerfile-git'
        ? `We'll create your app, then build and deploy it straight from your repository. This usually takes a minute or two. ${appYamlAside}`
        : selected === 'docker-compose'
          ? `We'll create one app per service defined in your compose.yaml. ${appYamlAside}`
          : "We'll set this database up on your server and connect it automatically, no manual configuration needed."
    : ''

  const gridClassName = fullscreen
    ? 'grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 xl:grid-cols-5'
    : 'grid grid-cols-2 gap-3'

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={trigger} />
      <DialogContent
        size={fullscreen ? 'fullscreen' : 'default'}
        // sm:max-w-lg only applies to the centered `default` popup;
        // passing it while `fullscreen` is active would fight that
        // variant's own `max-w-none` at the sm breakpoint and up.
        className={fullscreen ? undefined : 'sm:max-w-lg'}
      >
        {/* Absolutely positioned against DialogContent itself, sitting
            left of the close button so collapsing back to compact stays
            a distinct action from closing the wizard entirely. */}
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
            <div className="relative">
              <MagnifyingGlassIcon
                className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
                aria-hidden="true"
              />
              <Input
                value={search}
                onChange={(e) => {
                  setSearch(e.target.value)
                }}
                placeholder="Search resources..."
                aria-label="Search resources"
                className="pl-8"
              />
            </div>
            {filteredApplications.length > 0 ? (
              <div className="space-y-2">
                <h3 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
                  Applications
                </h3>
                <div className={gridClassName}>
                  {filteredApplications.map((option) => (
                    <OptionCard
                      key={option.value}
                      option={option}
                      onSelect={setSelected}
                    />
                  ))}
                </div>
              </div>
            ) : null}
            {filteredDatabases.length > 0 ? (
              <div className="space-y-2">
                <h3 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
                  Databases
                </h3>
                <div className={gridClassName}>
                  {filteredDatabases.map((option) => (
                    <OptionCard
                      key={option.value}
                      option={option}
                      onSelect={setSelected}
                    />
                  ))}
                </div>
              </div>
            ) : null}
            {!hasResults && options.length > 0 ? (
              <p className="py-6 text-center text-sm text-muted-foreground">
                No resources match &ldquo;{search}&rdquo;.
              </p>
            ) : null}
          </>
        ) : (
          // `display: contents` when compact makes this wrapper invisible
          // to layout. Full screen swaps it for a real flex column that
          // caps line length to a sane reading width.
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
            {/* key={selected} forces a fresh mount (and so a blank form)
                whenever the picked option changes, including toggling
                between two different database engines which both render
                CreateDatabaseFields. Toggling `fullscreen` does not
                touch `selected`, so it never forces this remount. */}
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
            {selected === 'docker-compose' ? (
              <CreateComposeFields
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
