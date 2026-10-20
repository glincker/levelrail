import * as React from 'react'

const MOBILE_BREAKPOINT = 768

export function useIsMobile() {
  // Lazy initializer computes the real value on first render instead of
  // starting at `undefined` and setting it from inside the effect: this
  // is a pure client SPA (no SSR), so `window` is always available here,
  // and the effect below is left to do only what an effect should,
  // subscribe to the media query's own change events.
  const [isMobile, setIsMobile] = React.useState<boolean>(
    () => window.innerWidth < MOBILE_BREAKPOINT,
  )

  React.useEffect(() => {
    const mql = window.matchMedia(`(max-width: ${MOBILE_BREAKPOINT - 1}px)`)
    const onChange = () => {
      setIsMobile(window.innerWidth < MOBILE_BREAKPOINT)
    }
    mql.addEventListener('change', onChange)
    return () => mql.removeEventListener('change', onChange)
  }, [])

  return isMobile
}
