// GET /api/v1/backup-targets/field-descriptors (handleGetBackupTargetFields).
import { queryOptions } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { FieldDescriptorSet } from '../types/fieldDescriptor'

export const fieldDescriptorKeys = {
  resource: (name: string) => ['field-descriptors', name] as const,
  backupTarget: ['field-descriptors', 'backup-targets'] as readonly [
    'field-descriptors',
    string,
  ],
  logDrain: ['field-descriptors', 'log-drains'] as readonly [
    'field-descriptors',
    string,
  ],
}

async function fetchFields(url: string): Promise<FieldDescriptorSet> {
  const res = await fetch(url)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `load fields failed: ${res.status}`),
    )
  }
  return (await res.json()) as FieldDescriptorSet
}

export const fetchBackupTargetFields = () =>
  fetchFields('/api/v1/backup-targets/field-descriptors')

export function logDrainFieldsQueryOptions() {
  return queryOptions({
    queryKey: fieldDescriptorKeys.logDrain,
    queryFn: () => fetchFields('/api/v1/log-drains/field-descriptors'),
    staleTime: Infinity,
  })
}

export function backupTargetFieldsQueryOptions() {
  return queryOptions({
    queryKey: fieldDescriptorKeys.backupTarget,
    queryFn: fetchBackupTargetFields,
    staleTime: Infinity,
  })
}

// Any registered resource is served by the one generic route, so a new descriptor file
// needs no query of its own.
export function fieldDescriptorsQueryOptions(resource: string) {
  return queryOptions({
    queryKey: fieldDescriptorKeys.resource(resource),
    queryFn: () =>
      fetchFields(`/api/v1/field-descriptors/${encodeURIComponent(resource)}`),
    staleTime: Infinity,
  })
}

// Resource names as registered in internal/fielddesc.
export const FIELD_RESOURCES = {
  notificationChannel: 'notification-channels',
  dnsProvider: 'dns-providers',
  registryCredential: 'registry-credentials',
} as const
