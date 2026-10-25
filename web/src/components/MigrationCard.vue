<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
import { ask } from './dialog'
import { runJob } from './jobs'

const { t, locale } = useI18n()
const view = ref(null)
const token = ref('')
const password = ref('')
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

const history = () => call(async () => {
  await runJob('POST', '/migration/history')
  await load()
})

const cancel = () => call(async () => {
  if (!(await ask(t(view.value.prepared ? 'migration.cancelPreparedConfirm' : 'migration.cancelConfirm'),
    { ok: t('migration.cancel'), danger: true }))) return
  await api('DELETE', '/migration')
  await load()
})

const count = (status) => view.value?.devices?.filter((d) => d.status === status).length || 0
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
      <p v-if="objects.length && !view.prepared">
        {{ $t('migration.also') }}
        <span v-for="([k, n], i) in objects" :key="k">{{ i ? ', ' : ' ' }}{{ $t('migration.kinds.' + k) }}: {{ n }}</span>.
      </p>

      <form v-if="!view.prepared" @submit.prevent="prepare">
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
        <p class="alert ok">{{ $t('migration.progress', { done: count('done'), all: view.devices.length }) }}</p>
        <p v-if="objects.length">
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
        <p>{{ $t('migration.switch', { server: view.server }) }}</p>
        <p v-if="count('error')" class="alert error">{{ $t('migration.errors', { n: count('error') }) }}</p>
        <details>
          <summary>{{ $t('migration.list', { waiting: count('waiting') }) }}</summary>
          <table class="mt">
            <tr v-for="d in view.devices" :key="d.id">
              <td>{{ d.name }} <span class="muted small">ID {{ d.id }}</span></td>
              <td>
                <span class="badge" :class="{ ok: d.status === 'done', bad: d.status === 'error' }">{{ $t('migration.status.' + d.status) }}</span>
                <div v-if="d.error" class="small" style="color: var(--danger)">{{ d.error }}</div>
              </td>
            </tr>
          </table>
        </details>
        <p v-if="view.history" :class="view.history.error ? 'alert error' : 'muted'">
          {{ $t('migration.history', { date: when(view.history.time), n: view.history.items }) }}
          <template v-if="view.history.error"> {{ view.history.error }}</template>
        </p>
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
