import { useTranslation } from 'react-i18next'
import { toast } from '@/components/ui/toast'
import {
  useCreateDashboard,
  useDeleteDashboard,
  useImportDashboard,
  useUpdateDashboard,
} from '@/queries/customDashboards'
import type { CustomDashboard, DashboardDoc } from '@/types/dashboards'

function downloadJson(name: string, doc: DashboardDoc) {
  const blob = new Blob([JSON.stringify(doc, null, 2)], {
    type: 'application/json',
  })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `${name.replace(/[^a-z0-9-]+/gi, '-').toLowerCase()}.json`
  a.click()
  URL.revokeObjectURL(url)
}

/** Create, import, rename, export and delete saved boards, with the toasts the old Dashboards page showed. */
export function useBoardActions(onSaved: (dashboardId: string) => void) {
  const { t } = useTranslation('dashboards')
  const create = useCreateDashboard()
  const update = useUpdateDashboard()
  const remove = useDeleteDashboard()
  const importer = useImportDashboard()

  const fail = (title: string, e: Error) => {
    toast.add({ title, description: e.message, type: 'error' })
  }

  return {
    busy: create.isPending || importer.isPending,
    renaming: update.isPending,
    deleting: remove.isPending,
    create(doc: DashboardDoc, done?: () => void) {
      create.mutate(
        { scope: 'user', doc },
        {
          onSuccess: (d) => {
            done?.()
            onSaved(d.id)
            toast.add({
              title: t('toast.created', { name: d.name }),
              type: 'success',
            })
          },
          onError: (e) => {
            fail(t('toast.createFailed'), e)
          },
        },
      )
    },
    async importFile(file: File) {
      try {
        const parsed = JSON.parse(await file.text()) as DashboardDoc
        importer.mutate(parsed, {
          onSuccess: (d) => {
            onSaved(d.id)
            toast.add({
              title: t('toast.imported', { name: d.name }),
              type: 'success',
            })
          },
          onError: (e) => {
            fail(t('toast.importFailed'), e)
          },
        })
      } catch {
        toast.add({
          title: t('toast.importFailed'),
          description: t('toast.notJson'),
          type: 'error',
        })
      }
    },
    rename(board: CustomDashboard, name: string, done: () => void) {
      update.mutate(
        {
          id: board.id,
          body: {
            scope: board.scope,
            project_id: board.project_id,
            is_default: board.is_default,
            doc: { ...board.doc, name },
          },
        },
        {
          onSuccess: () => {
            done()
            toast.add({ title: t('toast.saved'), type: 'success' })
          },
          onError: (e) => {
            fail(t('toast.saveFailed'), e)
          },
        },
      )
    },
    exportBoard(board: CustomDashboard) {
      downloadJson(board.name, board.doc)
    },
    delete(board: CustomDashboard, done: () => void) {
      remove.mutate(board.id, {
        onSuccess: () => {
          done()
          toast.add({ title: t('toast.deleted'), type: 'success' })
        },
        onError: (e) => {
          fail(t('toast.deleteFailed'), e)
        },
      })
    },
  }
}
