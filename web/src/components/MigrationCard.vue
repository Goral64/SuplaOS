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

const cancel = () => call(async () => {
  if (!(await ask(t(view.value.prepared ? 'migration.cancelPreparedConfirm' : 'migration.cancelConfirm'),
    { ok: t('migration.cancel'), danger: true }))) return
  await api('DELETE', '/migration')
  await load()
})

const count = (status) => view.value?.devices?.filter((d) => d.status === status).length || 0
const others = computed(() => Object.entries(view.value?.other || {}))
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
      <p v-if="others.length" class="alert warn">
        {{ $t('migration.notMoved') }}
        <span v-for="([k, n], i) in others" :key="k">{{ i ? ', ' : ' ' }}{{ $t('migration.other.' + k) }}: {{ n }}</span>
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
        <div class="row mt">
          <button type="button" class="link danger" :disabled="busy" @click="cancel">{{ $t('migration.finish') }}</button>
        </div>
      </template>
    </template>
  </div>
</template>
