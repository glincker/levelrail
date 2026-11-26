import type { CreateBackupTargetRequest } from '../../../types/backupTarget'
import type { BrandIconName } from '../../BrandIcon'

export type PresetId =
  'r2' | 'aws' | 'b2' | 'wasabi' | 'spaces' | 'minio' | 'local'
export type FieldId = 'accountId' | 'region' | 'endpoint' | 'bucket' | 'path'

export interface Preset {
  id: PresetId
  label: string
  brand?: BrandIconName
  fields: FieldId[]
  regions?: string[]
  defaultRegion?: string
  // Endpoint template for presets that derive it from account or region.
  endpoint?: (v: PresetValues) => string
}

export interface PresetValues {
  accountId: string
  region: string
  endpoint: string
  bucket: string
  path: string
  accessKey: string
  secret: string
}

export const EMPTY_VALUES: PresetValues = {
  accountId: '',
  region: '',
  endpoint: '',
  bucket: '',
  path: '',
  accessKey: '',
  secret: '',
}

// Backend accepts aws, r2, custom (any S3 compatible) and local; the vendor presets
// below are custom targets with a prefilled endpoint.
export const PRESETS: Preset[] = [
  {
    id: 'r2',
    label: 'Cloudflare R2',
    brand: 'cloudflare',
    fields: ['accountId', 'bucket'],
    endpoint: (v) => `https://${v.accountId.trim()}.r2.cloudflarestorage.com`,
  },
  {
    id: 'aws',
    label: 'Amazon S3',
    brand: 'aws',
    fields: ['region', 'bucket'],
    regions: [
      'us-east-1',
      'us-east-2',
      'us-west-2',
      'eu-west-1',
      'eu-central-1',
      'ap-southeast-1',
      'ap-northeast-1',
    ],
    defaultRegion: 'us-east-1',
  },
  {
    id: 'b2',
    label: 'Backblaze B2',
    brand: 'backblaze',
    fields: ['region', 'bucket'],
    regions: ['us-west-004', 'us-west-002', 'us-west-001', 'eu-central-003'],
    defaultRegion: 'us-west-004',
    endpoint: (v) => `https://s3.${v.region}.backblazeb2.com`,
  },
  {
    id: 'wasabi',
    label: 'Wasabi',
    brand: 'wasabi',
    fields: ['region', 'bucket'],
    regions: [
      'us-east-1',
      'us-east-2',
      'us-west-1',
      'eu-central-1',
      'eu-west-1',
    ],
    defaultRegion: 'us-east-1',
    endpoint: (v) => `https://s3.${v.region}.wasabisys.com`,
  },
  {
    id: 'spaces',
    label: 'DigitalOcean Spaces',
    brand: 'digitalocean',
    fields: ['region', 'bucket'],
    regions: ['nyc3', 'sfo3', 'ams3', 'fra1', 'sgp1'],
    defaultRegion: 'nyc3',
    endpoint: (v) => `https://${v.region}.digitaloceanspaces.com`,
  },
  {
    id: 'minio',
    label: 'MinIO',
    brand: 'minio',
    fields: ['endpoint', 'bucket'],
  },
  { id: 'local', label: 'Local disk', fields: ['path'] },
]

/** Builds the create request for a preset, or null while a required field is empty. */
export function buildTargetRequest(
  preset: Preset,
  name: string,
  v: PresetValues,
): CreateBackupTargetRequest | null {
  if (name.trim() === '') return null
  if (preset.id === 'local') {
    if (v.path.trim() === '') return null
    return {
      name: name.trim(),
      provider: 'local',
      bucket: v.path.trim(),
      access_key_id: '',
      secret_access_key: '',
    }
  }
  if (v.accessKey.trim() === '' || v.secret === '' || v.bucket.trim() === '') {
    return null
  }
  const base = {
    name: name.trim(),
    bucket: v.bucket.trim(),
    access_key_id: v.accessKey.trim(),
    secret_access_key: v.secret,
  }
  if (preset.id === 'aws') {
    return v.region ? { ...base, provider: 'aws', region: v.region } : null
  }
  if (preset.id === 'r2') {
    return v.accountId.trim()
      ? {
          ...base,
          provider: 'r2',
          endpoint: preset.endpoint?.(v),
          region: 'auto',
        }
      : null
  }
  const endpoint = preset.endpoint ? preset.endpoint(v) : v.endpoint.trim()
  if (endpoint === '' || (preset.regions && !v.region)) return null
  return {
    ...base,
    provider: 'custom',
    endpoint,
    ...(v.region ? { region: v.region } : {}),
  }
}
