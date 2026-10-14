import { useNavigate, useSearch } from '@tanstack/react-router'
import {
  closeUrlDialogSearch,
  DEFAULT_DIALOG_PARAM,
  isUrlDialogOpen,
  openUrlDialogSearch,
  type SearchRecord,
} from '../../lib/urlDialog'

export interface UrlDialogControls {
  isOpen: boolean
  open: (extra?: Record<string, string>) => void
  close: () => void
}

// Drives open state from the URL, so a deep link opens the dialog on load
// and closing it rewrites the URL without the dialog's params.
export function useUrlDialog(
  value: string,
  param: string = DEFAULT_DIALOG_PARAM,
  ownedParams: readonly string[] = [],
): UrlDialogControls {
  const navigate = useNavigate()
  const search: SearchRecord = useSearch({ strict: false })
  const go = (next: SearchRecord) => {
    // The route's own validateSearch has already shaped what reaches this
    // point, the shared hook has no per-route search type to name.
    void navigate({ to: '.', search: next as never })
  }
  return {
    isOpen: isUrlDialogOpen(search, param, value),
    open: (extra) => go(openUrlDialogSearch(search, param, value, extra)),
    close: () => go(closeUrlDialogSearch(search, param, ownedParams)),
  }
}

// Reads and replaces one dialog-owned param, e.g. the chosen `method`.
export function useDialogParam(
  name: string,
): [string | undefined, (next: string) => void] {
  const navigate = useNavigate()
  const search: SearchRecord = useSearch({ strict: false })
  const raw = search[name]
  return [
    typeof raw === 'string' && raw !== '' ? raw : undefined,
    (next) =>
      void navigate({
        to: '.',
        search: { ...search, [name]: next } as never,
        replace: true,
      }),
  ]
}
