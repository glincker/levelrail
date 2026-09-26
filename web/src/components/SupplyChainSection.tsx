import { Link } from '@tanstack/react-router'
import {
  DownloadSimpleIcon,
  ShieldCheckIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { InfoTip, StatusPill, type Tone } from './kit'
import {
  useSbom,
  useScanDeployment,
  useSupplyChainSettings,
  useVulnerabilities,
} from '../queries/supplyChain'
import type {
  GateAction,
  SbomSummary,
  VulnCounts,
  VulnReport,
  VulnSeverity,
} from '../types/supplyChain'

const SEVERITIES: { key: VulnSeverity; label: string; tone: Tone }[] = [
  { key: 'critical', label: 'Critical', tone: 'danger' },
  { key: 'high', label: 'High', tone: 'danger' },
  { key: 'medium', label: 'Medium', tone: 'warning' },
  { key: 'low', label: 'Low', tone: 'info' },
  { key: 'unknown', label: 'Unknown', tone: 'neutral' },
]

const TOP_PACKAGES_SHOWN = 5
const TOP_LICENSES_SHOWN = 3

const GATE_TEXT: Record<GateAction, string> = {
  allow: 'Allowed',
  warn: 'Released with a warning',
  block: 'Blocked, the previous release kept serving',
  override: 'Released by an operator override',
}

const GATE_TONE: Record<GateAction, Tone> = {
  allow: 'success',
  warn: 'warning',
  block: 'danger',
  override: 'warning',
}

export function VulnCountPills({ counts }: { counts: VulnCounts }) {
  const shown = SEVERITIES.filter((s) => counts[s.key] > 0)
  if (shown.length === 0) {
    return (
      <StatusPill tone="success" label="No known vulnerabilities" size="sm" />
    )
  }
  return (
    <div className="flex flex-wrap gap-1.5">
      {shown.map((s) => (
        <StatusPill
          key={s.key}
          tone={s.tone}
          size="sm"
          label={`${counts[s.key]} ${s.label.toLowerCase()}`}
        />
      ))}
    </div>
  )
}

function licenseSummary(sbom: SbomSummary): string {
  const top = sbom.licenses.slice(0, TOP_LICENSES_SHOWN)
  const parts = top.map((l) => `${l.license} ${l.count}`)
  if (sbom.unlicensed > 0) parts.push(`${sbom.unlicensed} without a license`)
  return parts.length > 0 ? parts.join(', ') : 'No license data'
}

function ScanResult({ report }: { report: VulnReport }) {
  const { scan, gate } = report
  if (scan.status === 'failed') {
    return (
      <p className="text-sm text-tone-danger">
        The last scan failed: {scan.error ?? 'unknown error'}
      </p>
    )
  }
  if (!scan.counts) {
    return <p className="text-sm text-muted-foreground">Not scanned yet.</p>
  }
  return (
    <div className="flex flex-col gap-2">
      <VulnCountPills counts={scan.counts} />
      {scan.top.length > 0 && (
        <ul className="flex flex-col gap-1 text-xs">
          {scan.top.slice(0, TOP_PACKAGES_SHOWN).map((v) => (
            <li key={`${v.id}-${v.package}`} className="break-words">
              <span className="font-mono">{v.id}</span> {v.package} {v.version}
              <span className="text-muted-foreground">
                {v.fixed_version
                  ? ` fixed in ${v.fixed_version}`
                  : ' no fix yet'}
              </span>
            </li>
          ))}
        </ul>
      )}
      {gate && gate.action !== 'allow' && (
        <div className="flex flex-wrap items-center gap-2">
          <StatusPill
            tone={GATE_TONE[gate.action]}
            size="sm"
            label={GATE_TEXT[gate.action]}
          />
          {gate.reason && (
            <span className="text-xs text-muted-foreground">{gate.reason}</span>
          )}
        </div>
      )}
    </div>
  )
}

export interface SupplyChainSectionProps {
  appName: string
  deploymentId: string
  /** From the deploy list: null or undefined means no SBOM was recorded. */
  sbomPackages: number | null | undefined
}

// SupplyChainSection shows the SBOM and vulnerability scan of one deploy
// (GET .../deployments/{id}/sbom and .../vulnerabilities) with a Scan now
// action. It only fetches when the deploy list says an SBOM exists.
export function SupplyChainSection({
  appName,
  deploymentId,
  sbomPackages,
}: Readonly<SupplyChainSectionProps>) {
  const hasSbom = sbomPackages !== null && sbomPackages !== undefined
  const sbom = useSbom(appName, deploymentId, hasSbom)
  const vulns = useVulnerabilities(appName, deploymentId, hasSbom)
  const settings = useSupplyChainSettings(appName)
  const scan = useScanDeployment(appName, deploymentId)

  if (!hasSbom) {
    return (
      <p className="text-sm text-muted-foreground">
        No software bill of materials was recorded for this deploy. SBOMs are
        made for Dockerfile builds when the server sets{' '}
        <code className="font-mono text-xs">APP_BUILD_ATTEST=true</code>.
      </p>
    )
  }
  if (sbom.isPending) {
    return <p className="text-sm text-muted-foreground">Loading</p>
  }
  if (sbom.isError) {
    return (
      <p className="text-sm text-muted-foreground">
        The software bill of materials could not be loaded.
      </p>
    )
  }

  const data = sbom.data
  const canScan = settings.data?.scan_enabled === true
  const scanDisabledHint =
    settings.data?.server_enabled === false
      ? 'Scanning is switched off on this server.'
      : 'Turn on scanning in Deploy settings to scan this deploy.'

  function scanNow() {
    scan.mutate(undefined, {
      onSuccess: () => toast.add({ title: 'Scan finished.', type: 'success' }),
      onError: (error) =>
        toast.add({
          title: 'Scan failed.',
          description: error.message,
          type: 'error',
        }),
    })
  }

  return (
    <div className="flex flex-col gap-3">
      <dl className="flex flex-col gap-1 text-sm">
        <div className="flex items-baseline justify-between gap-4">
          <dt className="text-muted-foreground">Packages</dt>
          <dd>
            {data.package_count}
            <span className="ml-1 text-xs text-muted-foreground">
              {data.format === 'spdx' ? 'SPDX' : 'CycloneDX'}
              {data.provenance ? ', provenance recorded' : ''}
            </span>
          </dd>
        </div>
        <div className="flex items-baseline justify-between gap-4">
          <dt className="shrink-0 text-muted-foreground">Licenses</dt>
          <dd className="min-w-0 text-right break-words">
            {licenseSummary(data)}
          </dd>
        </div>
      </dl>
      {data.top_packages.length > 0 && (
        <ul className="flex flex-col gap-0.5 font-mono text-xs">
          {data.top_packages.slice(0, TOP_PACKAGES_SHOWN).map((p) => (
            <li key={`${p.name}@${p.version ?? ''}`} className="break-words">
              {p.name}
              {p.version ? ` ${p.version}` : ''}
            </li>
          ))}
        </ul>
      )}
      {vulns.data ? <ScanResult report={vulns.data} /> : null}
      <div className="flex flex-wrap items-center gap-2">
        <Button
          size="sm"
          variant="outline"
          onClick={scanNow}
          disabled={!canScan || scan.isPending}
        >
          <ShieldCheckIcon className="size-4" aria-hidden="true" />
          {scan.isPending ? 'Scanning' : 'Scan now'}
        </Button>
        <InfoTip label="About scanning">
          Runs the scanner in a short-lived container against this deploy's
          SBOM. The first scan pulls the scanner image and its vulnerability
          database, so it can take a few minutes.
        </InfoTip>
        {data.available && data.download_url && (
          <Button
            variant="ghost"
            size="sm"
            nativeButton={false}
            render={<a href={data.download_url} download />}
          >
            <DownloadSimpleIcon className="size-4" aria-hidden="true" />
            SBOM
          </Button>
        )}
        {!canScan && (
          <span className="text-xs text-muted-foreground">
            {scanDisabledHint}{' '}
            <Link
              to="/apps/$name/deploy-settings"
              params={{ name: appName }}
              className="underline underline-offset-4"
            >
              Deploy settings
            </Link>
          </span>
        )}
      </div>
    </div>
  )
}
