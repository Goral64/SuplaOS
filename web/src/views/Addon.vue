<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { api, formatBytes, imageVersions, text } from '../api'
import { runJob } from '../components/jobs'
import { ask } from '../components/dialog'
import { active, formValues } from '../components/options'
import OptionField from '../components/OptionField.vue'
import StateBadge from '../components/StateBadge.vue'
import LogView from '../components/LogView.vue'
import AccountFields from '../components/AccountFields.vue'
import MigrationCard from '../components/MigrationCard.vue'
import SuplaDevicesCard from '../components/SuplaDevicesCard.vue'

const props = defineProps({ id: String })
const router = useRouter()
const { t, locale } = useI18n()

const all = ref([])
const addon = computed(() => all.value.find((a) => a.id === props.id))
const installedIds = computed(() => all.value.filter((a) => a.installed).map((a) => a.id))
const missing = computed(() => addon.value ? addon.value.requires.filter((r) => !installedIds.value.includes(r)) : [])
const values = ref({})
const visible = computed(() => addon.value
  ? addon.value.options.filter((o) => active(o, values.value, installedIds.value) && !(byConnect.value && connectKeys.includes(o.key)))
  : [])
const name = computed(() => addon.value ? text(addon.value.name, locale.value) : props.id)
const error = ref('')
const showLogs = ref(false)
const purge = ref(false)
const backups = ref([])
const account = ref({ email: '', password: '' })
const showPassword = ref(false)
// Accounts in the Supla database; null while unknown.
const suplaUsers = ref(null)
// Creating accounts here: when there is none yet, or registration is on.
const showAccount = computed(() => addon.value?.id === 'supla' &&
  (suplaUsers.value === 0 || addon.value.values.SUPLA_ACCOUNTS_REGISTRATION_ENABLED === 'true'))

// Logging in to SUPLA Scripts with the Supla Cloud of this device: only
// while Supla runs here.
const suplaRunning = computed(() => {
  const supla = all.value.find((a) => a.id === 'supla')
  return !!supla?.installed && supla.containers.length > 0 && supla.containers.every((c) => c.State === 'running')
})
// Then the OAuth app comes from Connect with Supla Cloud: its settings are
// not in the form, which would also overwrite them.
const connectKeys = ['SUPLA_SCRIPTS_OAUTH_CLOUD', 'SUPLA_SCRIPTS_OAUTH_ID', 'SUPLA_SCRIPTS_OAUTH_SECRET']
// The CA certificate for devices among the downloads of Supla.
const deviceCA = computed(() => addon.value?.downloads.findIndex((d) => d.file === 'ssl/server/ca.crt') ?? -1)
const byConnect = computed(() => addon.value?.id === 'supla-scripts' && suplaRunning.value)
const cloudLogin = ref({ email: '', password: '' })
async function connectCloud() {
  const result = await job('POST', '/supla-scripts/oauth', cloudLogin.value)
  if (result?.state === 'done') cloudLogin.value = { email: '', password: '' }
}

// MQTT ports reachable from the local network (mosquitto add-on).
const mqttPorts = computed(() => {
  const lan = addon.value?.values.MOSQUITTO_LAN
  return { plain: lan === 'plain' || lan === 'both', tls: lan === 'tls' || lan === 'both' }
})
const host = window.location.hostname

// Start only when something is not running, stop and restart only when
// something is.
const allRunning = computed(() => addon.value?.containers.every((c) => c.State === 'running'))
const anyRunning = computed(() => addon.value?.containers.some((c) => ['running', 'restarting', 'paused'].includes(c.State)))

async function load() {
  all.value = await api('GET', '/addons')
  if (addon.value) values.value = formValues(addon.value)
  if (addon.value?.id === 'supla' && addon.value.installed) {
    suplaUsers.value = await api('GET', '/supla/users').then((r) => r.count, () => null)
  }
  if (addon.value?.backup) {
    backups.value = (await api('GET', '/backups')).filter((b) => b.addon === props.id)
  }
}
onMounted(load)
watch(() => props.id, load)

async function job(method, path, body) {
  error.value = ''
  try {
    const result = await runJob(method, path, body)
    await load()
    return result
  } catch (e) {
    error.value = e.message
  }
}

// Only active options are sent; inactive ones keep their stored values.
function activeValues() {
  const v = {}
  for (const o of visible.value) v[o.key] = values.value[o.key] ?? ''
  return v
}

// The first account, only with installing Supla.
const firstAccount = ref({ email: '', password: '', repeat: '' })
function install() {
  const { email, password } = firstAccount.value
  const account = props.id === 'supla' && email ? { email, password } : undefined
  return job('POST', `/addons/${props.id}/install`, { values: activeValues(), account })
}
const saveConfig = () => job('PUT', `/addons/${props.id}/config`, { values: activeValues() })
const action = (a) => job('POST', `/addons/${props.id}/${a}`)

async function remove() {
  const message = t('addon.removeConfirm', { name: name.value }) + (purge.value ? '\n' + t('addon.purgeWarn') : '')
  if (!(await ask(message, { ok: t('addon.remove'), danger: true }))) return
  const result = await job('POST', `/addons/${props.id}/remove`, { purge: purge.value })
  if (result?.state === 'done') router.push('/addons')
}

async function createAccount() {
  const result = await job('POST', '/supla/users', account.value)
  if (result?.state === 'done') account.value = { email: '', password: '' }
}

async function restoreBackup(b) {
  if (!(await ask(t('addon.restoreConfirm', { file: b.name, name: name.value }), { ok: t('addon.restore'), danger: true }))) return
  await job('POST', `/addons/${props.id}/restore`, { name: b.name })
}

const uploading = ref(false)
async function uploadBackup(e) {
  const file = e.target.files[0]
  e.target.value = ''
  if (!file) return
  error.value = ''
  uploading.value = true
  try {
    await api('POST', `/addons/${props.id}/backups?name=${encodeURIComponent(file.name)}`, file)
    await load()
  } catch (err) {
    error.value = err.message
  } finally {
    uploading.value = false
  }
}

async function deleteBackup(b) {
  if (!(await ask(`${t('common.delete')} ${b.name}?`, { ok: t('common.delete'), danger: true }))) return
  await api('DELETE', `/backups/${encodeURIComponent(b.name)}`)
  await load()
}

function webURL(a) {
  return `${a.web.scheme}://${window.location.hostname}:${a.web.port}${a.web.path || '/'}`
}
</script>

<template>
  <p><router-link to="/addons">← {{ $t('addons.title') }}</router-link></p>
  <div v-if="!addon" class="muted">{{ $t('common.loading') }}</div>
  <template v-else>
    <div class="row">
      <h1 style="margin: 0">{{ name }}</h1>
      <span class="spacer"></span>
      <a v-if="addon.web" class="button primary" :href="webURL(addon)" target="_blank" rel="noopener">{{ $t('addons.open') }}</a>
    </div>
    <p class="muted mt">{{ text(addon.description, locale) }}</p>
    <div v-if="error" class="alert error">{{ error }}</div>

    <!-- Not installed: install form -->
    <div v-if="!addon.installed" class="card">
      <h2>{{ $t('addon.installTitle', { name }) }}</h2>
      <div v-if="missing.length" class="alert warn">
        {{ $t('addons.installFirst', { list: '' }) }}
        <template v-for="(r, i) in missing" :key="r"><router-link :to="'/addons/' + r">{{ r }}</router-link>{{ i < missing.length - 1 ? ', ' : '' }}</template>
      </div>
      <form v-else @submit.prevent="install">
        <OptionField v-for="o in visible" :key="o.key" v-model="values[o.key]" :option="o" :installed="false" />
        <AccountFields v-if="addon.id === 'supla'" v-model="firstAccount" />
        <p v-if="addon.recommends.length" class="muted small">{{ $t('addons.worksWith', { list: addon.recommends.join(', ') }) }}</p>
        <button class="primary">{{ $t('addon.install') }}</button>
      </form>
    </div>

    <template v-else>
      <div class="card">
        <h2>{{ $t('addon.status') }}</h2>
        <div class="row">
          <StateBadge v-for="c in addon.containers" :key="c.Service" :container="c" />
          <span v-if="addon.update === 'available'" class="badge warn">{{ $t('addon.updateAvailable') }}</span>
        </div>
        <table v-if="Object.keys(addon.images).length" class="mt small">
          <tr v-for="(img, svc) in addon.images" :key="svc">
            <td>{{ svc }}</td>
            <td>{{ imageVersions(img, t, locale) }}</td>
            <td><a v-if="img.changelog" :href="img.changelog" target="_blank" rel="noopener">{{ $t('addon.changelog') }}</a></td>
          </tr>
        </table>
        <div class="row mt">
          <button :disabled="allRunning" @click="action('start')">{{ $t('addon.start') }}</button>
          <button :disabled="!anyRunning" @click="action('stop')">{{ $t('addon.stop') }}</button>
          <button :disabled="!anyRunning" @click="action('restart')">{{ $t('addon.restart') }}</button>
          <button :disabled="addon.update === 'none'" :title="$t(addon.update === 'none' ? 'addon.upToDate' : 'addon.updateHelp')" @click="action('update')">{{ $t('addon.update') }}</button>
          <span class="spacer"></span>
          <button class="link" @click="showLogs = !showLogs">{{ showLogs ? $t('addon.hideLogs') : $t('addon.showLogs') }}</button>
        </div>
        <div v-if="showLogs" class="mt"><LogView :addon="addon.id" /></div>
      </div>

      <div v-if="addon.id === 'mosquitto'" class="card">
        <h2>{{ $t('addon.connection') }}</h2>
        <table>
          <tr><th>{{ $t('addon.host') }}</th><td><code>{{ host }}</code></td></tr>
          <tr>
            <th>{{ $t('addon.ports') }}</th>
            <td>
              <span v-if="mqttPorts.plain"><code>1883</code> {{ $t('addon.plain') }}</span>
              <span v-if="mqttPorts.plain && mqttPorts.tls">, </span>
              <span v-if="mqttPorts.tls"><code>8883</code> TLS</span>
              <span v-if="!mqttPorts.plain && !mqttPorts.tls" class="muted">{{ $t('addon.noLan') }}</span>
            </td>
          </tr>
          <tr><th>{{ $t('addon.user') }}</th><td><code>{{ addon.values.MOSQUITTO_USERNAME }}</code></td></tr>
          <tr>
            <th>{{ $t('addon.password') }}</th>
            <td>
              <code>{{ showPassword ? addon.values.MOSQUITTO_PASSWORD : '••••••••••••' }}</code>
              <button class="link" @click="showPassword = !showPassword">{{ showPassword ? $t('addon.hide') : $t('addon.show') }}</button>
            </td>
          </tr>
        </table>
        <div v-if="addon.downloads.length" class="row mt">
          <a v-for="(d, i) in addon.downloads" :key="d.file" class="button" :href="`/api/addons/${addon.id}/downloads/${i}`">
            {{ $t('common.download') }}: {{ text(d.label, locale) }}
          </a>
        </div>
        <p v-if="mqttPorts.tls" class="muted small mt">{{ $t('addon.tlsHelp') }}</p>
      </div>

      <div v-else-if="addon.downloads.length && !(addon.id === 'supla' && deviceCA >= 0 && addon.downloads.length === 1)" class="card">
        <h2>{{ $t('addon.files') }}</h2>
        <div class="row">
          <a v-for="(d, i) in addon.downloads" :key="d.file" class="button" :href="`/api/addons/${addon.id}/downloads/${i}`">
            {{ text(d.label, locale) }}
          </a>
        </div>
      </div>

      <div v-if="addon.id === 'supla-scripts' && suplaRunning" class="card">
        <h2>{{ $t('addon.cloudLogin') }}</h2>
        <p class="muted">{{ $t('addon.cloudLoginHelp') }}</p>
        <p v-if="addon.values.SUPLA_SCRIPTS_OAUTH_ID" class="alert ok">{{ $t('addon.cloudLoginDone') }}</p>
        <form class="row" style="align-items: flex-end" @submit.prevent="connectCloud">
          <div class="field" style="flex: 1; min-width: 200px; margin: 0">
            <label for="cl-email">{{ $t('addon.email') }}</label>
            <input id="cl-email" v-model="cloudLogin.email" type="email" required autocomplete="off">
          </div>
          <div class="field" style="flex: 1; min-width: 200px; margin: 0">
            <label for="cl-pass">{{ $t('addon.password') }}</label>
            <input id="cl-pass" v-model="cloudLogin.password" type="password" required autocomplete="new-password">
          </div>
          <button class="primary">{{ $t('addon.cloudConnect') }}</button>
        </form>
      </div>

      <SuplaDevicesCard v-if="addon.id === 'supla' && suplaRunning && deviceCA >= 0"
        :path="`/addons/supla/downloads/${deviceCA}`" :server="addon.values.SUPLA_HOST_ADDRESS" />

      <MigrationCard v-if="addon.id === 'supla' && suplaRunning" />

      <div v-if="showAccount" class="card">
        <h2>{{ $t('addon.account') }}</h2>
        <p class="muted">{{ $t('addon.accountHelp') }}</p>
        <form class="row" style="align-items: flex-end" @submit.prevent="createAccount">
          <div class="field" style="flex: 1; min-width: 200px; margin: 0">
            <label for="acc-email">{{ $t('addon.email') }}</label>
            <input id="acc-email" v-model="account.email" type="email" required autocomplete="off">
          </div>
          <div class="field" style="flex: 1; min-width: 200px; margin: 0">
            <label for="acc-pass">{{ $t('addon.password') }}</label>
            <input id="acc-pass" v-model="account.password" type="password" minlength="8" required autocomplete="new-password">
          </div>
          <button class="primary">{{ $t('addon.createAccount') }}</button>
        </form>
      </div>

      <div class="card">
        <h2>{{ $t('addon.settings') }}</h2>
        <p v-if="!visible.length" class="muted">{{ $t('addon.noSettings') }}</p>
        <form v-else @submit.prevent="saveConfig">
          <OptionField v-for="o in visible" :key="o.key" v-model="values[o.key]" :option="o" :installed="true" />
          <button class="primary">{{ $t('addon.save') }}</button>
        </form>
      </div>

      <div v-if="addon.backup" class="card">
        <div class="row">
          <h2 style="margin: 0">{{ $t('addon.backups') }}</h2>
          <span class="spacer"></span>
          <label class="button" :class="{ disabled: uploading }">
            {{ uploading ? $t('addon.uploading') : $t('addon.upload') }}
            <input type="file" accept=".gz" hidden :disabled="uploading" @change="uploadBackup">
          </label>
          <button @click="action('backup')">{{ $t('addon.backupNow') }}</button>
        </div>
        <p class="muted small mt">{{ $t('addon.backupsHelp') }}</p>
        <p v-if="!backups.length" class="muted mt">{{ $t('addon.noBackups') }}</p>
        <table v-else class="mt stack">
          <tr v-for="b in backups" :key="b.name">
            <td>{{ b.name }}</td>
            <td class="muted">{{ new Date(b.time).toLocaleString(locale) }}</td>
            <td class="muted">{{ formatBytes(b.size) }}</td>
            <td style="text-align: right">
              <button @click="restoreBackup(b)">{{ $t('addon.restore') }}</button>
              <a class="button" :href="'/api/backups/' + encodeURIComponent(b.name)">{{ $t('common.download') }}</a>
              <button class="link danger" @click="deleteBackup(b)">{{ $t('common.delete') }}</button>
            </td>
          </tr>
        </table>
      </div>

      <div class="card">
        <h2>{{ $t('addon.remove') }}</h2>
        <div class="row">
          <label class="check"><input v-model="purge" type="checkbox"> {{ $t('addon.purge') }}</label>
          <span class="spacer"></span>
          <button class="danger" :class="{ filled: purge }" @click="remove">{{ $t('addon.remove') }}</button>
        </div>
      </div>
    </template>
  </template>
</template>
