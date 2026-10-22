// Migration job extras on top of the app import session (internal/api/migration_guide*.go):
// the source connection test, live progress, per-domain probes and the safe-to-delete gate.
// The source credential only travels in the test request body, never the query cache.

import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { appImportKeys, type AppImportSource } from './appImport'

const base = '/api/v1/migration/apps'
const enc = encodeURIComponent

export type TestFailure = 'credentials' | 'blocked' | 'unreachable'

export interface MigrationSourceTest {
  ok: boolean
  platform: string
  failure?: TestFailure
  message?: string
  apps: number
  databases: number
  unsupported: number
  servers: string[]
  volumes: number
  domains: number
  crons: number
  warnings: string[]
}

export interface MigrationJobItem {
  source_id: string
  name: string
  target?: string
  state: string
  reason?: string
  retryable: boolean
  volumes_pending?: number
}

export interface MigrationJobDatabase {
  name: string
  status: string
  mismatched?: number
}

export interface MigrationJob {
  id: string
  platform: string
  step: string
  phase: 'empty' | 'working' | 'attention' | 'ready'
  running: boolean
  connected: boolean
  total: number
  done: number
  failed: number
  states: Record<string, number>
  items: MigrationJobItem[]
  databases: MigrationJobDatabase[]
  next: string[]
  revision: string
}

export type GateStatus = 'pass' | 'fail' | 'pending' | 'manual'

export interface RetireGate {
  id: string
  status: GateStatus
  detail: string
  open?: string[]
  fix?: string
}

export interface RetireReport {
  safe_to_delete: boolean
  gates: RetireGate[]
  fallback: string[]
}

export interface ProbeCheck {
  id: string
  status: 'pass' | 'warn' | 'fail'
  detail: string
  fix?: string
}

export interface DomainProbe {
  domain: string
  app: string
  targets: string[]
  dns?: ProbeCheck
  http?: ProbeCheck[]
  ok: boolean
}

async function send<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `request failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

function post(body: unknown): RequestInit {
  return {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }
}

export const migrationJobKeys = {
  retire: (id: string) => ['migration', 'apps', 'retire', id] as const,
}

export function useMigrationSourceTest() {
  return useMutation<MigrationSourceTest, ApiError, AppImportSource>({
    mutationFn: (source) =>
      send<MigrationSourceTest>(`${base}/test`, post(source)),
  })
}

// Follows the server-sent progress of a job while active. A refetch of the session
// rides on every event, so the rest of the wizard stays current without polling.
export function useMigrationJobEvents(id: string, active: boolean) {
  const qc = useQueryClient()
  const [job, setJob] = useState<MigrationJob | null>(null)
  useEffect(() => {
    if (!active || typeof EventSource === 'undefined') return
    const source = new EventSource(`${base}/sessions/${enc(id)}/events`)
    source.onmessage = (e: MessageEvent<string>) => {
      try {
        setJob(JSON.parse(e.data) as MigrationJob)
        void qc.invalidateQueries({ queryKey: appImportKeys.one(id) })
      } catch {
        // A malformed frame is skipped; the next one replaces it.
      }
    }
    return () => source.close()
  }, [id, active, qc])
  return job
}

export function useProbeDomain() {
  return useMutation<
    DomainProbe,
    ApiError,
    { id: string; domain: string; kind?: 'dns' | 'http'; targetIp?: string }
  >({
    mutationFn: ({ id, domain, kind, targetIp }) =>
      send<DomainProbe>(
        `${base}/sessions/${enc(id)}/probe${
          targetIp ? `?target_ip=${enc(targetIp)}` : ''
        }`,
        post({ domain, kind }),
      ),
  })
}

export function useRetireReport(id: string, acknowledged: string[]) {
  return useQuery<RetireReport, ApiError>({
    queryKey: migrationJobKeys.retire(id),
    queryFn: () =>
      acknowledged.length > 0
        ? send<RetireReport>(
            `${base}/sessions/${enc(id)}/retire`,
            post({ probe: false, acknowledged }),
          )
        : send<RetireReport>(`${base}/sessions/${enc(id)}/retire`),
  })
}

export function useRunRetire() {
  const qc = useQueryClient()
  return useMutation<
    RetireReport,
    ApiError,
    {
      id: string
      probe: boolean
      acknowledged: string[]
      targetIp?: string
    }
  >({
    mutationFn: ({ id, probe, acknowledged, targetIp }) =>
      send<RetireReport>(
        `${base}/sessions/${enc(id)}/retire${
          targetIp ? `?target_ip=${enc(targetIp)}` : ''
        }`,
        post({ probe, acknowledged }),
      ),
    onSuccess: (report, v) => {
      qc.setQueryData(migrationJobKeys.retire(v.id), report)
    },
  })
}
