import {
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import { cva, type VariantProps } from 'class-variance-authority'
import { ArrowSquareOutIcon } from '@phosphor-icons/react/dist/ssr'

import { cn } from '@/lib/utils'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import {
  FieldContext,
  useFieldContext,
  type FieldContextValue,
} from '@/components/ui/field-context'

function FieldSet({ className, ...props }: React.ComponentProps<'fieldset'>) {
  return (
    <fieldset
      data-slot="field-set"
      className={cn(
        'flex flex-col gap-4 has-[>[data-slot=checkbox-group]]:gap-3 has-[>[data-slot=radio-group]]:gap-3',
        className,
      )}
      {...props}
    />
  )
}

function FieldLegend({
  className,
  variant = 'legend',
  ...props
}: React.ComponentProps<'legend'> & { variant?: 'legend' | 'label' }) {
  return (
    <legend
      data-slot="field-legend"
      data-variant={variant}
      className={cn(
        'mb-1.5 font-medium data-[variant=label]:text-sm data-[variant=legend]:text-base',
        className,
      )}
      {...props}
    />
  )
}

function FieldGroup({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="field-group"
      className={cn(
        'group/field-group @container/field-group flex w-full flex-col gap-5 data-[slot=checkbox-group]:gap-3 *:data-[slot=field-group]:gap-4',
        className,
      )}
      {...props}
    />
  )
}

const fieldVariants = cva(
  'group/field flex w-full gap-2 data-[invalid=true]:text-destructive',
  {
    variants: {
      orientation: {
        vertical: 'flex-col *:w-full [&>.sr-only]:w-auto',
        horizontal:
          'flex-row items-center has-[>[data-slot=field-content]]:items-start *:data-[slot=field-label]:flex-auto has-[>[data-slot=field-content]]:[&>[role=checkbox],[role=radio]]:mt-px',
        responsive:
          'flex-col *:w-full @md/field-group:flex-row @md/field-group:items-center @md/field-group:*:w-auto @md/field-group:has-[>[data-slot=field-content]]:items-start @md/field-group:*:data-[slot=field-label]:flex-auto [&>.sr-only]:w-auto @md/field-group:has-[>[data-slot=field-content]]:[&>[role=checkbox],[role=radio]]:mt-px',
      },
    },
    defaultVariants: {
      orientation: 'vertical',
    },
  },
)

function Field({
  className,
  orientation = 'vertical',
  required = false,
  children,
  ...props
}: React.ComponentProps<'div'> &
  VariantProps<typeof fieldVariants> & { required?: boolean }) {
  const generatedId = useId()
  const [owners, setOwners] = useState<
    { uid: string; explicitId: string | undefined }[]
  >([])
  const [labelFor, setLabelFor] = useState<string | undefined>()
  const [hints, setHints] = useState<string[]>([])
  const [errors, setErrors] = useState<string[]>([])

  const claimControl = useCallback(
    (uid: string, explicitId: string | undefined) => {
      setOwners((list) => [
        ...list.filter((o) => o.uid !== uid),
        { uid, explicitId },
      ])
      return () => {
        setOwners((list) => list.filter((o) => o.uid !== uid))
      }
    },
    [],
  )
  const registerDescription = useCallback(
    (id: string, kind: 'hint' | 'error') => {
      const set = kind === 'hint' ? setHints : setErrors
      set((list) => (list.includes(id) ? list : [...list, id]))
      return () => {
        set((list) => list.filter((x) => x !== id))
      }
    },
    [],
  )

  const owner = owners[0]
  const dataInvalid: unknown = (props as { 'data-invalid'?: unknown })[
    'data-invalid'
  ]
  const invalid =
    errors.length > 0 || dataInvalid === true || dataInvalid === 'true'
  const describedBy = [...hints, ...errors].join(' ') || undefined
  const ctx = useMemo<FieldContextValue>(
    () => ({
      invalid,
      required,
      controlId: owner
        ? (owner.explicitId ?? labelFor ?? generatedId)
        : undefined,
      describedBy,
      claimControl,
      setLabelFor,
      registerDescription,
      ownerUid: owner?.uid ?? null,
    }),
    [
      invalid,
      required,
      owner,
      labelFor,
      generatedId,
      describedBy,
      claimControl,
      registerDescription,
    ],
  )

  return (
    <FieldContext.Provider value={ctx}>
      <div
        role="group"
        data-slot="field"
        data-orientation={orientation}
        className={cn(fieldVariants({ orientation }), className)}
        {...props}
      >
        {children}
      </div>
    </FieldContext.Provider>
  )
}

function FieldContent({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="field-content"
      className={cn(
        'group/field-content flex flex-1 flex-col gap-0.5 leading-snug',
        className,
      )}
      {...props}
    />
  )
}

function FieldLabel({
  className,
  htmlFor,
  ...props
}: React.ComponentProps<typeof Label>) {
  const ctx = useFieldContext()
  const setLabelFor = ctx?.setLabelFor
  useLayoutEffect(() => {
    if (!htmlFor) return undefined
    setLabelFor?.(htmlFor)
    return () => {
      setLabelFor?.(undefined)
    }
  }, [htmlFor, setLabelFor])
  // Only link to a control that exists; a dangling for= would break the
  // implicit link of a label that wraps its input.
  return (
    <Label
      htmlFor={htmlFor ?? ctx?.controlId}
      data-slot="field-label"
      className={cn(
        'group/field-label peer/field-label flex w-fit gap-2 leading-snug group-data-[disabled=true]/field:opacity-50 has-data-checked:border-primary/30 has-data-checked:bg-primary/5 has-[>[data-slot=field]]:rounded-lg has-[>[data-slot=field]]:border *:data-[slot=field]:p-2.5 dark:has-data-checked:border-primary/20 dark:has-data-checked:bg-primary/10',
        'has-[>[data-slot=field]]:w-full has-[>[data-slot=field]]:flex-col',
        className,
      )}
      {...props}
    />
  )
}

function FieldTitle({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="field-label"
      className={cn(
        'flex w-fit items-center gap-2 text-sm font-medium group-data-[disabled=true]/field:opacity-50',
        className,
      )}
      {...props}
    />
  )
}

function FieldDescription({
  className,
  id,
  ...props
}: React.ComponentProps<'p'>) {
  const generatedId = useId()
  const descId = id ?? generatedId
  const register = useFieldContext()?.registerDescription
  useLayoutEffect(() => register?.(descId, 'hint'), [register, descId])
  return (
    <p
      id={descId}
      data-slot="field-description"
      className={cn(
        'text-left text-sm leading-normal font-normal text-muted-foreground group-has-data-horizontal/field:text-balance [[data-variant=legend]+&]:-mt-1.5',
        'last:mt-0 nth-last-2:-mt-1',
        '[&>a]:underline [&>a]:underline-offset-4 [&>a:hover]:text-primary',
        className,
      )}
      {...props}
    />
  )
}

// A FieldDescription that can additionally carry a link to somewhere a
// first-time user finds the value being asked for (a GitHub token
// scope, a DNS record type) or the tradeoff between the choices offered
// (an engine picker). Extends FieldDescription's own markup rather than
// introducing a second, parallel description primitive: same tag, same
// muted-text styling, same `[&>a]` underline treatment, just with an
// external-link affordance appended when `href` is given. Renders
// exactly like FieldDescription when `href` is omitted.
function FieldHint({
  children,
  href,
  linkText = 'Learn more',
  className,
  ...props
}: React.ComponentProps<'p'> & {
  children?: ReactNode
  href?: string
  linkText?: string
}) {
  return (
    <FieldDescription className={className} {...props}>
      {children}
      {href ? (
        <>
          {children ? ' ' : null}
          <a
            href={href}
            target="_blank"
            rel="noreferrer noopener"
            className="inline-flex items-center gap-0.5"
          >
            {linkText}
            <ArrowSquareOutIcon className="size-3" aria-hidden="true" />
          </a>
        </>
      ) : null}
    </FieldDescription>
  )
}

// A small uppercase subheading for grouping related fields in a longer
// form (e.g. "Source", "Networking", "Health check"), matching the
// tracking-wide muted style CreateResourceWizard.tsx already uses for its
// own step-1 "Applications"/"Databases" groupings.
function FieldSectionLabel({
  className,
  ...props
}: React.ComponentProps<'h3'>) {
  return (
    <h3
      data-slot="field-section-label"
      className={cn(
        'text-xs font-medium tracking-wide text-muted-foreground uppercase',
        className,
      )}
      {...props}
    />
  )
}

function FieldSeparator({
  children,
  className,
  ...props
}: React.ComponentProps<'div'> & {
  children?: React.ReactNode
}) {
  return (
    <div
      data-slot="field-separator"
      data-content={!!children}
      className={cn(
        'relative -my-2 h-5 text-sm group-data-[variant=outline]/field-group:-mb-2',
        className,
      )}
      {...props}
    >
      <Separator className="absolute inset-0 top-1/2" />
      {children && (
        <span
          className="relative mx-auto block w-fit bg-background px-2 text-muted-foreground"
          data-slot="field-separator-content"
        >
          {children}
        </span>
      )}
    </div>
  )
}

function FieldError({
  className,
  children,
  errors,
  id,
  ...props
}: React.ComponentProps<'div'> & {
  errors?: Array<{ message?: string } | undefined>
}) {
  const generatedId = useId()
  const errorId = id ?? generatedId
  const register = useFieldContext()?.registerDescription
  // role=alert only for the first moment so later re-renders and a form
  // summary do not make a screen reader repeat the message.
  const [announce, setAnnounce] = useState(true)
  useEffect(() => {
    const timer = setTimeout(() => {
      setAnnounce(false)
    }, 1000)
    return () => {
      clearTimeout(timer)
    }
  }, [])
  const content = useMemo(() => {
    if (children) {
      return children
    }

    if (!errors?.length) {
      return null
    }

    const uniqueErrors = [
      ...new Map(errors.map((error) => [error?.message, error])).values(),
    ]

    if (uniqueErrors?.length == 1) {
      return uniqueErrors[0]?.message
    }

    return (
      <ul className="ml-4 flex list-disc flex-col gap-1">
        {uniqueErrors.map(
          (error, index) =>
            error?.message && <li key={index}>{error.message}</li>,
        )}
      </ul>
    )
  }, [children, errors])

  const hasContent = Boolean(content)
  useLayoutEffect(
    () => (hasContent ? register?.(errorId, 'error') : undefined),
    [register, errorId, hasContent],
  )

  if (!content) {
    return null
  }

  return (
    <div
      id={errorId}
      role={announce ? 'alert' : undefined}
      data-slot="field-error"
      className={cn('text-sm font-normal text-destructive', className)}
      {...props}
    >
      {content}
    </div>
  )
}

export {
  Field,
  FieldLabel,
  FieldDescription,
  FieldHint,
  FieldError,
  FieldGroup,
  FieldLegend,
  FieldSectionLabel,
  FieldSeparator,
  FieldSet,
  FieldContent,
  FieldTitle,
}
