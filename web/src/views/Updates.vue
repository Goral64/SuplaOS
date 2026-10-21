<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, text } from '../api'
import { runJob } from '../components/jobs'

const { t, locale } = useI18n()
const info = ref(null)
const addons = ref([])
const images = ref([])
const imagesChecked = ref(null)
const error = ref('')
const log = ref('')
const following = ref(false)
const pre = ref(null)
let timer = null

async function load() {
  const u = await api('GET', '/updates')
  info.value = u.system
  addons.value = u.addons
  images.value = u.images
  imagesChecked.value = u.imagesChecked
  if (u.system.state === 'running' && !following.value) follow()
}
onMounted(load)
onBeforeUnmount(() => clearTimeout(timer))

const suplaos = computed(() => info.value?.packages.find((p) => p.name === 'suplaos'))
// The changes of a SuplaOS version, in the language of the panel.
const changesURL = (version) => 'https://github.com/Goral64/SuplaOS/blob/main/' +
  (locale.value === 'pl' ? 'docs/pl/' : '') + 'CHANGELOG.md#' + version.replace(/\./g, '')
const others = computed(() => info.value?.packages.filter((p) => p.name !== 'suplaos') || [])
const when = (d) => (d ? new Date(d).toLocaleString(locale.value) : '-')

async function check() {
  error.value = ''
  try {
    await runJob('POST', '/updates/check')
    await load()
  } catch (e) {
    error.value = e.message
  }
}

async function upgrade() {
  if (!confirm(t('updates.confirm'))) return
  error.value = ''
  try {
    await api('POST', '/updates/upgrade')
    follow()
  } catch (e) {
    error.value = e.message
  }
}

// The update may restart the panel: errors while following only mean
// waiting a moment longer.
function follow() {
  following.value = true
  log.value = ''
  let offset = 0
  let quiet = 0
  const poll = async () => {
    try {
      const r = await api('GET', `/updates/log?offset=${offset}`)
      if (r.offset < offset) log.value = ''
      offset = r.offset
      if (r.text) {
        log.value += r.text
        await nextTick()
        if (pre.value) pre.value.scrollTop = pre.value.scrollHeight
      }
      quiet = r.text ? 0 : quiet + 1
      if (r.state !== 'running' && quiet > 1) {
        following.value = false
        await load()
        return
      }
    } catch {
      // panel restarting
    }
    timer = setTimeout(poll, 2000)
  }
  poll()
}

async function applyAddons() {
  error.value = ''
  try {
    await runJob('POST', '/addons/apply')
    await load()
  } catch (e) {
    error.value = e.message
  }
}

async function setAuto(e) {
  error.value = ''
  try {
    const r = await api('PUT', '/updates/auto', { enabled: e.target.checked })
    info.value.auto = r.enabled
  } catch (err) {
    error.value = err.message
  }
}

async function reboot() {
  if (!confirm(t('system.rebootConfirm'))) return
  await api('POST', '/system/reboot')
  error.value = ''
  info.value.rebootRequired = false
  alert(t('system.rebooting'))
}
</script>

<template>
  <h1>{{ $t('updates.title') }}</h1>
  <div v-if="error" class="alert error">{{ error }}</div>
  <div v-if="!info" class="muted">{{ $t('common.loading') }}</div>
  <template v-else>
    <div v-if="info.rebootRequired" class="alert warn row">
      <span>{{ $t('updates.rebootRequired') }}</span>
      <span class="spacer"></span>
      <button @click="reboot">{{ $t('system.reboot') }}</button>
    </div>

    <div class="card">
      <div class="row">
        <h2 style="margin: 0">SuplaOS {{ info.version }}</h2>
        <span class="spacer"></span>
        <button :disabled="following" @click="check">{{ $t('updates.check') }}</button>
      </div>
      <p class="muted small mt">{{ $t('updates.checked', { time: when(info.checked) }) }}</p>
      <p class="muted small">{{ $t('updates.imagesChecked', { time: when(imagesChecked) }) }}</p>

      <p v-if="suplaos" class="alert ok">
        {{ $t('updates.newSuplaos', { version: suplaos.to }) }}
        <a :href="changesURL(suplaos.to)" target="_blank" rel="noopener">{{ $t('addon.changelog') }}</a>
      </p>
      <p v-if="!info.packages.length" class="mt">{{ $t('updates.upToDate') }}</p>
      <template v-else>
        <p class="mt">{{ $t('updates.available', { n: info.packages.length }) }}</p>
        <details v-if="others.length">
          <summary class="muted">{{ $t('updates.packages') }}</summary>
          <table class="mt small">
            <tr v-for="p in others" :key="p.name"><td>{{ p.name }}</td><td class="muted">{{ p.from }} → {{ p.to }}</td></tr>
          </table>
        </details>
      </template>
      <div class="row mt">
        <button class="primary" :disabled="following || !info.packages.length" @click="upgrade">{{ $t('updates.upgrade') }}</button>
        <span v-if="info.state === 'failed' && !following" class="badge bad">{{ $t('updates.failed', { time: when(info.finished) }) }}</span>
        <span v-else-if="info.state === 'done' && !following" class="muted small">{{ $t('updates.done', { time: when(info.finished) }) }}</span>
      </div>
      <div v-if="following || log" class="mt">
        <p v-if="following" class="muted small">{{ $t('updates.running') }}</p>
        <pre ref="pre" class="log">{{ log || '...' }}</pre>
      </div>
    </div>

    <div class="card">
      <h2>{{ $t('updates.autoTitle') }}</h2>
      <label class="check"><input type="checkbox" :checked="info.auto" @change="setAuto"> {{ $t('updates.auto') }}</label>
      <p class="muted small mt">{{ $t('updates.autoHelp') }}</p>
      <p v-if="info.auto" class="muted small">{{ $t('updates.autoLast', { time: when(info.autoLast) }) }}</p>
    </div>

    <div v-if="images.length" class="card">
      <h2>{{ $t('updates.imagesTitle') }}</h2>
      <p>{{ $t('updates.imagesText') }}</p>
      <table class="small">
        <template v-for="a in images" :key="a.id">
          <tr v-for="(img, svc) in a.images" v-show="img.update" :key="svc">
            <td><router-link :to="'/addons/' + a.id">{{ text(a.name, locale) }}</router-link></td>
            <td>{{ svc }}</td>
            <td>{{ img.version || '-' }} → {{ img.newVersion || $t('addon.newImage') }}</td>
            <td><a v-if="img.changelog" :href="img.changelog" target="_blank" rel="noopener">{{ $t('addon.changelog') }}</a></td>
          </tr>
        </template>
      </table>
    </div>

    <div v-if="addons.length" class="card">
      <h2>{{ $t('updates.addonsTitle') }}</h2>
      <p>{{ $t('updates.addonsText', { list: addons.join(', ') }) }}</p>
      <button class="primary" @click="applyAddons">{{ $t('updates.apply') }}</button>
    </div>
  </template>
</template>
