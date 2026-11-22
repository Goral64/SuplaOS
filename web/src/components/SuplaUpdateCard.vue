<script setup>
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
import { ask } from './dialog'
import { runJob } from './jobs'

const emit = defineEmits(['updated'])
const { t, locale } = useI18n()
const view = ref(null)
const error = ref('')
const busy = ref(false)

async function load() {
  view.value = await api('GET', '/supla/update')
}
onMounted(() => load().catch((e) => { error.value = e.message }))

async function call(fn) {
  busy.value = true
  error.value = ''
  try {
    await fn()
    await load()
  } catch (e) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}

const check = () => call(() => runJob('POST', '/supla/check'))
const upgrade = () => call(async () => {
  const tag = update.value.release.tag
  const db = database.value
  let question = t('suplaUpdate.confirm', { tag })
  if (db?.raise) question += '\n\n' + t('suplaUpdate.confirmDatabase', { from: db.here, to: db.release })
  if (!(await ask(question, { ok: t('suplaUpdate.upgrade') }))) return
  await runJob('POST', '/supla/upgrade', { tag })
  emit('updated')
})

const update = computed(() => view.value?.update)
const database = computed(() => update.value?.database)
// The notes without the table of badges: steps to do by hand and the rest.
const notes = computed(() => (update.value?.release.notes || '').split('\n')
  .filter((l) => !/shields\.io|^\s*\|/.test(l)).join('\n').trim())
</script>

<template>
  <div v-if="view?.installed" class="card">
    <div class="row">
      <h2 style="margin: 0">{{ $t('suplaUpdate.title') }}</h2>
      <span class="spacer"></span>
      <button :disabled="busy" @click="check">{{ $t('suplaUpdate.check') }}</button>
    </div>
    <div v-if="error || view.error" class="alert error mt">{{ error || view.error }}</div>
    <p v-if="!update" class="muted mt">{{ $t('suplaUpdate.notChecked') }}</p>
    <template v-else>
      <p class="mt" :class="{ 'alert ok': update.state === 'available', 'alert warn': update.state === 'waiting' || update.state === 'unreadable' }">
        {{ $t('suplaUpdate.state.' + update.state, { tag: update.release.tag, date: new Date(update.release.published).toLocaleDateString(locale) }) }}
      </p>
      <table class="small">
        <tr><th></th><th>{{ $t('suplaUpdate.here') }}</th><th>{{ update.release.tag }}</th></tr>
        <tr><td>supla-docker</td><td>{{ update.tag || '-' }}</td><td><a :href="update.release.url" target="_blank" rel="noopener">{{ update.release.tag }}</a></td></tr>
        <tr><td>Supla Cloud</td><td>{{ update.here.cloud }}</td><td>{{ update.release.versions.cloud || '-' }}</td></tr>
        <tr><td>supla-server</td><td>{{ update.here.server.replace(/^v/, '') }}</td><td>{{ update.release.versions.server || '-' }}</td></tr>
        <tr v-if="database"><td>{{ $t('suplaUpdate.database') }}</td><td>{{ database.here }}</td><td>{{ database.release }}</td></tr>
      </table>
      <p v-if="update.waiting?.length" class="small">{{ $t('suplaUpdate.waiting', { list: update.waiting.join(', ') }) }}</p>
      <p v-if="database?.raise" class="alert warn small mt">{{ $t('suplaUpdate.databaseChanged', { to: database.release }) }}</p>
      <details v-if="notes && update.state !== 'current'" class="mt" open>
        <summary>{{ $t('suplaUpdate.notes') }}</summary>
        <pre class="log">{{ notes }}</pre>
      </details>
      <template v-if="update.state === 'available'">
        <p class="muted small mt">{{ $t('suplaUpdate.help') }}</p>
        <div class="row">
          <button class="primary" :disabled="busy" @click="upgrade">{{ $t('suplaUpdate.upgrade') }}</button>
          <span class="muted small">{{ $t('suplaUpdate.terminal') }} <code>suplaos supla upgrade</code></span>
        </div>
      </template>
    </template>
  </div>
</template>
