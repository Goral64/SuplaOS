<script setup>
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, formatBytes } from '../api'
import { ask } from './dialog'
import { runJob } from './jobs'

// In the setup wizard only a backup from another device is restored:
// sent from the browser, no list and no new backups.
const props = defineProps({ setup: Boolean })
const emit = defineEmits(['restored'])

const { t, locale } = useI18n()
const backups = ref([])
const password = ref('')
const repeat = ref('')
const noPassword = ref(false)
// The backup being restored: { name, encrypted, password }.
const restoring = ref(null)
const error = ref('')
const message = ref('')
const busy = ref('')

const encrypted = (name) => name.endsWith('.age')
const when = (d) => new Date(d).toLocaleString(locale.value)

async function load() {
  if (props.setup) return
  backups.value = (await api('GET', '/backups')).filter((b) => b.full)
}
onMounted(load)
defineExpose({ load })

async function call(kind, fn) {
  busy.value = kind
  error.value = message.value = ''
  try {
    await fn()
  } catch (e) {
    error.value = e.message
  } finally {
    busy.value = ''
  }
}

const mismatch = computed(() => !noPassword.value && repeat.value !== '' && password.value !== repeat.value)

const create = () => call('create', async () => {
  if (!noPassword.value && password.value !== repeat.value) {
    error.value = t('fullBackup.passwordsDiffer')
    return
  }
  const result = await runJob('POST', '/system-backup', { password: noPassword.value ? '' : password.value })
  if (result && result.state === 'done') {
    password.value = repeat.value = ''
    message.value = t('fullBackup.created')
  }
  await load()
})

const upload = (e) => call('upload', async () => {
  const file = e.target.files[0]
  e.target.value = ''
  if (!file) return
  const { name } = await api('POST', `/system-backup/upload?name=${encodeURIComponent(file.name)}`, file)
  await load()
  restoring.value = { name, encrypted: encrypted(name), password: '' }
})

function startRestore(b) {
  restoring.value = { name: b.name, encrypted: encrypted(b.name), password: '' }
}

const restore = () => call('restore', async () => {
  const { name, password: pw } = restoring.value
  const m = await api('POST', '/system-backup/check', { name, password: pw })
  const info = t('fullBackup.info', {
    host: m.hostname, date: when(m.created), version: m.version || '?',
    addons: m.addons.length ? m.addons.join(', ') : t('fullBackup.noAddons'),
  })
  const question = props.setup ? t('fullBackup.restoreSetupConfirm', { info }) : t('fullBackup.restoreConfirm', { info })
  if (!(await ask(question, { ok: t('fullBackup.restore'), danger: !props.setup }))) return
  const result = await runJob('POST', '/system-backup/restore', { name, password: pw })
  if (result && result.state === 'done') {
    restoring.value = null
    message.value = t('fullBackup.restored')
    emit('restored')
  }
  await load()
})

const remove = (b) => call('delete', async () => {
  if (!(await ask(`${t('common.delete')} ${b.name}?`, { ok: t('common.delete'), danger: true }))) return
  await api('DELETE', `/backups/${encodeURIComponent(b.name)}`)
  await load()
})
</script>

<template>
  <div :class="{ card: !props.setup }">
    <h2 v-if="!props.setup">{{ $t('fullBackup.title') }}</h2>
    <div v-if="error" class="alert error">{{ error }}</div>
    <div v-if="message" class="alert ok">{{ message }}</div>

    <template v-if="!props.setup">
      <p>{{ $t('fullBackup.intro') }}</p>
      <form @submit.prevent="create">
        <template v-if="!noPassword">
          <div class="field">
            <label for="fb-password" class="required">{{ $t('fullBackup.password') }}</label>
            <input id="fb-password" v-model="password" type="password" autocomplete="new-password" minlength="8" required>
            <div class="help">{{ $t('fullBackup.passwordHelp') }}</div>
          </div>
          <div class="field">
            <label for="fb-repeat" class="required">{{ $t('fullBackup.repeat') }}</label>
            <input id="fb-repeat" v-model="repeat" type="password" autocomplete="new-password" required>
            <div v-if="mismatch" class="help" style="color: var(--danger)">{{ $t('fullBackup.passwordsDiffer') }}</div>
          </div>
        </template>
        <div class="field">
          <label class="check"><input v-model="noPassword" type="checkbox"> {{ $t('fullBackup.noPassword') }}</label>
          <div v-if="noPassword" class="alert warn mt">{{ $t('fullBackup.noPasswordWarn') }}</div>
        </div>
        <button class="primary" :disabled="!!busy">{{ $t('fullBackup.create') }}</button>
      </form>
    </template>
    <p v-else class="muted small">{{ $t('fullBackup.setupIntro') }}</p>

    <form v-if="restoring" class="mt" @submit.prevent="restore">
      <p>{{ $t('fullBackup.restoring', { name: restoring.name }) }}</p>
      <div v-if="restoring.encrypted" class="field">
        <label for="fb-restore-password" class="required">{{ $t('fullBackup.backupPassword') }}</label>
        <input id="fb-restore-password" v-model="restoring.password" type="password" autocomplete="new-password" required>
      </div>
      <div class="row">
        <button type="button" :disabled="!!busy" @click="restoring = null">{{ $t('common.cancel') }}</button>
        <button class="primary" :disabled="!!busy">
          {{ busy === 'restore' ? $t('fullBackup.checking') : $t('fullBackup.restore') }}
        </button>
      </div>
    </form>

    <template v-else>
      <h3 v-if="!props.setup">{{ $t('fullBackup.list') }}</h3>
      <div class="row" :class="{ mt: props.setup }">
        <label class="button" :class="{ disabled: !!busy }">
          {{ busy === 'upload' ? $t('fullBackup.uploading') : $t('fullBackup.upload') }}
          <input type="file" accept=".gz,.age" hidden :disabled="!!busy" @change="upload">
        </label>
      </div>
      <template v-if="!props.setup">
        <p v-if="!backups.length" class="muted mt">{{ $t('fullBackup.none') }}</p>
        <table v-else class="mt stack">
          <tr v-for="b in backups" :key="b.name">
            <td>
              {{ when(b.time) }}
              <span v-if="encrypted(b.name)" class="badge ok">{{ $t('fullBackup.encrypted') }}</span>
              <span v-else class="badge warn">{{ $t('fullBackup.plain') }}</span>
              <div class="muted small">{{ b.name }}</div>
            </td>
            <td class="muted">{{ formatBytes(b.size) }}</td>
            <td style="text-align: right">
              <button :disabled="!!busy" @click="startRestore(b)">{{ $t('fullBackup.restore') }}</button>
              <a class="button" :href="'/api/backups/' + encodeURIComponent(b.name)">{{ $t('common.download') }}</a>
              <button class="link danger" :disabled="!!busy" @click="remove(b)">{{ $t('common.delete') }}</button>
            </td>
          </tr>
        </table>
      </template>
    </template>
  </div>
</template>
