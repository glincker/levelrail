import { docsBody, loadReleases } from '../.vitepress/releases.mts'

export default {
  async paths() {
    const releases = await loadReleases()
    return releases.map((r) => ({
      params: { slug: r.slug, tag: r.tag, date: r.date, url: r.url, prerelease: r.prerelease },
      content: docsBody(r.body),
    }))
  },
}
