// POST /api/v1/databases/{name}/branches (internal/api/database_branch.go).
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { databaseKeys } from './databases'

export interface CreateDatabaseBranchRequest {
  name: string
  backup_id?: string
  at?: string
}

export interface DatabaseBranchRecord {
  id: string
  name: string
  branched_from: string
  backup_id?: string
  at?: string
  status: string
}

export async function createDatabaseBranch(
  source: string,
  req: CreateDatabaseBranchRequest,
): Promise<DatabaseBranchRecord> {
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(source)}/branches`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `branch database failed: ${res.status}`),
    )
  }
  return (await res.json()) as DatabaseBranchRecord
}

export function useCreateDatabaseBranch(source: string) {
  const queryClient = useQueryClient()
  return useMutation<
    DatabaseBranchRecord,
    ApiError,
    CreateDatabaseBranchRequest
  >({
    mutationFn: (req) => createDatabaseBranch(source, req),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: databaseKeys.list() })
    },
  })
}
