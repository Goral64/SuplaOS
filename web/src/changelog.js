import en from '../../CHANGELOG.md?raw'
import pl from '../../docs/pl/CHANGELOG.md?raw'

const repo = 'Goral64/SuplaOS'
const files = { en: 'CHANGELOG.md', pl: 'docs/pl/CHANGELOG.md' }

// The changes as this panel was built with them.
export const builtIn = (lang) => (lang === 'pl' ? pl : en)

// fetchChangelog gets the changes of a released version from GitHub, null
// when they are not there (no connection, a development version).
export async function fetchChangelog(version, lang) {
  const ctl = new AbortController()
  const t = setTimeout(() => ctl.abort(), 5000)
  try {
    const r = await fetch(`https://raw.githubusercontent.com/${repo}/v${version}/${files[lang] || files.en}`, { signal: ctl.signal })
    return r.ok ? await r.text() : null
  } catch {
    return null
  } finally {
    clearTimeout(t)
  }
}

const escape = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')

// inline handles `code` and [links](url); relative links point to the file
// in the repository, next to the changelog of that language.
function inline(text, lang) {
  const dir = lang === 'pl' ? 'docs/pl/' : ''
  return escape(text)
    .replace(/`([^`]+)`/g, '<code>$1</code>')
    .replace(/\[([^\]]+)\]\(([^)\s]+)\)/g, (_, label, url) => {
      if (!/^https?:/.test(url)) url = new URL(url, `https://github.com/${repo}/blob/main/${dir}`).href
      return `<a href="${url}" target="_blank" rel="noopener">${label}</a>`
    })
}

// parseChangelog splits CHANGELOG.md into versions, [{version, html}],
// newest first. Only what the file uses: paragraphs and lists.
export function parseChangelog(md, lang) {
  const sections = []
  let cur = null
  let block = null
  // Text before the first version (title, link to the other language) is
  // left out.
  const flush = () => {
    if (block && cur) {
      cur.html += block.list
        ? '<ul>' + block.items.map((i) => `<li>${inline(i, lang)}</li>`).join('') + '</ul>'
        : `<p>${inline(block.items.join(' '), lang)}</p>`
    }
    block = null
  }
  for (const line of md.split('\n')) {
    const h = line.match(/^## (.+)/)
    if (h) {
      flush()
      cur = { version: h[1].trim(), html: '' }
      sections.push(cur)
    } else if (!line.trim()) {
      flush()
    } else if (line.startsWith('- ')) {
      if (!block?.list) {
        flush()
        block = { list: true, items: [] }
      }
      block.items.push(line.slice(2))
    } else if (block && /^\s/.test(line)) {
      block.items[block.items.length - 1] += ' ' + line.trim()
    } else {
      if (block?.list) flush()
      if (!block) block = { list: false, items: [] }
      block.items.push(line.trim())
    }
  }
  flush()
  return sections
}
