<script setup>
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
import { ask } from './dialog'

// In the setup wizard the card is a part of a step: no frame, and nothing
// at all without a Wi-Fi card.
const props = defineProps({ embedded: Boolean })

const { t } = useI18n()
const wifi = ref(null)
const networks = ref(null)
// The network being joined: { ssid, security, hidden }.
const join = ref(null)
const password = ref('')
const error = ref('')
const message = ref('')
const busy = ref(false)

async function load() {
  wifi.value = await api('GET', '/wifi')
}
onMounted(load)

async function call(fn) {
  busy.value = true
  error.value = message.value = ''
  try {
    await fn()
  } catch (e) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}

const scan = () => call(async () => {
  networks.value = await api('POST', '/wifi/scan')
})

function pick(n) {
  join.value = n ? { ssid: n.ssid, security: n.security, hidden: false } : { ssid: '', security: 'WPA2', hidden: true }
  password.value = ''
}

const connect = () => call(async () => {
  const { ssid, hidden } = join.value
  const res = await api('POST', '/wifi/connect', { ssid, password: password.value, hidden })
  join.value = null
  networks.value = null
  password.value = ''
  if (res.async) {
    message.value = t('wifi.connectingAsync', { ssid })
    return
  }
  await load()
  message.value = t('wifi.connected', { ssid, address: wifi.value.address })
})

const forget = (name) => call(async () => {
  if (!(await ask(t('wifi.forgetConfirm', { name }), { ok: t('wifi.forget'), danger: true }))) return
  const res = await api('DELETE', '/wifi/' + encodeURIComponent(name))
  if (res.async) {
    message.value = t('wifi.forgetAsync')
    return
  }
  await load()
})

const signalClass = (s) => (s >= 60 ? 'ok' : s >= 35 ? 'warn' : 'bad')
</script>

<template>
  <div v-if="wifi && (wifi.device || !props.embedded)" :class="{ card: !props.embedded }">
    <h2 v-if="!props.embedded">{{ $t('wifi.title') }}</h2>
    <template v-else>
      <h3>{{ $t('wifi.setupTitle') }}</h3>
      <p class="muted small">{{ $t('wifi.setupIntro') }}</p>
    </template>
    <div v-if="error" class="alert error">{{ error }}</div>
    <div v-if="message" class="alert ok">{{ message }}</div>

    <p v-if="!wifi.device" class="muted">{{ $t('wifi.noDevice') }}</p>
    <template v-else>
      <p class="muted">
        {{ wifi.device }} ·
        <template v-if="wifi.connection">{{ $t('wifi.connectedTo', { ssid: wifi.connection, address: wifi.address || '-' }) }}</template>
        <template v-else>{{ $t('wifi.notConnected') }}</template>
      </p>

      <form v-if="join" @submit.prevent="connect">
        <div v-if="join.hidden" class="field">
          <label for="ssid" class="required">{{ $t('wifi.ssid') }}</label>
          <input id="ssid" v-model="join.ssid" maxlength="32" required>
        </div>
        <p v-else>{{ $t('wifi.joining', { ssid: join.ssid }) }}</p>
        <div v-if="join.security || join.hidden" class="field">
          <label for="wifi-password" :class="{ required: !join.hidden }">{{ $t('wifi.password') }}</label>
          <input id="wifi-password" v-model="password" type="password" autocomplete="new-password"
            minlength="8" maxlength="64" :required="!join.hidden">
          <div v-if="join.hidden" class="help">{{ $t('wifi.passwordHelp') }}</div>
        </div>
        <div class="row">
          <button type="button" :disabled="busy" @click="join = null">{{ $t('common.cancel') }}</button>
          <button class="primary" :disabled="busy">{{ busy ? $t('wifi.connecting') : $t('wifi.connect') }}</button>
        </div>
      </form>

      <template v-else>
        <table v-if="networks" class="mt">
          <tr v-for="n in networks" :key="n.ssid">
            <td>{{ n.ssid }} <span v-if="n.inUse" class="badge ok">{{ $t('wifi.inUse') }}</span></td>
            <td class="small muted">{{ n.security || $t('wifi.open') }}</td>
            <td><span class="badge" :class="signalClass(n.signal)">{{ n.signal }}%</span></td>
            <td><button v-if="!n.inUse" :disabled="busy" @click="pick(n)">{{ $t('wifi.connect') }}</button></td>
          </tr>
          <tr v-if="!networks.length"><td class="muted">{{ $t('wifi.none') }}</td></tr>
        </table>
        <div class="row mt">
          <button :disabled="busy" @click="scan">{{ busy ? $t('wifi.scanning') : $t('wifi.scan') }}</button>
          <button class="link" :disabled="busy" @click="pick(null)">{{ $t('wifi.hidden') }}</button>
        </div>
      </template>

      <template v-if="wifi.saved.length && !props.embedded">
        <h3>{{ $t('wifi.saved') }}</h3>
        <table>
          <tr v-for="name in wifi.saved" :key="name">
            <td>{{ name }} <span v-if="name === wifi.connection" class="badge ok">{{ $t('wifi.inUse') }}</span></td>
            <td><button class="danger" :disabled="busy" @click="forget(name)">{{ $t('wifi.forget') }}</button></td>
          </tr>
        </table>
      </template>
    </template>
  </div>
</template>
