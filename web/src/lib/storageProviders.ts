import type { StoragePreset } from '../types/storage'

// Logo ids resolve through TEMPLATE_LOGO_LOADERS (lib/templateLogos.ts).
const PRESET_LOGO: Record<StoragePreset, string | undefined> = {
  aws: 'aws-s3',
  r2: 'cloudflare',
  b2: 'backblaze',
  minio: 'minio',
  wasabi: 'wasabi',
  custom: undefined,
}

export function logoIdForStoragePreset(
  preset: StoragePreset,
): string | undefined {
  return PRESET_LOGO[preset]
}

export const STORAGE_PRESET_LABEL: Record<StoragePreset, string> = {
  aws: 'AWS S3',
  r2: 'Cloudflare R2',
  b2: 'Backblaze B2',
  minio: 'MinIO',
  wasabi: 'Wasabi',
  custom: 'S3 compatible',
}
