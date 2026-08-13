import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { useEffect, useState } from 'react'
import { RateLimitError, useLogin } from '../queries/auth'
import { Button } from './ui/button'
import { Input } from './ui/input'
import { Field, FieldError, FieldGroup, FieldLabel } from './ui/field'
import { Alert, AlertDescription, AlertTitle } from './ui/alert'

const loginSchema = z.object({
  username: z.string().trim().min(1, 'Username is required'),
  password: z.string().min(1, 'Password is required'),
})

type LoginFormValues = z.infer<typeof loginSchema>

// internal/api/auth.go's handleLogin deliberately returns the same
// generic "invalid credentials" message whether the username is unknown
// or the password is wrong, so this form never tries to guess which one
// happened, it just surfaces the server's message as-is. The one status
// code that gets different treatment is 429: ratelimit.go's real
// exponential backoff means the operator needs a concrete "try again in
// Ns" countdown, not a message indistinguishable from a wrong password.
export function LoginForm() {
  const login = useLogin()
  const { register, handleSubmit, formState } = useForm<LoginFormValues>({
    resolver: zodResolver(loginSchema),
    defaultValues: { username: '', password: '' },
  })

  // A plain decrementing counter, not a target-timestamp computed against
  // Date.now() at render time: React's rules-of-hooks purity lint flags
  // any impure call (Date.now included) inside render. Ticking down once
  // a second via setInterval is a close enough approximation for a UI
  // countdown, this isn't a scheduling primitive that needs wall-clock
  // precision.
  const [secondsRemaining, setSecondsRemaining] = useState(0)
  const isRateLimited = secondsRemaining > 0

  useEffect(() => {
    if (!isRateLimited) {
      return
    }
    const id = window.setInterval(() => {
      setSecondsRemaining((s) => Math.max(0, s - 1))
    }, 1000)
    return () => {
      window.clearInterval(id)
    }
    // Depending on isRateLimited (not secondsRemaining itself) means this
    // effect only re-runs when the countdown starts or reaches zero, not
    // once per tick: the interval callback above updates state via its
    // own functional setState, exactly the "subscribe once, setState in a
    // callback" shape React's effect rules ask for, not a setState call
    // in the effect body itself.
  }, [isRateLimited])

  const onSubmit = handleSubmit((values) => {
    login.mutate(values, {
      // Setting state from a mutation callback (an event handler, not a
      // render effect) is the allowed place to do this: it runs in
      // response to the login attempt actually failing, not as a
      // synchronous side effect of rendering.
      onError: (error) => {
        if (error instanceof RateLimitError) {
          setSecondsRemaining(error.retryAfterSeconds)
        }
      },
    })
  })

  return (
    <form
      onSubmit={(e) => {
        void onSubmit(e)
      }}
      className="mt-4 space-y-4"
    >
      <FieldGroup>
        <Field data-invalid={formState.errors.username ? true : undefined}>
          <FieldLabel htmlFor="login-username">Username</FieldLabel>
          <Input
            id="login-username"
            autoComplete="username"
            aria-invalid={!!formState.errors.username}
            {...register('username')}
          />
          <FieldError errors={[formState.errors.username]} />
        </Field>
        <Field data-invalid={formState.errors.password ? true : undefined}>
          <FieldLabel htmlFor="login-password">Password</FieldLabel>
          <Input
            id="login-password"
            type="password"
            autoComplete="current-password"
            aria-invalid={!!formState.errors.password}
            {...register('password')}
          />
          <FieldError errors={[formState.errors.password]} />
        </Field>
      </FieldGroup>

      {isRateLimited ? (
        <Alert variant="destructive">
          <AlertTitle>Too many attempts</AlertTitle>
          <AlertDescription>Try again in {secondsRemaining}s.</AlertDescription>
        </Alert>
      ) : login.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{login.error.message}</AlertDescription>
        </Alert>
      ) : null}

      <Button
        type="submit"
        className="w-full"
        disabled={login.isPending || isRateLimited}
      >
        {login.isPending ? 'Signing in...' : 'Sign in'}
      </Button>
    </form>
  )
}
