import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { appKeys } from '../../queries/apps'
import type { WorkspacePackage } from '../../queries/importAnalyze'
import { createPackageApp, type PackageAppState } from '../../lib/packageApps'
import type { GitRepoSourceValue } from '../GitRepoSourcePicker'

/**
 * Workspace packages found in a monorepo: create one app per selected
 * package, or configure a single one through the stepper.
 */
export function MonorepoPackages({
  packages,
  source,
  onPick,
}: {
  packages: WorkspacePackage[]
  source: GitRepoSourceValue
  onPick: (pkg: WorkspacePackage) => void
}) {
  const { t } = useTranslation('newApp')
  const queryClient = useQueryClient()
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [state, setState] = useState<Record<string, PackageAppState>>({})
  const [busy, setBusy] = useState(false)

  const patch = (dir: string, next: PackageAppState) => {
    setState((prev) => ({ ...prev, [dir]: next }))
  }

  async function createAll() {
    setBusy(true)
    for (const pkg of packages) {
      if (!selected.has(pkg.dir) || state[pkg.dir]?.status === 'done') continue
      patch(pkg.dir, { status: 'creating' })
      try {
        const res = await createPackageApp(source, pkg)
        patch(pkg.dir, {
          status: 'done',
          appName: res.appName,
          deployId: res.deployId,
        })
      } catch (err) {
        patch(pkg.dir, {
          status: 'failed',
          message: err instanceof Error ? err.message : String(err),
        })
      }
    }
    void queryClient.invalidateQueries({ queryKey: appKeys.list() })
    setBusy(false)
  }

  return (
    <section className="space-y-3" aria-labelledby="f-packages">
      <h3 id="f-packages" className="text-sm font-semibold">
        {t('packages.title')}
      </h3>
      <p className="text-xs text-muted-foreground">{t('packages.hint')}</p>
      <ul className="divide-y divide-border rounded-md border border-border">
        {packages.map((pkg) => {
          const st = state[pkg.dir] ?? { status: 'idle' }
          const id = `pkg-${pkg.dir}`
          return (
            <li key={pkg.dir} className="space-y-1 px-3 py-2">
              <div className="flex flex-wrap items-center gap-3">
                <Checkbox
                  id={id}
                  checked={selected.has(pkg.dir)}
                  disabled={busy || st.status === 'done'}
                  onCheckedChange={(v) => {
                    setSelected((prev) => {
                      const next = new Set(prev)
                      if (v === true) next.add(pkg.dir)
                      else next.delete(pkg.dir)
                      return next
                    })
                  }}
                />
                <Label
                  htmlFor={id}
                  className="min-w-0 flex-1 flex-col items-start"
                >
                  <span className="font-medium">{pkg.name}</span>
                  <span className="font-mono text-xs text-muted-foreground">
                    {pkg.dir}
                  </span>
                </Label>
                {pkg.dockerfile ? (
                  <Badge variant="muted">{t('packages.dockerfile')}</Badge>
                ) : null}
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={busy}
                  onClick={() => {
                    onPick(pkg)
                  }}
                >
                  {t('packages.configure')}
                </Button>
              </div>
              {st.status === 'creating' ? (
                <p className="text-xs text-muted-foreground" role="status">
                  {t('packages.creating')}
                </p>
              ) : null}
              {st.status === 'done' && st.appName && st.deployId ? (
                <p className="text-xs">
                  <Link
                    className="underline underline-offset-2"
                    to="/apps/$name/deploys/$deployId/logs"
                    params={{ name: st.appName, deployId: st.deployId }}
                  >
                    {t('packages.open', { name: st.appName })}
                  </Link>
                </p>
              ) : null}
              {st.status === 'failed' ? (
                <Alert variant="destructive">
                  <AlertDescription>{st.message}</AlertDescription>
                </Alert>
              ) : null}
            </li>
          )
        })}
      </ul>
      <Button
        type="button"
        disabled={busy || selected.size === 0}
        onClick={() => {
          void createAll()
        }}
      >
        {busy
          ? t('packages.creatingAll')
          : t('packages.createSelected', { count: selected.size })}
      </Button>
      <p className="text-xs text-muted-foreground">{t('packages.envNote')}</p>
    </section>
  )
}
