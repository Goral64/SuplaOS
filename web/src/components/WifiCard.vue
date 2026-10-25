<script setup>
import { onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
import { ask } from './dialog'

// In the setup wizard the card is a part of a step: no frame, and nothing
// at all without a Wi-Fi card.
const props = defineProps({ embedded: Boolean })

const { t } = useI18n()
const wifi = ref(null)
const networks = ref(null)
// The network being joined: { ssid, security, hidden, autoconnect }.
const join = ref(null)
const password = ref('')
const error = ref('')
const message = ref('')
// What is going on: scan, connect... or empty.
const busy = ref('')
// The name of the device, for the address the phone finds it at later.
const host = ref('suplaos')

async function load() {
  wifi.value = await api('GET', '/wifi')
  if (wifi.value.hotspot) host.value = (await api('GET', '/settings').catch(() => ({}))).hostname || host.value
}

// The state is read again every 10 s, after a change made in the
// background every 3 s for a while: the panel shows the new connection
// without reloading the page.
let timer
let fast = 0
let waitFor = null
function schedule() {
  clearTimeout(timer)
  timer = setTimeout(tick, fast > 0 ? 3000 : 10000)
}
async function tick() {
  try {
    await load()
    if (waitFor && wifi.value.state === 'connected' && wifi.value.connection === waitFor) {
      message.value = t('wifi.connected', { ssid: waitFor, address: wifi.value.address })
      waitFor = null
      fast = 0
    }
  } catch {
    // Not reachable for a moment while the network changes.
  }
  if (fast > 0) fast--
  schedule()
}
onMounted(async () => {
  await load()
  schedule()
})
onUnmounted(() => clearTimeout(timer))

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

// After a change: when the panel is opened over Wi-Fi the answer comes
// before the change is made.
async function done(res, asyncText, text, ssid) {
  if (res.async) {
    message.value = asyncText
    waitFor = ssid || null
    fast = 20
    schedule()
    return
  }
  await load()
  if (text) message.value = text()
}

const scan = () => call('scan', async () => {
  networks.value = await api('POST', '/wifi/scan')
})

function pick(n) {
  join.value = n
    ? { ssid: n.ssid, security: n.security, hidden: false, autoconnect: true }
    : { ssid: '', security: 'WPA2', hidden: true, autoconnect: true }
  password.value = ''
}

const connected = (ssid) => () => t('wifi.connected', { ssid, address: wifi.value.address })
// Over the hotspot the phone loses the panel when the device joins a
// network: it has to go back to that network.
const connectingAsync = (ssid) => t(wifi.value.hotspot ? 'wifi.connectingHotspot' : 'wifi.connectingAsync', { ssid, hotspot: wifi.value.hotspot, host: host.value })

const connect = () => call('connect', async () => {
  const { ssid, hidden, autoconnect } = join.value
  const res = await api('POST', '/wifi/connect', { ssid, password: password.value, hidden, autoconnect })
  join.value = null
  networks.value = null
  password.value = ''
  await done(res, connectingAsync(ssid), connected(ssid), ssid)
})

const saved = (name) => '/wifi/saved/' + encodeURIComponent(name)

const connectSaved = (name) => call('connect', async () => {
  const res = await api('POST', saved(name) + '/connect')
  await done(res, connectingAsync(name), connected(name), name)
})

const disconnect = () => call('disconnect', async () => {
  if (!(await ask(t('wifi.disconnectConfirm', { ssid: wifi.value.connection }), { ok: t('wifi.disconnect') }))) return
  const res = await api('POST', '/wifi/disconnect')
  await done(res, t('wifi.disconnectAsync'))
})

const setAutoconnect = (n, on) => call('autoconnect', async () => {
  try {
    await api('PUT', saved(n.name), { autoconnect: on })
  } finally {
    await load()
  }
})

const forget = (name) => call('forget', async () => {
  if (!(await ask(t('wifi.forgetConfirm', { name }), { ok: t('wifi.forget'), danger: true }))) return
  const res = await api('DELETE', saved(name))
  await done(res, t('wifi.forgetAsync'))
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
      <div class="row">
        <p class="muted">
          {{ wifi.device }} ·
          <template v-if="wifi.connection">{{ $t('wifi.connectedTo', { ssid: wifi.connection, address: wifi.address || '-' }) }}</template>
          <template v-else-if="wifi.hotspot">{{ $t('wifi.hotspotOn', { ssid: wifi.hotspot }) }}</template>
          <template v-else>{{ $t('wifi.notConnected') }}</template>
        </p>
        <span class="spacer"></span>
        <button v-if="wifi.connection" :disabled="!!busy" @click="disconnect">
          {{ busy === 'disconnect' ? $t('wifi.disconnecting') : $t('wifi.disconnect') }}
        </button>
      </div>

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
        <div class="field">
          <label class="check"><input v-model="join.autoconnect" type="checkbox"> {{ $t('wifi.autoconnect') }}</label>
          <div class="help">{{ $t('wifi.autoconnectHelp') }}</div>
        </div>
        <div class="row">
          <button type="button" :disabled="!!busy" @click="join = null">{{ $t('common.cancel') }}</button>
          <button class="primary" :disabled="!!busy">{{ busy === 'connect' ? $t('wifi.connecting') : $t('wifi.connect') }}</button>
        </div>
      </form>

      <template v-else>
        <table v-if="networks" class="mt">
          <tr v-for="n in networks" :key="n.ssid">
            <td>{{ n.ssid }} <span v-if="n.inUse" class="badge ok">{{ $t('wifi.inUse') }}</span></td>
            <td class="small muted">{{ n.security || $t('wifi.open') }}</td>
            <td><span class="badge" :class="signalClass(n.signal)">{{ n.signal }}%</span></td>
            <td><button v-if="!n.inUse" :disabled="!!busy" @click="pick(n)">{{ $t('wifi.connect') }}</button></td>
          </tr>
          <tr v-if="!networks.length"><td class="muted">{{ $t('wifi.none') }}</td></tr>
        </table>
        <div class="row mt">
          <button :disabled="!!busy" @click="scan">{{ busy === 'scan' ? $t('wifi.scanning') : $t('wifi.scan') }}</button>
          <button class="link" :disabled="!!busy" @click="pick(null)">{{ $t('wifi.hidden') }}</button>
        </div>
      </template>

      <template v-if="wifi.saved.length && !props.embedded">
        <h3>{{ $t('wifi.saved') }}</h3>
        <table>
          <tr v-for="n in wifi.saved" :key="n.name">
            <td>{{ n.name }} <span v-if="n.name === wifi.connection" class="badge ok">{{ $t('wifi.inUse') }}</span></td>
            <td>
              <label class="check small">
                <input type="checkbox" :checked="n.autoconnect" :disabled="!!busy" @change="setAutoconnect(n, $event.target.checked)">
                {{ $t('wifi.autoconnect') }}
              </label>
            </td>
            <td>
              <div class="row">
                <button v-if="n.name !== wifi.connection" :disabled="!!busy" @click="connectSaved(n.name)">{{ $t('wifi.connect') }}</button>
                <button class="danger" :disabled="!!busy" @click="forget(n.name)">{{ $t('wifi.forget') }}</button>
              </div>
            </td>
          </tr>
        </table>
      </template>
    </template>
  </div>
</template>
