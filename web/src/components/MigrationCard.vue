<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
import { localError } from '../errors'
import { ask } from './dialog'
import { runJob } from './jobs'

const { t, locale } = useI18n()
const view = ref(null)
const token = ref('')
const scriptsURL = ref('https://scripts.supla.io')
const scriptsToken = ref('')
const password = ref('')
// Devices connected here already that get the settings of the source again.
const overwrite = ref([])
const error = ref('')
const busy = ref(false)
let timer

async function load() {
  view.value = await api('GET', '/migration')
}
onMounted(async () => {
  await load()
  // Devices connect while the page is open.
  timer = setInterval(() => view.value?.prepared && load(), 30000)
})
onUnmounted(() => clearInterval(timer))

async function call(fn) {
  busy.value = true
  error.value = ''
  try {
    await fn()
  } catch (e) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}

const fetchSource = () => call(async () => {
  view.value = await api('POST', '/migration/fetch', { token: token.value })
  token.value = ''
})

const prepare = () => call(async () => {
  if (!(await ask(t('migration.prepareConfirm'), { ok: t('migration.prepare') }))) return
  await runJob('POST', '/migration/prepare', { password: password.value })
  password.value = ''
  await load()
})

const resume = () => call(async () => {
  if (!(await ask(t('migration.resumeConfirm'), { ok: t('migration.resume') }))) return
  await runJob('POST', '/migration/resume', { password: password.value, overwrite: overwrite.value })
  password.value = ''
  overwrite.value = []
  await load()
})

const history = () => call(async () => {
  await runJob('POST', '/migration/history')
  await load()
})

const moveScripts = () => call(async () => {
  await runJob('POST', '/migration/scripts', { url: scriptsURL.value, token: scriptsToken.value.trim() })
  await load()
})

// What is lost by ending the move now.
function endQuestion() {
  const lines = []
  const all = moving.value
  const left = all - moved.value
  if (left) lines.push(t('migration.endWaiting', { n: left, all }))
  if (historyBad.value) lines.push(t('migration.endHistory'))
  lines.push(t('migration.cancelPreparedConfirm'))
  return lines.join('\n\n')
}

const cancel = () => call(async () => {
  // Devices connect while the page is open.
  await load()
  const prepared = view.value.prepared
  if (!(await ask(prepared ? endQuestion() : t('migration.cancelConfirm'),
    { ok: t(prepared ? 'migration.finish' : 'migration.forget'), danger: true }))) return
  await api('DELETE', '/migration')
  await load()
})

const count = (status) => view.value?.devices?.filter((d) => d.status === status).length || 0
// A resumed move leaves alone the devices not here or of another device.
const moving = computed(() => (view.value?.devices?.length || 0) - count('missing') - count('foreign'))
const moved = computed(() => count('done') + count('kept'))
const name = (id) => view.value?.devices?.find((d) => d.id === id)?.name || ''
// Not copied to the end: a failure, or interrupted before it was noted.
const historyBad = computed(() => !view.value?.history || !!view.value.history.error)
// Kinds of objects with their number in the source, those there are.
const objects = computed(() => Object.entries(view.value?.objects || {}).filter(([, n]) => n > 0))
const made = (kind) => view.value?.made?.counts?.[kind] || 0
const when = (d) => new Date(d).toLocaleString(locale.value)
</script>

<template>
  <div v-if="view" class="card">
    <h2>{{ $t('migration.title') }}</h2>
    <div v-if="error" class="alert error">{{ error }}</div>

    <form v-if="!view.source" @submit.prevent="fetchSource">
      <p>{{ $t('migration.intro') }}</p>
      <div class="field">
        <label for="mig-token" class="required">{{ $t('migration.token') }}</label>
        <textarea id="mig-token" v-model="token" rows="2" required spellcheck="false"></textarea>
        <div class="help">{{ $t('migration.tokenHelp') }}</div>
      </div>
      <button class="primary" :disabled="busy">{{ busy ? $t('migration.reading') : $t('migration.read') }}</button>
    </form>

    <template v-else>
      <p>
        {{ $t('migration.summary', { source: view.source, email: view.email, date: when(view.fetched) }) }}<br>
        {{ $t('migration.counts', { devices: view.devices.length, channels: view.channels, locations: view.locations, accessIds: view.accessIds }) }}
      </p>
      <p v-if="objects.length && !view.prepared && !view.resume">
        {{ $t('migration.also') }}
        <span v-for="([k, n], i) in objects" :key="k">{{ i ? ', ' : ' ' }}{{ $t('migration.kinds.' + k) }}: {{ n }}</span>.
      </p>

      <form v-if="!view.prepared && view.resume" @submit.prevent="resume">
        <p>{{ $t('migration.resumeIntro', { email: view.email }) }}</p>
        <p v-if="view.resume.waiting.length">{{ $t('migration.resumeWaiting', { n: view.resume.waiting.length }) }}</p>
        <template v-if="view.resume.here.length">
          <p>{{ $t('migration.resumeHere', { n: view.resume.here.length }) }}</p>
          <div v-for="id in view.resume.here" :key="id" class="field">
            <label class="check">
              <input v-model="overwrite" type="checkbox" :value="id">
              {{ name(id) }} <span class="muted small">ID {{ id }}</span>
            </label>
          </div>
        </template>
        <p v-if="view.resume.missing.length" class="muted small">
          {{ $t('migration.resumeMissing') }} {{ view.resume.missing.map((id) => `${name(id)} (ID ${id})`).join(', ') }}
        </p>
        <p v-if="view.resume.foreign.length" class="muted small">
          {{ $t('migration.resumeForeign') }} {{ view.resume.foreign.map((id) => `${name(id)} (ID ${id})`).join(', ') }}
        </p>
        <div class="field">
          <label for="mig-pass" class="required">{{ $t('migration.password', { email: view.email }) }}</label>
          <input id="mig-pass" v-model="password" type="password" autocomplete="current-password" required>
        </div>
        <div class="row">
          <button class="primary" :disabled="busy">{{ $t('migration.resume') }}</button>
          <button type="button" class="link danger" :disabled="busy" @click="cancel">{{ $t('migration.cancel') }}</button>
        </div>
      </form>

      <form v-else-if="!view.prepared" @submit.prevent="prepare">
        <p class="muted small">{{ $t('migration.prepareHelp', { email: view.email }) }}</p>
        <div class="field">
          <label for="mig-pass" class="required">{{ $t('migration.password', { email: view.email }) }}</label>
          <input id="mig-pass" v-model="password" type="password" autocomplete="new-password" required>
        </div>
        <div class="row">
          <button class="primary" :disabled="busy">{{ $t('migration.prepare') }}</button>
          <button type="button" class="link danger" :disabled="busy" @click="cancel">{{ $t('migration.cancel') }}</button>
        </div>
      </form>

      <template v-else>
        <p class="alert ok">{{ $t('migration.progress', { done: moved, all: moving }) }}</p>
        <p v-if="objects.length && !view.resumed">
          {{ $t('migration.made') }}
          <span v-for="([k, n], i) in objects" :key="k">{{ i ? ', ' : ' ' }}{{ $t('migration.kinds.' + k) }}: {{ made(k) }}/{{ n }}</span>.
        </p>
        <div v-if="view.made?.errors?.length" class="alert error">
          {{ $t('migration.notMade') }}
          <ul class="small"><li v-for="e in view.made.errors" :key="e">{{ e }}</li></ul>
        </div>
        <template v-if="view.links.length">
          <p>{{ $t('migration.links') }}</p>
          <table class="stack small">
            <tr v-for="l in view.links" :key="l.url">
              <td>{{ l.caption }}</td>
              <td><code class="fingerprint">{{ l.url }}</code></td>
            </tr>
          </table>
        </template>
        <p>{{ $t('migration.switch', { server: view.server, email: view.email }) }}</p>
        <p v-if="count('error')" class="alert error">{{ $t('migration.errors', { n: count('error') }) }}</p>
        <details>
          <summary>{{ $t('migration.list', { waiting: count('waiting') }) }}</summary>
          <table class="mt">
            <tr v-for="d in view.devices" :key="d.id">
              <td>{{ d.name }} <span class="muted small">ID {{ d.id }}</span></td>
              <td>
                <span class="badge" :class="{ ok: d.status === 'done' || d.status === 'kept', bad: d.status === 'error' }">{{ $t('migration.status.' + d.status) }}</span>
                <div v-if="d.error" class="small" style="color: var(--danger)">{{ localError(d.error) }}</div>
              </td>
            </tr>
          </table>
        </details>
        <div v-if="historyBad" class="alert error">
          {{ $t('migration.historyFailed') }}
          <div v-if="view.history?.error" class="small">{{ localError(view.history.error) }}</div>
        </div>
        <p v-else class="muted">
          {{ $t('migration.history', { date: when(view.history.time), n: view.history.items }) }}
        </p>
        <form v-if="view.scriptsInstalled" class="mt" @submit.prevent="moveScripts">
          <h3>SUPLA Scripts</h3>
          <p class="muted small">{{ $t('migration.scriptsHelp') }}</p>
          <div class="field">
            <label for="mig-scripts">{{ $t('migration.scriptsURL') }}</label>
            <input id="mig-scripts" v-model="scriptsURL" type="url" required>
          </div>
          <div class="field">
            <label for="mig-scripts-token">{{ $t('migration.scriptsToken') }}</label>
            <textarea id="mig-scripts-token" v-model="scriptsToken" rows="2" spellcheck="false"></textarea>
            <div class="help">{{ $t('migration.scriptsTokenHelp') }}</div>
          </div>
          <p v-if="view.scripts">
            {{ $t('migration.scriptsDone', { date: when(view.scripts.time) }) }}
            <span v-for="([k, n], i) in Object.entries(view.scripts.counts || {})" :key="k">{{ i ? ', ' : ' ' }}{{ $t('migration.scriptsKinds.' + k) }}: {{ n }}</span>
          </p>
          <div v-if="view.scripts?.errors?.length" class="alert error">
            <ul class="small"><li v-for="e in view.scripts.errors" :key="e">{{ e }}</li></ul>
          </div>
          <button :disabled="busy">{{ $t('migration.scriptsMove') }}</button>
        </form>
        <div class="row mt">
          <button type="button" :disabled="busy" @click="history">{{ $t('migration.historyAgain') }}</button>
          <span class="spacer"></span>
          <button type="button" class="link danger" :disabled="busy" @click="cancel">{{ $t('migration.finish') }}</button>
        </div>
        <p class="muted small">{{ $t('migration.historyHelp') }}</p>
      </template>
    </template>
  </div>
</template>
