import { reactive } from 'vue'
import { api, ApiError, followJob } from '../api'

// The job shown in the job dialog (App.vue).
export const activeJob = reactive({ job: null, lines: [], result: null, resolve: null })

function show(job) {
  Object.assign(activeJob, { job, lines: [], result: null })
  return new Promise((resolve) => {
    activeJob.resolve = resolve
    followJob(job.id, (line) => activeJob.lines.push(line)).then((result) => {
      activeJob.result = result
    })
  })
}

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
