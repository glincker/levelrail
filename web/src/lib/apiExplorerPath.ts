// Pure path-parameter helper for ApiExplorerRoute.tsx's "Try it" form,
// split into its own module (not exported from the component file)
// because components/settings/** only exports components, enforced by
// react-refresh/only-export-components.

export function pathParamNames(path: string): string[] {
  return [...path.matchAll(/\{([^}]+)\}/g)]
    .map((m) => m[1])
    .filter((name): name is string => Boolean(name))
}
