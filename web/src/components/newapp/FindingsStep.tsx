import { lazy, Suspense, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckCircleIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  normalizeAnalysis,
  type RawImportAnalysis,
} from '../../queries/importAnalyze'
import {
  APP_NAME_PATTERN,
  validateEdits,
  type NewAppEdits,
} from '../../lib/newApp'
import type { GitSourceBuildType } from '../../types/gitSource'
import type { GitRepoSourceValue } from '../GitRepoSourcePicker'
import { DatabaseFinding, EnvFinding, Finding } from './FindingParts'
import { DetectionSummary } from './DetectionSummary'
import { MonorepoPackages } from './MonorepoPackages'

const ImportPlanPreview = lazy(() =>
  import('./ImportPlanPreview').then((m) => ({ default: m.ImportPlanPreview })),
)

const BUILD_TYPES: GitSourceBuildType[] = ['railpack', 'dockerfile', 'static']

/**
 * Step 2, "What we found": every item is a pre-filled, editable suggestion
 * with the evidence that produced it.
 */
export function FindingsStep({
  analysis: rawAnalysis,
  source,
  edits,
  onEdits,
  onReanalyze,
  onBack,
  onNext,
}: {
  analysis: RawImportAnalysis
  source: GitRepoSourceValue
  edits: NewAppEdits
  onEdits: (next: NewAppEdits) => void
  onReanalyze: (override: {
    base_directory: string
    dockerfile_path: string
    build_type: string
  }) => void
  onBack: () => void
  onNext: () => void
}) {
  const { t } = useTranslation('newApp')
  const analysis = useMemo(() => normalizeAnalysis(rawAnalysis), [rawAnalysis])
  const set = <K extends keyof NewAppEdits>(key: K, value: NewAppEdits[K]) => {
    onEdits({ ...edits, [key]: value })
  }
  const problems = validateEdits(edits)
  const required = analysis.env.filter((k) => k.required)
  const optional = analysis.env.filter((k) => !k.required)
  const alternatives = analysis.build.alternatives
  const packages = analysis.packages ?? []

  return (
    <div className="space-y-5">
      {analysis.warnings.map((w) => (
        <Alert key={w.code}>
          <AlertDescription>
            {t(`findings.warnings.${w.code}`, { defaultValue: w.message })}
          </AlertDescription>
        </Alert>
      ))}

      {analysis.detection ? (
        <DetectionSummary
          detection={analysis.detection}
          hideWarnings={analysis.warnings.map((w) => w.code)}
        />
      ) : null}

      {analysis.monorepo && packages.length > 0 ? (
        <Suspense fallback={null}>
          <ImportPlanPreview source={source} />
        </Suspense>
      ) : null}

      {analysis.monorepo && packages.length > 0 ? (
        <MonorepoPackages
          packages={packages}
          source={source}
          onPick={(pkg) => {
            onReanalyze({
              base_directory: pkg.dir,
              dockerfile_path: pkg.dockerfile ?? '',
              build_type: pkg.dockerfile ? 'dockerfile' : 'railpack',
            })
          }}
        />
      ) : null}

      <section className="space-y-3" aria-labelledby="f-app">
        <h3 id="f-app" className="text-sm font-semibold">
          {t('findings.app.title')}
        </h3>
        <Finding
          id="f-name"
          label={t('findings.app.name')}
          evidence={analysis.name.evidence}
        >
          <Input
            id="f-name"
            value={edits.name}
            aria-invalid={!APP_NAME_PATTERN.test(edits.name)}
            onChange={(e) => {
              set('name', e.target.value.toLowerCase())
            }}
          />
        </Finding>
        <Finding
          id="f-domain"
          label={t('findings.app.domain')}
          evidence={analysis.domain.evidence}
        >
          <Input
            id="f-domain"
            value={edits.domain}
            placeholder="app.example.com"
            onChange={(e) => {
              set('domain', e.target.value.trim())
            }}
          />
        </Finding>
        <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <Badge variant="info">
            {analysis.stack.framework ?? analysis.stack.runtime}
          </Badge>
          <span>{analysis.stack.evidence}</span>
        </div>
      </section>

      <section className="space-y-3" aria-labelledby="f-build">
        <h3 id="f-build" className="text-sm font-semibold">
          {t('findings.build.title')}
        </h3>
        {alternatives.length > 1 ? (
          <Finding
            id="f-alt"
            label={t('findings.build.app')}
            evidence={t('findings.build.appHint')}
          >
            <Select
              value={`${edits.baseDirectory}|${edits.dockerfilePath}`}
              onValueChange={(v) => {
                const alt = alternatives.find(
                  (a) =>
                    `${a.base_directory ?? ''}|${a.dockerfile_path ?? ''}` ===
                    v,
                )
                if (alt) {
                  onReanalyze({
                    base_directory: alt.base_directory ?? '',
                    dockerfile_path: alt.dockerfile_path ?? '',
                    build_type: alt.build_type,
                  })
                }
              }}
            >
              <SelectTrigger id="f-alt" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {alternatives.map((a) => {
                  const value = `${a.base_directory ?? ''}|${a.dockerfile_path ?? ''}`
                  return (
                    <SelectItem key={value} value={value}>
                      {a.dockerfile_path || a.base_directory || '.'}
                    </SelectItem>
                  )
                })}
              </SelectContent>
            </Select>
          </Finding>
        ) : null}
        <p className="text-xs text-muted-foreground">
          {analysis.build.evidence}
        </p>
        <Finding
          id="f-port"
          label={t('findings.port.label')}
          evidence={analysis.port.evidence}
        >
          <Input
            id="f-port"
            inputMode="numeric"
            value={edits.port > 0 ? String(edits.port) : ''}
            aria-invalid={problems.includes('port')}
            onChange={(e) => {
              set('port', Number.parseInt(e.target.value, 10) || 0)
            }}
          />
        </Finding>
        <Finding
          id="f-health"
          label={t('findings.health.label')}
          evidence={analysis.health.evidence}
        >
          <Input
            id="f-health"
            value={edits.healthPath}
            className="font-mono text-xs"
            onChange={(e) => {
              set('healthPath', e.target.value)
            }}
          />
        </Finding>
      </section>

      {analysis.databases.length > 0 ? (
        <section className="space-y-3" aria-labelledby="f-db">
          <h3 id="f-db" className="text-sm font-semibold">
            {t('findings.databases.title')}
          </h3>
          {analysis.databases.map((need, i) => {
            const edit = edits.databases[i]
            if (!edit) return null
            return (
              <DatabaseFinding
                key={need.engine}
                need={need}
                edit={edit}
                onChange={(next) => {
                  set(
                    'databases',
                    edits.databases.map((d, j) => (j === i ? next : d)),
                  )
                }}
              />
            )
          })}
        </section>
      ) : null}

      <section className="space-y-3" aria-labelledby="f-env">
        <h3 id="f-env" className="text-sm font-semibold">
          {t('findings.env.title')}
        </h3>
        {analysis.env.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('findings.env.none')}
          </p>
        ) : null}
        {required.map((k) => (
          <EnvFinding
            key={k.key}
            entry={k}
            value={edits.env[k.key] ?? ''}
            onChange={(v) => {
              set('env', { ...edits.env, [k.key]: v })
            }}
          />
        ))}
        {optional.length > 0 ? (
          <Collapsible>
            <CollapsibleTrigger className="text-xs text-muted-foreground underline underline-offset-2">
              {t('findings.env.optional', { count: optional.length })}
            </CollapsibleTrigger>
            <CollapsiblePanel className="space-y-3 pt-3">
              {optional.map((k) => (
                <EnvFinding
                  key={k.key}
                  entry={k}
                  value={edits.env[k.key] ?? ''}
                  onChange={(v) => {
                    set('env', { ...edits.env, [k.key]: v })
                  }}
                />
              ))}
            </CollapsiblePanel>
          </Collapsible>
        ) : null}
        <p className="text-xs text-muted-foreground">
          {t('findings.env.noValues')}
        </p>
      </section>

      <Collapsible>
        <CollapsibleTrigger className="text-sm text-muted-foreground underline underline-offset-2">
          {t('findings.advanced.title')}
        </CollapsibleTrigger>
        <CollapsiblePanel className="space-y-3 pt-3">
          <Finding
            id="f-btype"
            label={t('findings.advanced.buildType')}
            evidence=""
          >
            <Select
              value={edits.buildType}
              onValueChange={(v) => {
                set('buildType', v as GitSourceBuildType)
              }}
            >
              <SelectTrigger id="f-btype" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {BUILD_TYPES.map((b) => (
                  <SelectItem key={b} value={b}>
                    {t(`findings.advanced.types.${b}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Finding>
          <Finding
            id="f-dockerfile"
            label={t('findings.advanced.dockerfile')}
            evidence=""
          >
            <Input
              id="f-dockerfile"
              value={edits.dockerfilePath}
              className="font-mono text-xs"
              onChange={(e) => {
                set('dockerfilePath', e.target.value)
              }}
            />
          </Finding>
          <Finding
            id="f-base"
            label={t('findings.advanced.baseDirectory')}
            evidence=""
          >
            <div className="flex gap-2">
              <Input
                id="f-base"
                value={edits.baseDirectory}
                className="font-mono text-xs"
                onChange={(e) => {
                  set('baseDirectory', e.target.value)
                }}
              />
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  onReanalyze({
                    base_directory: edits.baseDirectory.trim(),
                    dockerfile_path: edits.dockerfilePath,
                    build_type: edits.buildType,
                  })
                }}
              >
                <CheckCircleIcon aria-hidden="true" />
                {t('findings.advanced.validateDirectory')}
              </Button>
            </div>
          </Finding>
          <Finding
            id="f-mem"
            label={t('findings.advanced.memory')}
            evidence={analysis.memory.evidence}
          >
            <Input
              id="f-mem"
              inputMode="numeric"
              value={String(edits.memoryMb)}
              onChange={(e) => {
                set('memoryMb', Number.parseInt(e.target.value, 10) || 0)
              }}
            />
          </Finding>
        </CollapsiblePanel>
      </Collapsible>

      <div className="flex flex-wrap justify-between gap-2">
        <Button type="button" variant="outline" onClick={onBack}>
          {t('nav.back')}
        </Button>
        <Button type="button" disabled={problems.length > 0} onClick={onNext}>
          {t('nav.review')}
        </Button>
      </div>
    </div>
  )
}
