import type { AppDetail } from '../types/appDetail'
import type { ReconcileCondition } from '../types/deploy'
import { digestOfImage, unpinnedImage } from './imageDigest'

export type TreeGroupId =
  'deployment' | 'replicas' | 'domains' | 'volumes' | 'databases' | 'env'

export interface TreeLeaf {
  label: string
  detail?: string
  // Full digest for PinnedDigestBadge, only set on the deployment leaf.
  digest?: string
}

export interface TreeGroup {
  id: TreeGroupId
  // Count shown next to the group title; undefined for single-value groups.
  count?: number
  leaves: TreeLeaf[]
}

export interface AppResourceTree {
  name: string
  groups: TreeGroup[]
}

type TreeApp = Pick<
  AppDetail,
  | 'name'
  | 'image'
  | 'image_digest'
  | 'replicas'
  | 'domains'
  | 'volumes'
  | 'database_env'
  | 'database_attachment'
  | 'env'
  | 'secret_env'
  | 'vault_env'
>

// Env values are never read: only the number of distinct keys is exposed.
export function buildAppResourceTree(
  app: TreeApp,
  conditions: ReconcileCondition[] = [],
): AppResourceTree {
  const groups: TreeGroup[] = []
  if (app.image) {
    groups.push({
      id: 'deployment',
      leaves: [
        {
          label: unpinnedImage(app.image),
          digest: app.image_digest || digestOfImage(app.image) || undefined,
        },
      ],
    })
  }
  const replicas = app.replicas > 0 ? app.replicas : 1
  groups.push({
    id: 'replicas',
    count: replicas,
    leaves: conditions.map((c) => ({
      label: c.Type,
      detail: c.Reason ? `${c.Status} (${c.Reason})` : c.Status,
    })),
  })
  const domains = [...(app.domains ?? [])].sort()
  if (domains.length > 0) {
    groups.push({
      id: 'domains',
      count: domains.length,
      leaves: domains.map((d) => ({ label: d })),
    })
  }
  const volumes = app.volumes ?? []
  if (volumes.length > 0) {
    groups.push({
      id: 'volumes',
      count: volumes.length,
      leaves: volumes.map((v) => ({ label: v.name, detail: v.container_path })),
    })
  }
  const dbs = new Map<string, TreeLeaf>()
  for (const [envVar, ref] of Object.entries(app.database_env ?? {})) {
    dbs.set(`${ref.database}:${envVar}`, {
      label: ref.database,
      detail: envVar,
    })
  }
  const att = app.database_attachment
  if (att && !dbs.has(`${att.database_name}:${att.env_var}`)) {
    dbs.set(`${att.database_name}:${att.env_var}`, {
      label: att.database_name,
      detail: att.env_var,
    })
  }
  if (dbs.size > 0) {
    groups.push({ id: 'databases', count: dbs.size, leaves: [...dbs.values()] })
  }
  const keys = new Set([
    ...Object.keys(app.env ?? {}),
    ...(app.secret_env ?? []),
    ...Object.keys(app.vault_env ?? {}),
  ])
  if (keys.size > 0) groups.push({ id: 'env', count: keys.size, leaves: [] })
  return { name: app.name, groups }
}
