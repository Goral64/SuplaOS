<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { api, panelUrl } from '../api'
import { ask } from '../components/dialog'

const { t, locale } = useI18n()
const router = useRouter()
const settings = ref({ timezone: '', hostname: '' })
const timezones = ref([])
const net = ref(null)
const ca = ref(null)
// How to install the CA certificate, in the language of the panel.
const certificateHelp = computed(() => 'https://github.com/Goral64/SuplaOS/blob/main/docs/' +
  (locale.value === 'pl' ? 'pl/' : '') + 'certificate.md')
const dns = ref('')
const message = ref('')
const error = ref('')
const busy = ref(false)

onMounted(async () => {
  settings.value = await api('GET', '/settings')
  timezones.value = await api('GET', '/timezones')
  net.value = await api('GET', '/network')
  dns.value = (net.value.dns || []).join(', ')
  ca.value = await api('GET', '/ca')
})

async function call(fn) {
  busy.value = true
  message.value = error.value = ''
  try {
    await fn()
  } catch (e) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}

const saveGeneral = () => call(async () => {
  settings.value = await api('PUT', '/settings', { timezone: settings.value.timezone, hostname: settings.value.hostname })
  message.value = t('common.saved')
})

// Address the panel will have after the network change.
const newAddress = computed(() => {
  if (!net.value) return ''
  if (net.value.method === 'manual') return panelUrl(net.value.address.split('/')[0])
  return panelUrl(`${settings.value.hostname}.local`)
})

const saveNetwork = () => call(async () => {
  const body = { ...net.value, dns: dns.value.split(/[\s,]+/).filter(Boolean) }
  if (!(await ask(t('system.networkWarn', { address: newAddress.value }), { ok: t('common.save') }))) return
  await api('PUT', '/network', body)
  message.value = t('system.networkWarn', { address: newAddress.value })
})

const power = (action) => call(async () => {
  if (!(await ask(t(action === 'reboot' ? 'system.rebootConfirm' : 'system.poweroffConfirm'),
    { ok: t(action === 'reboot' ? 'system.reboot' : 'system.poweroff'), danger: action === 'poweroff' }))) return
  const { boot } = await api('POST', '/system/' + action)
  router.push({ path: '/power/' + action, query: { boot } })
})
</script>

<template>
  <h1>{{ $t('system.title') }}</h1>
  <div v-if="error" class="alert error">{{ error }}</div>
  <div v-if="message" class="alert ok">{{ message }}</div>

  <div class="card">
    <h2>{{ $t('system.general') }}</h2>
    <form @submit.prevent="saveGeneral">
      <div class="field">
        <label for="tz">{{ $t('system.timezone') }}</label>
        <select id="tz" v-model="settings.timezone">
          <option v-for="z in timezones" :key="z" :value="z">{{ z }}</option>
        </select>
      </div>
      <div class="field">
        <label for="host">{{ $t('system.hostname') }}</label>
        <input id="host" v-model="settings.hostname" pattern="[a-z0-9]([a-z0-9-]*[a-z0-9])?" required>
        <div class="help">{{ $t('setup.hostnameHelp', { name: settings.hostname }) }}</div>
      </div>
      <button class="primary" :disabled="busy">{{ $t('common.save') }}</button>
    </form>
  </div>

  <div v-if="net" class="card">
    <h2>{{ $t('system.network') }}</h2>
    <p class="muted">{{ net.device }} · {{ $t('system.current', { address: net.address }) }}</p>
    <form @submit.prevent="saveNetwork">
      <div class="field">
        <label class="check"><input v-model="net.method" type="radio" value="auto"> {{ $t('system.dhcp') }}</label>
        <label class="check"><input v-model="net.method" type="radio" value="manual"> {{ $t('system.static') }}</label>
      </div>
      <template v-if="net.method === 'manual'">
        <div class="field">
          <label for="addr" class="required">{{ $t('system.address') }}</label>
          <input id="addr" v-model="net.address" required>
          <div class="help">{{ $t('system.addressHelp') }}</div>
        </div>
        <div class="field">
          <label for="gw">{{ $t('system.gateway') }}</label>
          <input id="gw" v-model="net.gateway">
        </div>
        <div class="field">
          <label for="dns">{{ $t('system.dns') }}</label>
          <input id="dns" v-model="dns">
          <div class="help">{{ $t('system.dnsHelp') }}</div>
        </div>
      </template>
      <button class="primary" :disabled="busy">{{ $t('system.apply') }}</button>
    </form>
  </div>

  <div v-if="ca" class="card">
    <h2>{{ $t('system.certificate') }}</h2>
    <p>{{ $t('system.certificateIntro') }}</p>
    <p class="muted small">{{ $t('system.fingerprint') }}<br><code class="fingerprint">{{ ca.sha256 }}</code></p>
    <div class="row">
      <a class="button" href="/api/ca.crt" download>{{ $t('system.downloadCA') }}</a>
      <a :href="certificateHelp" target="_blank" rel="noopener">{{ $t('system.certificateHow') }}</a>
    </div>
  </div>

  <div class="card">
    <h2>{{ $t('system.password') }}</h2>
    <router-link to="/password" class="button">{{ $t('system.changePassword') }}</router-link>
  </div>

  <div class="card">
    <h2>{{ $t('system.power') }}</h2>
    <div class="row">
      <button :disabled="busy" @click="power('reboot')">{{ $t('system.reboot') }}</button>
      <button class="danger" :disabled="busy" @click="power('poweroff')">{{ $t('system.poweroff') }}</button>
    </div>
  </div>
</template>
