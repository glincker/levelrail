import type {
  BackupProvider,
  CreateBackupTargetRequest,
} from '../../types/backupTarget'

export type PresetId = 'r2' | 'b2' | 'wasabi' | 'aws' | 'minio' | 'local'

export interface TargetPreset {
  id: PresetId
  provider: BackupProvider
  /** Endpoint pattern; {region} is replaced. Empty means AWS' own. */
  endpoint: string
  region: string
  /** Whether the operator must supply the endpoint by hand. */
  endpointRequired: boolean
  /** Backups on the same machine do not survive the machine. */
  sameMachine: boolean
}

export const TARGET_PRESETS: TargetPreset[] = [
  {
    id: 'r2',
    provider: 'r2',
    endpoint: 'https://<account-id>.r2.cloudflarestorage.com',
    region: 'auto',
    endpointRequired: true,
    sameMachine: false,
  },
  {
    id: 'b2',
    provider: 'custom',
    endpoint: 'https://s3.{region}.backblazeb2.com',
    region: 'us-west-004',
    endpointRequired: false,
    sameMachine: false,
  },
  {
    id: 'wasabi',
    provider: 'custom',
    endpoint: 'https://s3.{region}.wasabisys.com',
    region: 'us-east-1',
    endpointRequired: false,
    sameMachine: false,
  },
  {
    id: 'aws',
    provider: 'aws',
    endpoint: '',
    region: 'us-east-1',
    endpointRequired: false,
    sameMachine: false,
  },
  {
    id: 'minio',
    provider: 'custom',
    endpoint: 'https://minio.example.com',
    region: 'us-east-1',
    endpointRequired: true,
    sameMachine: false,
  },
  {
    id: 'local',
    provider: 'custom',
    endpoint: 'http://127.0.0.1:9000',
    region: 'us-east-1',
    endpointRequired: false,
    sameMachine: true,
  },
]

export interface TargetFormValues {
  name: string
  bucket: string
  region: string
  endpoint: string
  accessKeyId: string
  secretAccessKey: string
}

export function presetById(id: PresetId): TargetPreset {
  const found = TARGET_PRESETS.find((p) => p.id === id)
  if (!found) throw new Error(`unknown backup target preset ${id}`)
  return found
}

/** resolveEndpoint fills the preset's endpoint pattern, or returns what the operator typed. */
export function resolveEndpoint(
  preset: TargetPreset,
  typed: string,
  region: string,
): string {
  const explicit = typed.trim()
  if (explicit) return explicit
  if (preset.endpointRequired && preset.id !== 'local') return ''
  return preset.endpoint.replace('{region}', region.trim() || preset.region)
}

/** buildTargetRequest turns form values into the API body, or returns the first missing field. */
export function buildTargetRequest(
  preset: TargetPreset,
  v: TargetFormValues,
):
  { request: CreateBackupTargetRequest } | { missing: keyof TargetFormValues } {
  const endpoint = resolveEndpoint(preset, v.endpoint, v.region)
  if (!v.name.trim()) return { missing: 'name' }
  if (!v.bucket.trim()) return { missing: 'bucket' }
  if (preset.provider !== 'aws' && !endpoint) return { missing: 'endpoint' }
  if (!v.accessKeyId.trim()) return { missing: 'accessKeyId' }
  if (!v.secretAccessKey) return { missing: 'secretAccessKey' }
  const region = v.region.trim() || preset.region
  return {
    request: {
      name: v.name.trim(),
      provider: preset.provider,
      bucket: v.bucket.trim(),
      ...(region ? { region } : {}),
      ...(preset.provider !== 'aws' ? { endpoint } : {}),
      access_key_id: v.accessKeyId.trim(),
      secret_access_key: v.secretAccessKey,
    },
  }
}
