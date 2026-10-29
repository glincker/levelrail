import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  DatabaseGlassIcon,
  GitBranchGlassIcon,
  LayersGlassIcon,
  PackageGlassIcon,
  SearchGlassIcon,
} from '@thesvg/ui-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { BrandIcon, BRAND_ICON_NAMES } from './BrandIcon'
import { DatabasePicker } from './DatabasePicker'
import { buildPickerEntries, type PostgresFlavor } from '@/lib/databaseCatalog'
import { engineAccent } from '@/lib/engineAccents'
import { StepHero } from './createResource/StepHero'
import { BrowseTemplatesFields } from './BrowseTemplatesFields'
import { CreateAppFields } from './CreateAppFields'
import { CreateAppFromGitFields } from './CreateAppFromGitFields'
import { CreateComposeFields } from './CreateComposeFields'
import { CreateDatabaseFields } from './CreateDatabaseFields'
import { ImportFrontDoor } from './ImportFrontDoor'
import { ImportPlanPreview } from './ImportPlanPreview'
import type { ImportPlan, ImportPlanRequest } from '../queries/imports'
import { useDatabaseEnginesOptional } from '../queries/databaseEngines'

// Step 1's fixed application starting points. Database cards are
// appended dynamically below, from GET /api/v1/database-engines
// (queries/databaseEngines.ts), rather than hardcoded here.
// browse-templates leads to BrowseTemplatesFields, a searchable grid
// over Levelrail's own curated GET /api/v1/service-templates catalog
// (internal/catalog, ADR 015), distinct from docker-compose's own
// paste-your-own-YAML path below.
type FixedWizardOption =
  'docker-image' | 'dockerfile-git' | 'docker-compose' | 'browse-templates'

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
    icon: <GitBranchGlassIcon className="size-7" />,
  },
  {
    value: 'docker-compose',
    label: 'Docker Compose',
    description:
      'Paste or upload a compose.yaml and deploy every service it defines at once.',
    icon: <LayersGlassIcon className="size-7" />,
  },
  {
    value: 'browse-templates',
    label: 'Browse templates',
    description:
      'Pick from a curated catalog of ready-to-deploy services like n8n, Uptime Kuma, and MinIO.',
    icon: <PackageGlassIcon className="size-7" />,
  },
]

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
// hooks the Create*Fields components already back (no validation or
// submission logic is duplicated here).
//
// `scope` lets a caller that already declared intent (the apps or
// databases list's own "New" button) skip the other category entirely.
// Leave it undefined for a generic entry point (dashboard overview).
export function CreateResourceWizard({
  trigger,
  scope,
  initialSelected,
  initialTemplateId,
  open: controlledOpen,
  onOpenChange,
}: {
  trigger?: React.ReactElement
  /** Lets a caller open the wizard from its own control (a menu item) instead of a trigger. */
  open?: boolean
  onOpenChange?: (open: boolean) => void
  scope?: 'applications' | 'databases'
  /** Opens straight to step 2 on this option instead of step 1's picker, e.g. "browse-templates" for a "Start from a template" entry point. */
  initialSelected?: FixedWizardOption
  /** Paired with `initialSelected="browse-templates"`: skips that step's
   *  own grid too and opens straight to this template's preview/
   *  configure form, e.g. the /templates/$id detail page's "Configure and
   *  deploy" button. Ignored otherwise. */
  initialTemplateId?: string
}) {
  const { t } = useTranslation('databases')
  const [innerOpen, setOpen] = useState(false)
  const open = controlledOpen ?? innerOpen
  const [selected, setSelected] = useState<string | null>(
    initialSelected ?? null,
  )
  const [flavor, setFlavor] = useState<PostgresFlavor | null>(null)
  // Only meaningful while selected === 'browse-templates': the catalog
  // grid wants a wider dialog than its own configure-form view.
  const [templateView, setTemplateView] = useState<'grid' | 'detail'>('grid')
  const [search, setSearch] = useState('')
  // Set once the import front door has produced a plan preview; shown in
  // place of the picker until the operator goes back or deploys.
  const [importState, setImportState] = useState<{
    plan: ImportPlan
    request: ImportPlanRequest
  } | null>(null)
  const [category, setCategory] = useState<
    'all' | 'applications' | 'databases'
  >('all')
  const activeCategory = scope ?? category
  // Optional convenience, see useDatabaseEnginesOptional's own doc
  // comment: a slow/failed fetch just means step 1 shows the fixed
  // options without any database cards yet, never a blocked dialog.
  const engineList = useDatabaseEnginesOptional()
  const engines = engineList.data ?? []
  const isFixedOption = (value: string): value is FixedWizardOption =>
    value === 'docker-image' ||
    value === 'dockerfile-git' ||
    value === 'docker-compose' ||
    value === 'browse-templates'

  const pickerEntries = buildPickerEntries(engines, BRAND_ICON_NAMES)
  const selectedKey =
    selected === null || isFixedOption(selected) ? null : (flavor ?? selected)

  const normalizedSearch = search.trim().toLowerCase()
  const filteredApplications =
    activeCategory === 'databases'
      ? []
      : FIXED_OPTIONS.filter((option) =>
          matchesSearch(option, normalizedSearch),
        )
  const showDatabases = activeCategory !== 'applications'

  function handleOpenChange(next: boolean) {
    setOpen(next)
    onOpenChange?.(next)
    if (!next) {
      // A fresh step 1 next time the dialog opens (or back to
      // initialSelected, for a caller that always opens straight to
      // step 2); the field components below already reset their own
      // local form state off this same `open` prop.
      setSelected(initialSelected ?? null)
      setFlavor(null)
      setTemplateView('grid')
      setSearch('')
      setCategory('all')
      setImportState(null)
    }
  }

  function handleCreated() {
    handleOpenChange(false)
  }

  const databaseLabel =
    pickerEntries.find((e) => e.key === selectedKey)?.label ?? selected ?? ''

  const selectedTitle = selected
    ? isFixedOption(selected)
      ? selected === 'docker-image'
        ? 'New app from an image'
        : selected === 'dockerfile-git'
          ? 'New app from a git repo'
          : selected === 'docker-compose'
            ? 'New app from Docker Compose'
            : 'New app from a template'
      : `New ${databaseLabel} database`
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
          : selected === 'browse-templates'
            ? `Pick a service from the catalog, we'll pre-fill its compose.yaml and create one app per service it defines. ${appYamlAside}`
            : "We'll set this database up on your server and connect it automatically, no manual configuration needed."
    : ''

  const heroEntry = pickerEntries.find((e) => e.key === selectedKey)
  const heroMark =
    selected === 'docker-image' ? (
      <PackageGlassIcon aria-hidden="true" className="size-8" />
    ) : selected === 'dockerfile-git' ? (
      <GitBranchGlassIcon aria-hidden="true" className="size-8" />
    ) : selected !== null && isFixedOption(selected) ? (
      <LayersGlassIcon aria-hidden="true" className="size-8" />
    ) : heroEntry?.brand ? (
      <BrandIcon name={heroEntry.brand} className="size-8" />
    ) : heroEntry?.flavor ? (
      <span className="text-sm font-semibold" aria-hidden="true">
        {heroEntry.label.slice(0, 2)}
      </span>
    ) : (
      <DatabaseGlassIcon aria-hidden="true" className="size-8" />
    )
  const heroChips: string[] = []
  if (heroEntry?.port)
    heroChips.push(t('picker.chips.port', { port: heroEntry.port }))
  if (heroEntry?.compat === 'redis')
    heroChips.push(t('picker.chips.redisCompatible'))
  if (heroEntry?.compat === 'mysql')
    heroChips.push(t('picker.chips.mysqlCompatible'))
  if (heroEntry?.compat === 'postgres')
    heroChips.push(t('picker.chips.postgresCompatible'))

  const pickerTitle =
    scope === 'applications'
      ? 'New application'
      : scope === 'databases'
        ? t('picker.databaseTitle')
        : 'New resource'
  const pickerDescription =
    scope === 'databases'
      ? t('picker.databaseDescription')
      : 'Pick a starting point.'

  const wide =
    selected === null ||
    (selected === 'browse-templates' && templateView === 'grid')

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      {trigger ? <DialogTrigger render={trigger} /> : null}
      <DialogContent
        // sm:max-w-* overrides the default variant's sm:max-w-sm; the
        // base variant already caps height at 85vh and scrolls inside.
        className={wide ? 'sm:max-w-5xl' : 'sm:max-w-2xl'}
      >
        {importState !== null ? (
          <div className="contents">
            <DialogHeader>
              <DialogTitle>Deployment plan</DialogTitle>
              <DialogDescription>
                Review what will be created. Nothing exists until you deploy.
              </DialogDescription>
            </DialogHeader>
            <ImportPlanPreview
              plan={importState.plan}
              request={importState.request}
              onBack={() => {
                setImportState(null)
              }}
              onDeployed={handleCreated}
            />
          </div>
        ) : selected === null ? (
          <div className="contents">
            {/* Pinned while the card grid scrolls; negative margins make
                it span the dialog's own padding. */}
            <div className="sticky -top-4 z-10 -mx-4 -mt-4 space-y-3 border-b border-border bg-popover px-4 pt-4 pb-3">
              <DialogHeader>
                <DialogTitle>{pickerTitle}</DialogTitle>
                <DialogDescription>{pickerDescription}</DialogDescription>
              </DialogHeader>
              <div className="relative">
                <SearchGlassIcon
                  className="pointer-events-none absolute top-1/2 left-2.5 size-4.5 -translate-y-1/2 text-muted-foreground"
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
              {scope === undefined ? (
                <Tabs
                  value={category}
                  onValueChange={(value) => {
                    setCategory(value as typeof category)
                  }}
                >
                  <TabsList>
                    <TabsTrigger value="all">All</TabsTrigger>
                    <TabsTrigger value="applications">Applications</TabsTrigger>
                    <TabsTrigger value="databases">Databases</TabsTrigger>
                  </TabsList>
                </Tabs>
              ) : null}
            </div>
            {scope !== 'databases' ? (
              <ImportFrontDoor
                onPlan={(plan, request) => {
                  setImportState({ plan, request })
                }}
              />
            ) : null}
            {filteredApplications.length > 0 ? (
              <div className="space-y-2">
                <h3 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
                  Applications
                </h3>
                <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
                  {filteredApplications.map((option) => (
                    <OptionCard
                      key={option.value}
                      option={option}
                      onSelect={(value) => {
                        setFlavor(null)
                        setSelected(value)
                      }}
                    />
                  ))}
                </div>
              </div>
            ) : null}
            {showDatabases ? (
              <div className="space-y-2">
                {scope !== 'databases' && pickerEntries.length > 0 ? (
                  <h3 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
                    Databases
                  </h3>
                ) : null}
                <DatabasePicker
                  entries={pickerEntries}
                  query={normalizedSearch}
                  selectedKey={selectedKey}
                  showEmpty={filteredApplications.length === 0}
                  onClearQuery={() => {
                    setSearch('')
                  }}
                  onSelect={(entry) => {
                    setFlavor(entry.flavor)
                    setSelected(entry.engineId)
                  }}
                />
                <button
                  type="button"
                  onClick={() => {
                    setFlavor(null)
                    setSelected('docker-image')
                  }}
                  className="flex w-full items-center gap-3 rounded-lg border border-dashed border-border p-3 text-left text-sm transition-colors hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                >
                  <span className="flex size-10 shrink-0 items-center justify-center rounded-lg border border-border bg-background">
                    <PackageGlassIcon
                      aria-hidden="true"
                      className="size-5.5 text-muted-foreground"
                    />
                  </span>
                  <span className="min-w-0">
                    <span className="block font-medium">
                      {t('wizard.custom.title')}
                    </span>
                    <span className="block text-xs text-muted-foreground">
                      {t('wizard.custom.description')}
                    </span>
                  </span>
                </button>
              </div>
            ) : null}
            {activeCategory === 'applications' &&
            filteredApplications.length === 0 ? (
              <p className="py-6 text-center text-sm text-muted-foreground">
                No resources match &ldquo;{search}&rdquo;.
              </p>
            ) : null}
          </div>
        ) : (
          <div
            className={
              selected === 'browse-templates' && templateView === 'grid'
                ? 'mx-auto flex w-full max-w-6xl flex-col gap-4 py-2'
                : 'mx-auto flex w-full max-w-2xl flex-col gap-4 py-2'
            }
          >
            <StepHero
              mark={heroMark}
              title={selectedTitle}
              description={selectedDescription}
              chips={heroChips}
              accent={engineAccent(
                selected !== null && isFixedOption(selected)
                  ? null
                  : selectedKey,
              )}
              onBack={() => {
                setSelected(null)
              }}
            />
            {/* key forces a fresh mount (and so a blank form) whenever the
                picked option or flavor changes. */}
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
            {selected === 'browse-templates' ? (
              <BrowseTemplatesFields
                key={selected}
                open={open}
                onCreated={handleCreated}
                initialTemplateId={initialTemplateId}
                onViewChange={setTemplateView}
              />
            ) : null}
            {!isFixedOption(selected) ? (
              <CreateDatabaseFields
                key={selectedKey ?? selected}
                open={open}
                onCreated={handleCreated}
                engine={selected}
                flavor={flavor}
              />
            ) : null}
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
