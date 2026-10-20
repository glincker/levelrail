export const CLASS_TITLE = {
  dangling_images: 'capacity.classes.dangling_images.title',
  build_cache: 'capacity.classes.build_cache.title',
  stopped_containers: 'capacity.classes.stopped_containers.title',
  rollback_images: 'capacity.classes.rollback_images.title',
  orphaned_volumes: 'capacity.classes.orphaned_volumes.title',
} as const

export const CLASS_HINT = {
  dangling_images: 'capacity.classes.dangling_images.hint',
  build_cache: 'capacity.classes.build_cache.hint',
  stopped_containers: 'capacity.classes.stopped_containers.hint',
  rollback_images: 'capacity.classes.rollback_images.hint',
  orphaned_volumes: 'capacity.classes.orphaned_volumes.hint',
} as const

export const METRIC_LABEL = {
  disk: 'capacity.metrics.disk',
  inodes: 'capacity.metrics.inodes',
  memory: 'capacity.metrics.memory',
  swap: 'capacity.metrics.swap',
} as const

export const FIELD_LABEL = {
  diskWarn: 'capacity.fields.diskWarn',
  diskCrit: 'capacity.fields.diskCrit',
  inodeWarn: 'capacity.fields.inodeWarn',
  inodeCrit: 'capacity.fields.inodeCrit',
  memWarn: 'capacity.fields.memWarn',
  memCrit: 'capacity.fields.memCrit',
  swapWarn: 'capacity.fields.swapWarn',
  swapCrit: 'capacity.fields.swapCrit',
} as const
