import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import {
  useAccessSearch,
  type AccessSearchFilters,
} from '../../queries/accessLogSearch'

const EMPTY: AccessSearchFilters = {
  status: '',
  method: '',
  path: '',
  q: '',
  since: '1h',
  cursor: '',
}

function statusTone(code: number): string {
  if (code >= 500) return 'text-destructive'
  if (code >= 400) return 'text-warning'
  return 'text-muted-foreground'
}

// Recent edge requests for one app. Paths carry no query string because the
// edge never records one, and client IPs are never part of the response.
export default function AccessLogSearchPanel({ appName }: { appName: string }) {
  const { t } = useTranslation('observability')
  const [draft, setDraft] = useState<AccessSearchFilters>(EMPTY)
  const [applied, setApplied] = useState<AccessSearchFilters>(EMPTY)
  const [cursors, setCursors] = useState<string[]>([])
  const { data, error, isPending } = useAccessSearch(appName, applied)

  const submit = (e: FormEvent) => {
    e.preventDefault()
    setCursors([])
    setApplied({ ...draft, cursor: '' })
  }
  const set = (key: keyof AccessSearchFilters) => (value: string) =>
    setDraft((prev) => ({ ...prev, [key]: value }))

  return (
    <div className="flex flex-col gap-3">
      <form
        onSubmit={submit}
        className="flex flex-wrap items-end gap-2"
        aria-label={t('accessSearch.formLabel')}
      >
        <NativeSelect
          aria-label={t('accessSearch.status')}
          value={draft.status}
          onChange={(e) => set('status')(e.target.value)}
          className="w-28"
        >
          <option value="">{t('accessSearch.anyStatus')}</option>
          <option value="2xx">2xx</option>
          <option value="3xx">3xx</option>
          <option value="4xx">4xx</option>
          <option value="5xx">5xx</option>
        </NativeSelect>
        <NativeSelect
          aria-label={t('accessSearch.method')}
          value={draft.method}
          onChange={(e) => set('method')(e.target.value)}
          className="w-28"
        >
          <option value="">{t('accessSearch.anyMethod')}</option>
          {['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS'].map(
            (m) => (
              <option key={m} value={m}>
                {m}
              </option>
            ),
          )}
        </NativeSelect>
        <Input
          aria-label={t('accessSearch.path')}
          placeholder={t('accessSearch.pathPlaceholder')}
          value={draft.path}
          onChange={(e) => set('path')(e.target.value)}
          className="w-40"
        />
        <Input
          aria-label={t('accessSearch.text')}
          placeholder={t('accessSearch.textPlaceholder')}
          value={draft.q}
          onChange={(e) => set('q')(e.target.value)}
          className="w-44"
        />
        <NativeSelect
          aria-label={t('accessSearch.window')}
          value={draft.since}
          onChange={(e) => set('since')(e.target.value)}
          className="w-28"
        >
          {['15m', '1h', '6h', '24h'].map((w) => (
            <option key={w} value={w}>
              {w}
            </option>
          ))}
        </NativeSelect>
        <Button type="submit" size="sm">
          {t('accessSearch.search')}
        </Button>
      </form>
      <p className="text-xs text-muted-foreground">
        {t('accessSearch.privacyNote')}
      </p>
      {error ? (
        <p role="alert" className="text-sm text-destructive">
          {error.message}
        </p>
      ) : null}
      {isPending ? (
        <p className="text-sm text-muted-foreground">
          {t('accessSearch.loading')}
        </p>
      ) : null}
      {data && data.records.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('accessSearch.empty')}
        </p>
      ) : null}
      {data && data.records.length > 0 ? (
        <div className="overflow-x-auto rounded-xl border border-border">
          <table className="w-full text-sm">
            <thead className="text-left text-muted-foreground">
              <tr>
                <th className="px-3 py-2 font-medium">
                  {t('accessSearch.colTime')}
                </th>
                <th className="px-3 py-2 font-medium">
                  {t('accessSearch.colStatus')}
                </th>
                <th className="px-3 py-2 font-medium">
                  {t('accessSearch.colMethod')}
                </th>
                <th className="px-3 py-2 font-medium">
                  {t('accessSearch.colPath')}
                </th>
                <th className="px-3 py-2 text-right font-medium">
                  {t('accessSearch.colDuration')}
                </th>
              </tr>
            </thead>
            <tbody>
              {data.records.map((r) => (
                <tr
                  key={`${r.time}-${r.request_id}`}
                  className="border-t border-border"
                >
                  <td className="whitespace-nowrap px-3 py-1.5 tabular-nums">
                    {new Date(r.time).toLocaleTimeString()}
                  </td>
                  <td
                    className={`px-3 py-1.5 tabular-nums ${statusTone(r.status)}`}
                  >
                    {r.status}
                  </td>
                  <td className="px-3 py-1.5">{r.method}</td>
                  <td className="max-w-md truncate px-3 py-1.5 font-mono">
                    {r.host}
                    {r.path}
                  </td>
                  <td className="px-3 py-1.5 text-right tabular-nums">
                    {Math.round(r.duration_ms)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
      {data?.truncated ? (
        <p className="text-xs text-muted-foreground">
          {t('accessSearch.truncated')}
        </p>
      ) : null}
      <div className="flex gap-2">
        {cursors.length > 0 ? (
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              const prev = cursors.slice(0, -1)
              setCursors(prev)
              setApplied((a) => ({ ...a, cursor: prev.at(-1) ?? '' }))
            }}
          >
            {t('accessSearch.newer')}
          </Button>
        ) : null}
        {data?.next_cursor ? (
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              const next = data.next_cursor ?? ''
              setCursors((c) => [...c, next])
              setApplied((a) => ({ ...a, cursor: next }))
            }}
          >
            {t('accessSearch.older')}
          </Button>
        ) : null}
      </div>
    </div>
  )
}
