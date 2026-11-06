import type storageCli from '../../locales/en/storageCli.json'

export type StorageCliTab = 'usage' | 'clean' | 'automate'
export type StorageCliRole = 'viewer' | 'admin'

export interface StorageCliCommand {
  id: string
  tab: StorageCliTab
  command: string
  /** Key under storageCli:commands for the one-line description. */
  description: keyof typeof storageCli.commands
  minRole: StorageCliRole
  /** Appends --node when the dialog knows which node the user is looking at. */
  nodeAware?: boolean
  /** Needs the write:sensitive scope on top of the admin role. */
  sensitive?: boolean
}

export const STORAGE_CLI_TABS: StorageCliTab[] = ['usage', 'clean', 'automate']

// Subcommands verified in cmd/levelrail-cli: main.go (doctor, janitor,
// volumes-orphaned, volumes-orphaned-cleanup), apps_build_cache.go,
// janitor.go, volumes_orphaned_cleanup.go and the images, builds and
// containers command files. Dry run comes before apply in array order.
// system-prune is left out on purpose: it removes unused volumes fleet-wide.
export const STORAGE_CLI_COMMANDS: StorageCliCommand[] = [
  {
    id: 'doctorDisk',
    tab: 'usage',
    command: 'levelrail-cli doctor disk',
    description: 'doctorDisk',
    minRole: 'viewer',
  },
  {
    id: 'imagesList',
    tab: 'usage',
    command: 'levelrail-cli images list',
    description: 'imagesList',
    minRole: 'viewer',
    nodeAware: true,
  },
  {
    id: 'janitorStatus',
    tab: 'usage',
    command: 'levelrail-cli janitor status',
    description: 'janitorStatus',
    minRole: 'viewer',
  },
  {
    id: 'buildsCache',
    tab: 'usage',
    command: 'levelrail-cli builds cache',
    description: 'buildsCache',
    minRole: 'viewer',
  },
  {
    id: 'buildCacheShow',
    tab: 'usage',
    command: 'levelrail-cli apps build-cache show --global',
    description: 'buildCacheShow',
    minRole: 'viewer',
  },
  {
    id: 'volumesOrphaned',
    tab: 'usage',
    command: 'levelrail-cli volumes-orphaned',
    description: 'volumesOrphaned',
    minRole: 'viewer',
  },
  {
    id: 'containersOrphans',
    tab: 'usage',
    command: 'levelrail-cli containers orphans',
    description: 'containersOrphans',
    minRole: 'viewer',
  },
  {
    id: 'janitorDryRun',
    tab: 'clean',
    command: 'levelrail-cli janitor run --dry-run',
    description: 'janitorDryRun',
    minRole: 'admin',
  },
  {
    id: 'janitorRun',
    tab: 'clean',
    command: 'levelrail-cli janitor run',
    description: 'janitorRun',
    minRole: 'admin',
  },
  {
    id: 'imagesGcPreview',
    tab: 'clean',
    command: 'levelrail-cli images gc',
    description: 'imagesGcPreview',
    minRole: 'admin',
    nodeAware: true,
  },
  {
    id: 'imagesGcApply',
    tab: 'clean',
    command: 'levelrail-cli images gc --apply',
    description: 'imagesGcApply',
    minRole: 'admin',
    nodeAware: true,
  },
  {
    id: 'imagesPruneDry',
    tab: 'clean',
    command: 'levelrail-cli images prune --unmanaged --dry-run',
    description: 'imagesPruneDry',
    minRole: 'admin',
    nodeAware: true,
  },
  {
    id: 'buildsCacheApply',
    tab: 'clean',
    command: 'levelrail-cli builds cache --apply',
    description: 'buildsCacheApply',
    minRole: 'admin',
    sensitive: true,
  },
  {
    id: 'volumesCleanup',
    tab: 'clean',
    command: 'levelrail-cli volumes-orphaned-cleanup --names a,b',
    description: 'volumesCleanup',
    minRole: 'admin',
  },
  {
    id: 'containersReap',
    tab: 'clean',
    command: 'levelrail-cli containers reap',
    description: 'containersReap',
    minRole: 'admin',
  },
  {
    id: 'janitorPolicy',
    tab: 'automate',
    command:
      'levelrail-cli janitor policy --enabled=true --dry-run=false --build-cache-max-age-hours 168 --disk-free-budget-percent 15',
    description: 'janitorPolicy',
    minRole: 'admin',
  },
]

const ROLE_RANK: Record<StorageCliRole, number> = { viewer: 0, admin: 1 }

export function roleAllows(
  role: StorageCliRole,
  minRole: StorageCliRole,
): boolean {
  return ROLE_RANK[role] >= ROLE_RANK[minRole]
}

/** The runnable command, personalised with the node when the entry supports it. */
export function commandFor(
  entry: StorageCliCommand,
  node: string | undefined,
): string {
  return entry.nodeAware && node
    ? `${entry.command} --node ${node}`
    : entry.command
}
