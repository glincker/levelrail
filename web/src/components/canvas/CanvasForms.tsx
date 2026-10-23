import { useId, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import type { ProjectTopologyGraph } from '../../types/projectTopology'
import {
  DRAFT_ENGINES,
  validateEnvVar,
  validateNewName,
  validatePort,
  type DraftEngine,
  type NameError,
} from '../../lib/canvasDraft'

const SELECT_CLASS =
  'h-8 w-full rounded-lg border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring dark:bg-input/30'

function FieldError({ id, message }: { id: string; message: string | null }) {
  if (!message) return null
  return (
    <p id={id} role="alert" className="text-xs text-destructive">
      {message}
    </p>
  )
}

export function AddAppDialog({
  open,
  graph,
  onOpenChange,
  onAdd,
}: {
  open: boolean
  graph: ProjectTopologyGraph
  onOpenChange: (open: boolean) => void
  onAdd: (name: string, image: string, port: number) => void
}) {
  const { t } = useTranslation('canvas')
  const uid = useId()
  const [name, setName] = useState('')
  const [image, setImage] = useState('')
  const [port, setPort] = useState('3000')
  const [submitted, setSubmitted] = useState(false)

  const nameErr: NameError | null = validateNewName(name, graph, 'app')
  const imageErr = image.trim() === '' ? 'required' : null
  const portNum = Number(port)
  const portOk = validatePort(portNum)

  function submit(e: FormEvent) {
    e.preventDefault()
    setSubmitted(true)
    if (nameErr || imageErr || !portOk) return
    onAdd(name.trim(), image.trim(), portNum)
    setName('')
    setImage('')
    setSubmitted(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={submit} noValidate className="space-y-4">
          <DialogHeader>
            <DialogTitle>{t('appForm.title')}</DialogTitle>
            <DialogDescription>{t('appForm.description')}</DialogDescription>
          </DialogHeader>
          <Field>
            <FieldLabel htmlFor={`${uid}-name`}>{t('appForm.name')}</FieldLabel>
            <Input
              id={`${uid}-name`}
              value={name}
              autoComplete="off"
              aria-invalid={submitted && nameErr !== null}
              aria-describedby={`${uid}-name-err`}
              onChange={(e) => setName(e.target.value)}
            />
            <FieldError
              id={`${uid}-name-err`}
              message={submitted && nameErr ? t(`errors.${nameErr}`) : null}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor={`${uid}-image`}>
              {t('appForm.image')}
            </FieldLabel>
            <Input
              id={`${uid}-image`}
              value={image}
              autoComplete="off"
              spellCheck={false}
              aria-invalid={submitted && imageErr !== null}
              aria-describedby={`${uid}-image-err`}
              onChange={(e) => setImage(e.target.value)}
            />
            <FieldDescription>{t('appForm.imageHint')}</FieldDescription>
            <FieldError
              id={`${uid}-image-err`}
              message={submitted && imageErr ? t('errors.required') : null}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor={`${uid}-port`}>{t('appForm.port')}</FieldLabel>
            <Input
              id={`${uid}-port`}
              inputMode="numeric"
              value={port}
              aria-invalid={submitted && !portOk}
              aria-describedby={`${uid}-port-err`}
              onChange={(e) => setPort(e.target.value)}
            />
            <FieldError
              id={`${uid}-port-err`}
              message={submitted && !portOk ? t('errors.port') : null}
            />
          </Field>
          <DialogFooter>
            <Button type="submit">{t('appForm.submit')}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export function AddDatabaseDialog({
  open,
  graph,
  onOpenChange,
  onAdd,
}: {
  open: boolean
  graph: ProjectTopologyGraph
  onOpenChange: (open: boolean) => void
  onAdd: (name: string, engine: DraftEngine, version: string) => void
}) {
  const { t } = useTranslation('canvas')
  const uid = useId()
  const [name, setName] = useState('')
  const [engine, setEngine] = useState<DraftEngine>('postgres')
  const [version, setVersion] = useState('')
  const [submitted, setSubmitted] = useState(false)
  const nameErr = validateNewName(name, graph, 'database')

  function submit(e: FormEvent) {
    e.preventDefault()
    setSubmitted(true)
    if (nameErr) return
    onAdd(name.trim(), engine, version.trim())
    setName('')
    setVersion('')
    setSubmitted(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={submit} noValidate className="space-y-4">
          <DialogHeader>
            <DialogTitle>{t('databaseForm.title')}</DialogTitle>
            <DialogDescription>
              {t('databaseForm.description')}
            </DialogDescription>
          </DialogHeader>
          <Field>
            <FieldLabel htmlFor={`${uid}-name`}>
              {t('databaseForm.name')}
            </FieldLabel>
            <Input
              id={`${uid}-name`}
              value={name}
              autoComplete="off"
              aria-invalid={submitted && nameErr !== null}
              aria-describedby={`${uid}-name-err`}
              onChange={(e) => setName(e.target.value)}
            />
            <FieldError
              id={`${uid}-name-err`}
              message={submitted && nameErr ? t(`errors.${nameErr}`) : null}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor={`${uid}-engine`}>
              {t('databaseForm.engine')}
            </FieldLabel>
            <select
              id={`${uid}-engine`}
              className={SELECT_CLASS}
              value={engine}
              onChange={(e) => setEngine(e.target.value as DraftEngine)}
            >
              {DRAFT_ENGINES.map((en) => (
                <option key={en} value={en}>
                  {en}
                </option>
              ))}
            </select>
          </Field>
          <Field>
            <FieldLabel htmlFor={`${uid}-version`}>
              {t('databaseForm.version')}
            </FieldLabel>
            <Input
              id={`${uid}-version`}
              value={version}
              autoComplete="off"
              onChange={(e) => setVersion(e.target.value)}
            />
            <FieldDescription>{t('databaseForm.versionHint')}</FieldDescription>
          </Field>
          <DialogFooter>
            <Button type="submit">{t('databaseForm.submit')}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export function ConnectForm({
  apps,
  databases,
  onConnect,
}: {
  apps: readonly string[]
  databases: readonly string[]
  onConnect: (app: string, database: string, envVar: string) => void
}) {
  const { t } = useTranslation('canvas')
  const uid = useId()
  const [app, setApp] = useState('')
  const [database, setDatabase] = useState('')
  const [envVar, setEnvVar] = useState('')
  const envErr = !validateEnvVar(envVar.trim())

  if (apps.length === 0 || databases.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">{t('connect.needBoth')}</p>
    )
  }

  function submit(e: FormEvent) {
    e.preventDefault()
    if (!app || !database || envErr) return
    onConnect(app, database, envVar.trim())
    setEnvVar('')
  }

  return (
    <form onSubmit={submit} noValidate className="space-y-3">
      <Field>
        <FieldLabel htmlFor={`${uid}-app`}>{t('connect.app')}</FieldLabel>
        <select
          id={`${uid}-app`}
          className={SELECT_CLASS}
          value={app}
          onChange={(e) => setApp(e.target.value)}
        >
          <option value="">{t('connect.choose')}</option>
          {apps.map((a) => (
            <option key={a} value={a}>
              {a}
            </option>
          ))}
        </select>
      </Field>
      <Field>
        <FieldLabel htmlFor={`${uid}-db`}>{t('connect.database')}</FieldLabel>
        <select
          id={`${uid}-db`}
          className={SELECT_CLASS}
          value={database}
          onChange={(e) => setDatabase(e.target.value)}
        >
          <option value="">{t('connect.choose')}</option>
          {databases.map((d) => (
            <option key={d} value={d}>
              {d}
            </option>
          ))}
        </select>
      </Field>
      <Field>
        <FieldLabel htmlFor={`${uid}-env`}>{t('connect.envVar')}</FieldLabel>
        <Input
          id={`${uid}-env`}
          value={envVar}
          autoComplete="off"
          spellCheck={false}
          aria-invalid={envErr}
          aria-describedby={`${uid}-env-err`}
          onChange={(e) => setEnvVar(e.target.value)}
        />
        <FieldDescription>{t('connect.envVarHint')}</FieldDescription>
        <FieldError
          id={`${uid}-env-err`}
          message={envErr ? t('errors.envVar') : null}
        />
      </Field>
      <Button
        type="submit"
        size="sm"
        variant="outline"
        disabled={!app || !database || envErr}
      >
        {t('connect.submit')}
      </Button>
    </form>
  )
}
