import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { databaseKeys } from './databases'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  DynamoItemsResponse,
  DynamoReadParams,
  DynamoTablesResponse,
} from '../types/databaseDynamo'

const base = (name: string) =>
  `/api/v1/databases/${encodeURIComponent(name)}/dynamo`

const keys = {
  all: (name: string) => [...databaseKeys.detail(name), 'dynamo'] as const,
  tables: (name: string) => [...keys.all(name), 'tables'] as const,
  read: (name: string, p: DynamoReadParams) =>
    [...keys.all(name), 'read', p] as const,
}

async function send<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(res.status, await readErrorMessage(res, 'dynamodb'))
  }
  return (await res.json()) as T
}

export function useDynamoTables(name: string) {
  return useQuery({
    queryKey: keys.tables(name),
    queryFn: () => send<DynamoTablesResponse>(`${base(name)}/tables`),
    staleTime: 0,
  })
}

export function useDynamoRead(name: string, p: DynamoReadParams | null) {
  return useQuery({
    queryKey: p ? keys.read(name, p) : [...keys.all(name), 'read', 'idle'],
    queryFn: () => {
      const { op, ...body } = p as DynamoReadParams
      return send<DynamoItemsResponse>(`${base(name)}/${op}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
    },
    enabled: p !== null,
    staleTime: 0,
    retry: false,
    placeholderData: keepPreviousData,
  })
}
