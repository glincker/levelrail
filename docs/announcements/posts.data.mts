import { createContentLoader } from 'vitepress'

export interface Post {
  title: string
  description: string
  date: string
  url: string
}

declare const data: Post[]
export { data }

export default createContentLoader('announcements/*.md', {
  transform(raw): Post[] {
    return raw
      .filter((p) => !p.url.endsWith('/announcements/') && p.frontmatter.date)
      .map((p) => ({
        title: String(p.frontmatter.title),
        description: String(p.frontmatter.description ?? ''),
        date: new Date(p.frontmatter.date).toISOString().slice(0, 10),
        url: p.url,
      }))
      .sort((a, b) => b.date.localeCompare(a.date))
  },
})
