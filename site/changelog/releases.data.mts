import { loadReleases } from '../.vitepress/releases.mts'

export default {
  async load() {
    const releases = await loadReleases()
    return releases.map(({ tag, slug, date, prerelease, highlights }) => ({
      tag,
      slug,
      date,
      prerelease,
      highlights: highlights.slice(0, 4),
    }))
  },
}
