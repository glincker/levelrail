import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  PlusGlassIcon,
  SettingsGlassIcon,
  UniversityGlassIcon,
} from '@thesvg/ui-react'
import type { SAMLConnection } from '@/queries/sso'
import { fetchSAMLTestResult } from '@/queries/sso'
import type { SSOTestResult } from '@/queries/ssoOidc'
import type { RoleResource } from '@/queries/roles'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { FormSection } from '@/components/ui/form-section'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { SAMLConnectionForm } from './SAMLConnectionForm'
import { SAMLTestResult } from './SAMLTestResult'

const TEST_PARAM = 'sso_test'
const TEST_PREFIX = 'saml:'

// A test sign-in ends on this page with saml:<id> in the query string.
function returnedConnectionID(): string | null {
  try {
    const v = new URLSearchParams(window.location.search).get(TEST_PARAM)
    return v?.startsWith(TEST_PREFIX) ? v.slice(TEST_PREFIX.length) : null
  } catch {
    return null
  }
}

function clearReturnedTest() {
  const url = new URL(window.location.href)
  url.searchParams.delete(TEST_PARAM)
  window.history.replaceState(null, '', url)
}

type Editing = { kind: 'new' } | { kind: 'edit'; connection: SAMLConnection }

function stateOf(c: SAMLConnection): 'enabled' | 'disabled' | 'incomplete' {
  if (!c.configured) return 'incomplete'
  return c.enabled ? 'enabled' : 'disabled'
}

const STATE_BADGE = {
  enabled: 'success',
  disabled: 'muted',
  incomplete: 'outline',
} as const

/** SAMLConnectionList lists connections and opens each one's form in a sheet. */
export function SAMLConnectionList({
  connections,
  roles,
}: {
  connections: SAMLConnection[]
  roles: RoleResource[]
}) {
  const { t, i18n } = useTranslation('sso')
  const [returnedID] = useState(returnedConnectionID)
  const [editing, setEditing] = useState<Editing | null>(() => {
    const back = connections.find((c) => c.id === returnedID)
    return back ? { kind: 'edit', connection: back } : null
  })
  const [result, setResult] = useState<SSOTestResult | null>(null)

  useEffect(() => {
    if (!returnedID) return
    let live = true
    fetchSAMLTestResult(returnedID)
      .then((r) => {
        if (live && r.status !== 'none') setResult(r)
      })
      .catch(() => {
        if (live)
          setResult({ status: 'failed', failure_code: 'response_invalid' })
      })
    return () => {
      live = false
    }
    // One-shot result: read once for the connection the page returned with.
  }, [returnedID])
  const current =
    editing?.kind === 'edit'
      ? (connections.find((c) => c.id === editing.connection.id) ??
        editing.connection)
      : undefined
  const dateFmt = new Intl.DateTimeFormat(i18n.language, {
    dateStyle: 'medium',
  })

  const addButton = (
    <Button type="button" onClick={() => setEditing({ kind: 'new' })}>
      <PlusGlassIcon size="1.4em" aria-hidden="true" />
      {t('page.addConnection')}
    </Button>
  )

  return (
    <FormSection title={t('list.title')} description={t('list.description')}>
      {connections.length === 0 ? (
        <EmptyState
          icon={<UniversityGlassIcon className="size-6" />}
          title={t('list.emptyTitle')}
          description={t('list.emptyBody')}
          action={addButton}
          compact
        />
      ) : (
        <>
          <ul className="divide-y divide-border rounded-lg border border-border bg-card px-4">
            {connections.map((c) => {
              const state = stateOf(c)
              const soonest = c.certificates
                .map((x) => new Date(x.not_after))
                .sort((a, b) => a.getTime() - b.getTime())[0]
              return (
                <li
                  key={c.id}
                  className="flex flex-wrap items-center gap-x-3 gap-y-2 py-3"
                >
                  <span className="min-w-0 flex-1 truncate text-sm font-medium">
                    {c.name}
                  </span>
                  <Badge variant={STATE_BADGE[state]}>
                    {t(`list.state.${state}`)}
                  </Badge>
                  {c.jit_provisioning ? (
                    <Badge variant="outline">{t('list.jit')}</Badge>
                  ) : null}
                  {soonest ? (
                    <span className="text-xs text-muted-foreground">
                      {t('list.certExpires', {
                        date: dateFmt.format(soonest),
                      })}
                    </span>
                  ) : null}
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => setEditing({ kind: 'edit', connection: c })}
                  >
                    <SettingsGlassIcon size="1.4em" aria-hidden="true" />
                    {t('list.configure')}
                  </Button>
                </li>
              )
            })}
          </ul>
          <div>{addButton}</div>
        </>
      )}
      <Sheet
        open={editing !== null}
        onOpenChange={(open) => {
          if (!open) {
            setEditing(null)
            setResult(null)
            clearReturnedTest()
          }
        }}
      >
        <SheetContent className="data-[side=right]:sm:max-w-xl">
          <SheetHeader className="border-b border-border pr-12">
            <SheetTitle>
              {current
                ? t('form.editTitle', { name: current.name })
                : t('form.newTitle')}
            </SheetTitle>
            <SheetDescription>{t('form.sheetDescription')}</SheetDescription>
          </SheetHeader>
          <div className="min-h-0 flex-1 space-y-6 overflow-y-auto px-4 pb-4">
            {current && result ? (
              <SAMLTestResult
                result={result}
                connection={current}
                roles={roles}
              />
            ) : null}
            {editing ? (
              <SAMLConnectionForm
                key={current?.id ?? 'new'}
                connection={current}
                roles={roles}
                onSaved={(c) => setEditing({ kind: 'edit', connection: c })}
                onDeleted={() => setEditing(null)}
              />
            ) : null}
          </div>
        </SheetContent>
      </Sheet>
    </FormSection>
  )
}
