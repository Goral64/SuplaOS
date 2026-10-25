import { reactive } from 'vue'
import { api, ApiError, followJob } from '../api'
import { i18n } from '../i18n'

// The job shown in the job dialog (App.vue).
export const activeJob = reactive({ job: null, note: '', lines: [], result: null, resolve: null })

// showOutput shows in the job dialog the lines follow(onLine) passes on;
// follow resolves with the result ({state, error}). It resolves with the
// result when the dialog is closed.
export function showOutput(title, note, follow) {
  Object.assign(activeJob, { job: { title }, note, lines: [], result: null })
  return new Promise((resolve) => {
    activeJob.resolve = resolve
    follow((line) => activeJob.lines.push(line)).then((result) => {
      activeJob.result = result
    })
  })
}

// The titles of jobs come from the panel in English: the fixed ones and
// those with a name after a verb ("install supla") are shown translated.
const fixedTitles = {
  'create Supla account': 'createAccount',
  'check for updates': 'checkUpdates',
  'apply add-on updates': 'applyUpdates',
  'automatic backup': 'autoBackup',
  'addresses at the domain': 'domainAddresses',
  'connect SUPLA Scripts with Supla Cloud': 'connectScripts',
  'full backup': 'fullBackup',
  'restore full backup': 'restoreFull',
  'prepare the move from Supla Cloud': 'prepareMove',
  'copy the measurement history': 'copyHistory',
  'certificate of the device': 'deviceCertificate',
}
const namedTitles = ['install', 'configure', 'remove', 'restore', 'start', 'stop', 'restart', 'update', 'backup', 'certificate for']

export function jobTitle(title) {
  const { t, te } = i18n.global
  if (fixedTitles[title]) return t('jobTitle.' + fixedTitles[title])
  for (const verb of namedTitles) {
    const key = 'jobTitle.' + verb.replace(' ', '_')
    if (title.startsWith(verb + ' ') && te(key)) return t(key, { name: title.slice(verb.length + 1) })
  }
  return title
}

const show = (job) => showOutput(jobTitle(job.title), '', (onLine) => followJob(job.id, onLine))

// runJob starts a job with an API call and shows its output; it resolves
// with the final job state when the dialog is closed.
export async function runJob(method, path, body) {
  try {
    return await show(await api(method, path, body))
  } catch (e) {
    if (e instanceof ApiError && e.status === 409 && e.data && e.data.job) {
      return show(e.data.job)
    }
    throw e
  }
}

export function closeJob() {
  const { resolve, result } = activeJob
  activeJob.job = null
  if (resolve) resolve(result)
}
