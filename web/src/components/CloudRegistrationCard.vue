<script setup>
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
import { ask } from './dialog'
import { runJob } from './jobs'

// Registration of the Supla Cloud of this device at cloud.supla.org.
const { t, te } = useI18n()
const view = ref(null)
const token = ref('')
const error = ref('')
const busy = ref(false)
const checking = ref(false)

async function load() {
  checking.value = true
  try {
    view.value = await api('GET', '/supla/registration')
    error.value = ''
  } catch (e) {
    error.value = e.message
  } finally {
    checking.value = false
  }
}
onMounted(load)

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

const register = () => call(async () => {
  const result = await runJob('POST', '/supla/registration', { token: token.value })
  if (result?.state === 'done') token.value = ''
  await load()
})

const forget = () => call(async () => {
  if (!(await ask(t('cloudRegistration.forgetConfirm'), { ok: t('cloudRegistration.forget'), danger: true }))) return
  await api('DELETE', '/supla/registration')
  await load()
})

const badge = { ok: 'ok', warn: 'warn', fail: 'bad' }
function checkText(c) {
  const key = `cloudRegistration.checks.${c.id}.${c.reason || 'ok'}`
  return te(key) ? t(key, { detail: c.detail || '' }) : `${c.id}: ${c.reason || c.state} ${c.detail || ''}`
}
</script>

<template>
  <div v-if="view || error" class="card">
    <h2>{{ $t('cloudRegistration.title') }}</h2>
    <div v-if="error" class="alert error">{{ error }}</div>

    <template v-if="view?.registered">
      <p class="alert ok">{{ $t('cloudRegistration.registered', { address: view.address }) }}</p>
      <p v-if="view.notifications">
        {{ view.notifications === 'true'
          ? $t('cloudRegistration.notificationsOn', { limit: view.limit || '0' })
          : $t('cloudRegistration.notificationsOff') }}
      </p>
      <p class="muted small">{{ $t('cloudRegistration.registeredHelp') }}</p>
      <button type="button" class="link danger" :disabled="busy" @click="forget">{{ $t('cloudRegistration.forget') }}</button>
    </template>

    <template v-else-if="view">
      <p>{{ $t('cloudRegistration.intro') }}</p>
      <h3>{{ $t('cloudRegistration.conditions', { address: view.address }) }}</h3>
      <ul class="checks">
        <li v-for="c in view.checks" :key="c.id">
          <span class="badge" :class="badge[c.state]">{{ $t('cloudRegistration.state.' + c.state) }}</span>
          {{ checkText(c) }}
        </li>
      </ul>
      <p class="muted small">{{ $t('cloudRegistration.conditionsHelp') }}</p>
      <div class="row">
        <button type="button" :disabled="checking || busy" @click="load">
          {{ checking ? $t('cloudRegistration.checking') : $t('cloudRegistration.checkAgain') }}
        </button>
      </div>

      <form v-if="view.ready" class="mt" @submit.prevent="register">
        <p>
          {{ $t('cloudRegistration.step1') }}
          <a :href="view.registerURL" target="_blank" rel="noopener">cloud.supla.org/register-cloud</a>
        </p>
        <div class="field">
          <label for="reg-token" class="required">{{ $t('cloudRegistration.token') }}</label>
          <textarea id="reg-token" v-model="token" rows="2" required spellcheck="false"></textarea>
          <div class="help">{{ $t('cloudRegistration.tokenHelp') }}</div>
        </div>
        <button class="primary" :disabled="busy">{{ $t('cloudRegistration.register') }}</button>
      </form>
      <p v-else class="alert warn mt">{{ $t('cloudRegistration.notReady') }}</p>
    </template>
  </div>
</template>

<style scoped>
.checks { list-style: none; padding: 0; margin: .5rem 0; }
.checks li { margin: .35rem 0; }
.checks .badge { margin-right: .4rem; min-width: 4.5rem; text-align: center; }
</style>
