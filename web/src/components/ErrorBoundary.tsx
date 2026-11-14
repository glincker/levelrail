import { Component, type ReactNode } from 'react'

interface Props {
  /** Rendered instead of the children after a render error. */
  fallback: (error: Error) => ReactNode
  /** The boundary clears itself when any of these change (shallow compare). */
  resetKeys?: readonly unknown[]
  children: ReactNode
}

interface State {
  error: Error | null
}

function changed(a: readonly unknown[] = [], b: readonly unknown[] = []) {
  return a.length !== b.length || a.some((v, i) => !Object.is(v, b[i]))
}

/** Contains a render crash to one region so the rest of the page stays usable. */
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: unknown): State {
    return { error: error instanceof Error ? error : new Error(String(error)) }
  }

  componentDidUpdate(prev: Props): void {
    if (this.state.error && changed(prev.resetKeys, this.props.resetKeys)) {
      this.setState({ error: null })
    }
  }

  render(): ReactNode {
    return this.state.error
      ? this.props.fallback(this.state.error)
      : this.props.children
  }
}
