// One module per language keeps its file map out of the first-load chunk.
export const load = (namespace: string) =>
  import(`../../locales/de/${namespace}.json`)
