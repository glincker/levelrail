import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { databaseKeys } from './databases'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  MongoCollectionsResponse,
  MongoFindParams,
  MongoFindResponse,
} from '../types/databaseMongo'

const base = (name: string) =>
  `/api/v1/databases/${encodeURIComponent(name)}/mongo`

const keys = {
  all: (name: string) => [...databaseKeys.detail(name), 'mongo'] as const,
  collections: (name: string) => [...keys.all(name), 'collections'] as const,
  find: (name: string, p: MongoFindParams) =>
    [...keys.all(name), 'find', p] as const,
}

async function send<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(res.status, await readErrorMessage(res, 'mongo'))
  }
  return (await res.json()) as T
}

export function useMongoCollections(name: string) {
  return useQuery({
    queryKey: keys.collections(name),
    queryFn: () => send<MongoCollectionsResponse>(`${base(name)}/collections`),
    staleTime: 0,
  })
}

export function useMongoFind(name: string, p: MongoFindParams | null) {
  return useQuery({
    queryKey: keys.find(name, p ?? { database: '', collection: '', limit: 0 }),
    queryFn: () =>
      send<MongoFindResponse>(`${base(name)}/find`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(p),
      }),
    enabled: p !== null,
    staleTime: 0,
    retry: false,
    placeholderData: keepPreviousData,
  })
}
