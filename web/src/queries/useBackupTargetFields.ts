import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  backupTargetFieldsQueryOptions,
  logDrainFieldsQueryOptions,
} from './fieldDescriptors'
import { isFieldVisible } from '../types/fieldDescriptor'

/**
 * useBackupTargetFields serves labels, help and visibility from the field registry.
 * Until the registry loads (or if it fails) every call falls back to the caller's text,
 * so a form never renders blank.
 */
export function useBackupTargetFields() {
  return useDescriptorFields(backupTargetFieldsQueryOptions(), 'backups')
}

/** useLogDrainFields is the same lookup for the app log drain form. */
export function useLogDrainFields() {
  return useDescriptorFields(logDrainFieldsQueryOptions(), 'integrations')
}

function useDescriptorFields(
  options: ReturnType<typeof backupTargetFieldsQueryOptions>,
  ns: 'backups' | 'integrations',
) {
  const { t: tKey } = useTranslation(ns)
  // Keys come from the server registry, so they cannot be checked at compile time.
  const t = tKey as (key: string) => string
  const { data } = useQuery({ ...options, retry: false })
  const byId = new Map((data?.fields ?? []).map((f) => [f.id, f]))
  return {
    label: (id: string, fallback: string): string => {
      const f = byId.get(id)
      return f ? t(f.label_key) : fallback
    },
    help: (id: string, fallback: string): string => {
      const f = byId.get(id)
      return f ? t(f.help_key) : fallback
    },
    visible: (
      id: string,
      values: Record<string, string>,
      fallback: boolean,
    ): boolean => {
      const f = byId.get(id)
      return f ? isFieldVisible(f, values) : fallback
    },
  }
}
