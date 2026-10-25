<script setup>
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, formatBytes } from '../api'
import { runJob } from './jobs'

const emit = defineEmits(['done'])
const { t, locale } = useI18n()
const view = ref(null)
const form = ref(null)
const password = ref('')
const repeat = ref('')
const smbPassword = ref('')
const error = ref('')
const message = ref('')
const busy = ref('')

const weekdays = computed(() => [1, 2, 3, 4, 5, 6, 0].map((d) => ({
  value: d,
  name: new Date(2026, 9, 25 + d).toLocaleDateString(locale.value, { weekday: 'long' }),
})))
const when = (d) => new Date(d).toLocaleString(locale.value)

async function load() {
  view.value = await api('GET', '/system-backup/auto')
  form.value = JSON.parse(JSON.stringify(view.value.settings))
  password.value = repeat.value = smbPassword.value = ''
}
onMounted(load)

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

const body = () => ({ ...form.value, password: password.value, smbPassword: smbPassword.value })

function checkPasswords() {
  if (!form.value.plain && password.value !== repeat.value) {
    error.value = t('fullBackup.passwordsDiffer')
    return false
  }
  return true
}

const save = () => call('save', async () => {
  if (!checkPasswords()) return
  view.value = await api('PUT', '/system-backup/auto', body())
  form.value = JSON.parse(JSON.stringify(view.value.settings))
  password.value = repeat.value = smbPassword.value = ''
  message.value = t('common.saved')
})

const test = () => call('test', async () => {
  if (!checkPasswords()) return
  await api('POST', '/system-backup/auto/test', body())
  message.value = t('autoBackup.testOK')
})

const runNow = () => call('run', async () => {
  await runJob('POST', '/system-backup/auto/run')
  await load()
  emit('done')
})

const share = computed(() => form.value && form.value.target !== 'device')
</script>

<template>
  <div v-if="form" class="card">
    <h2>{{ $t('autoBackup.title') }}</h2>
    <div v-if="error" class="alert error">{{ error }}</div>
    <div v-if="message" class="alert ok">{{ message }}</div>

    <p v-if="view.status.time" :class="view.status.ok ? 'muted' : 'alert error'">
      {{ $t('autoBackup.last', { date: when(view.status.time) }) }}
      <template v-if="view.status.ok">{{ $t('autoBackup.lastOK', { file: view.status.file, size: formatBytes(view.status.size) }) }}</template>
      <template v-else>{{ $t('autoBackup.lastFailed', { error: view.status.error }) }}</template>
    </p>
    <p v-if="view.next" class="muted">{{ $t('autoBackup.next', { date: when(view.next) }) }}</p>

    <form @submit.prevent="save">
      <div class="row">
        <div class="field">
          <label for="ab-schedule">{{ $t('autoBackup.schedule') }}</label>
          <select id="ab-schedule" v-model="form.schedule">
            <option value="off">{{ $t('autoBackup.off') }}</option>
            <option value="daily">{{ $t('autoBackup.daily') }}</option>
            <option value="weekly">{{ $t('autoBackup.weekly') }}</option>
          </select>
        </div>
        <div v-if="form.schedule === 'weekly'" class="field">
          <label for="ab-day">{{ $t('autoBackup.weekday') }}</label>
          <select id="ab-day" v-model.number="form.weekday">
            <option v-for="d in weekdays" :key="d.value" :value="d.value">{{ d.name }}</option>
          </select>
        </div>
        <div v-if="form.schedule !== 'off'" class="field">
          <label for="ab-time">{{ $t('autoBackup.time') }}</label>
          <input id="ab-time" v-model="form.time" type="time" required>
        </div>
      </div>

      <template v-if="form.schedule !== 'off'">
        <div class="field">
          <label>{{ $t('autoBackup.target') }}</label>
          <label class="check"><input v-model="form.target" type="radio" value="device"> {{ $t('autoBackup.device') }}</label>
          <label class="check"><input v-model="form.target" type="radio" value="smb"> {{ $t('autoBackup.smb') }}</label>
          <label class="check"><input v-model="form.target" type="radio" value="nfs"> {{ $t('autoBackup.nfs') }}</label>
          <div v-if="form.target === 'device'" class="help">{{ $t('autoBackup.deviceHelp') }}</div>
        </div>

        <template v-if="form.target === 'smb'">
          <div class="field">
            <label for="ab-share" class="required">{{ $t('autoBackup.share') }}</label>
            <input id="ab-share" v-model="form.smb.share" placeholder="//192.168.1.10/kopie" required>
          </div>
          <div class="row">
            <div class="field">
              <label for="ab-user">{{ $t('autoBackup.username') }}</label>
              <input id="ab-user" v-model="form.smb.username" autocomplete="off">
            </div>
            <div class="field">
              <label for="ab-smb-password">{{ $t('autoBackup.smbPassword') }}</label>
              <input id="ab-smb-password" v-model="smbPassword" type="password" autocomplete="new-password"
                :placeholder="view.smbHasPassword ? $t('autoBackup.saved') : ''">
            </div>
          </div>
          <div class="help field">{{ $t('autoBackup.guestHelp') }}</div>
          <div class="row">
            <div class="field">
              <label for="ab-domain">{{ $t('autoBackup.domain') }} <span class="muted small">({{ $t('common.optional') }})</span></label>
              <input id="ab-domain" v-model="form.smb.domain" autocomplete="off">
            </div>
            <div class="field">
              <label for="ab-version">{{ $t('autoBackup.version') }}</label>
              <select id="ab-version" v-model="form.smb.version">
                <option value="">{{ $t('autoBackup.versionAuto') }}</option>
                <option v-for="v in ['3.1.1', '3.0', '2.1', '2.0', '1.0']" :key="v" :value="v">SMB {{ v }}</option>
              </select>
            </div>
          </div>
        </template>

        <div v-if="form.target === 'nfs'" class="field">
          <label for="ab-export" class="required">{{ $t('autoBackup.export') }}</label>
          <input id="ab-export" v-model="form.nfs.export" placeholder="192.168.1.10:/volume1/kopie" required>
          <div class="help">{{ $t('autoBackup.nfsHelp') }}</div>
        </div>

        <div v-if="share" class="field">
          <label for="ab-dir">{{ $t('autoBackup.dir') }} <span class="muted small">({{ $t('common.optional') }})</span></label>
          <input id="ab-dir" v-model="form.dir" placeholder="suplaos">
        </div>

        <div class="field">
          <label for="ab-keep">{{ $t('autoBackup.keep') }}</label>
          <input id="ab-keep" v-model.number="form.keep" type="number" min="1" max="100" required style="max-width: 8rem">
          <div class="help">{{ $t('autoBackup.keepHelp') }}</div>
        </div>

        <template v-if="!form.plain">
          <div class="row">
            <div class="field">
              <label for="ab-password" :class="{ required: !view.hasPassword }">{{ $t('fullBackup.password') }}</label>
              <input id="ab-password" v-model="password" type="password" autocomplete="new-password" minlength="8"
                :required="!view.hasPassword" :placeholder="view.hasPassword ? $t('autoBackup.saved') : ''">
            </div>
            <div class="field">
              <label for="ab-repeat" :class="{ required: !view.hasPassword }">{{ $t('fullBackup.repeat') }}</label>
              <input id="ab-repeat" v-model="repeat" type="password" autocomplete="new-password" :required="!view.hasPassword || !!password">
            </div>
          </div>
          <div class="help field">{{ $t('autoBackup.passwordHelp') }}</div>
        </template>
        <div class="field">
          <label class="check"><input v-model="form.plain" type="checkbox"> {{ $t('fullBackup.noPassword') }}</label>
          <div v-if="form.plain" class="alert warn mt">{{ $t('fullBackup.noPasswordWarn') }}</div>
        </div>
      </template>

      <div class="row">
        <button class="primary" :disabled="!!busy">{{ $t('common.save') }}</button>
        <button v-if="form.schedule !== 'off' && share" type="button" :disabled="!!busy" @click="test">
          {{ busy === 'test' ? $t('autoBackup.testing') : $t('autoBackup.test') }}
        </button>
        <span class="spacer"></span>
        <button v-if="view.settings.schedule !== 'off'" type="button" :disabled="!!busy" @click="runNow">{{ $t('autoBackup.runNow') }}</button>
      </div>
    </form>
  </div>
</template>
