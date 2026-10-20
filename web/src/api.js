import { reactive } from 'vue'

// Session state shared by the whole interface.
export const session = reactive({
  loaded: false,
  loggedIn: false,
  mustChange: false,
  setupDone: false,
  lang: '',
  user: 'supla',
})

export class ApiError extends Error {
  constructor(status, message, data) {
    super(message)
    this.status = status
    this.data = data
  }
}

// api calls the panel API. Changes carry the X-SuplaOS header the server
// requires.
export async function api(method, path, body) {
  const opts = { method, headers: {} }
  if (method !== 'GET') opts.headers['X-SuplaOS'] = '1'
  if (body !== undefined) {
    opts.headers['Content-Type'] = 'application/json'
    opts.body = JSON.stringify(body)
  }
  const res = await fetch('/api' + path, opts)
  let data = null
  const text = await res.text()
  if (text) {
    try { data = JSON.parse(text) } catch { data = text }
  }
  if (!res.ok) {
    if (res.status === 401 && path !== '/login') {
      // Session expired: back to the login page.
      session.loggedIn = false
      window.location.assign('/login')
    }
    throw new ApiError(res.status, (data && data.error) || res.statusText, data)
  }
  return data
}

export async function loadSession() {
  Object.assign(session, await api('GET', '/session'), { loaded: true })
  return session
}

// followJob streams the output of a job; onLine gets every line, the
// promise resolves with the final job state.
export function followJob(id, onLine) {
  return new Promise((resolve) => {
    const es = new EventSource(`/api/jobs/${id}/events`)
    es.addEventListener('line', (e) => onLine(e.data))
    es.addEventListener('end', (e) => {
      es.close()
      resolve(JSON.parse(e.data))
    })
    es.onerror = () => {
      es.close()
      api('GET', `/jobs/${id}`).then(resolve, () => resolve({ state: 'failed', error: 'connection lost' }))
    }
  })
}

export function formatBytes(n) {
  if (!n) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  while (n >= 1024 && i < units.length - 1) { n /= 1024; i++ }
  return `${n.toFixed(i ? 1 : 0)} ${units[i]}`
}

// text picks the language version of a catalog text ({en, pl}).
export function text(t, lang) {
  if (!t) return ''
  return t[lang] || t.en || ''
}
