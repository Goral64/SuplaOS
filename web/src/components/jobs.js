import { reactive } from 'vue'
import { api, ApiError, followJob } from '../api'

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

const show = (job) => showOutput(job.title, '', (onLine) => followJob(job.id, onLine))

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
