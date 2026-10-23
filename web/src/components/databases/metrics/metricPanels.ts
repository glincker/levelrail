export type PanelId = 'memory' | 'cpu' | 'network' | 'disk'

const DISK_BACKED: PanelId[] = ['memory', 'cpu', 'network', 'disk']
const IN_MEMORY: PanelId[] = ['memory', 'cpu', 'network']

// In-memory engines keep nothing on disk, so a disk panel would stay flat.
export function panelsForEngine(hasDisk: boolean): PanelId[] {
  return hasDisk ? DISK_BACKED : IN_MEMORY
}

export type MetricsEmptyReason = 'stopped' | 'provisioning' | null

export function metricsEmptyReason(db: {
  suspended?: boolean
  provision_state?: string
}): MetricsEmptyReason {
  if (db.provision_state) return 'provisioning'
  if (db.suspended) return 'stopped'
  return null
}
