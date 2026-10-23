<script setup>
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { api, panelUrl, session } from '../api'
import { languages, setLanguage } from '../i18n'
import { runJob } from '../components/jobs'

const router = useRouter()
const { locale } = useI18n()
const step = ref(1)
const lang = ref(locale.value)
const timezone = ref('')
const hostname = ref('')
const timezones = ref([])
const error = ref('')
const busy = ref(false)

onMounted(async () => {
  const s = await api('GET', '/settings')
  timezone.value = s.timezone
  hostname.value = s.hostname
  timezones.value = await api('GET', '/timezones')
})

async function save(body, nextStep) {
  busy.value = true
  error.value = ''
  try {
    await api('PUT', '/settings', body)
    step.value = nextStep
  } catch (e) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}

function saveLanguage() {
  setLanguage(lang.value)
  session.lang = lang.value
  save({ lang: lang.value }, 2)
}

async function saveSystem() {
  await save({ timezone: timezone.value, hostname: hostname.value }, 3)
  // Opened by the old .local name: move to the new one (log in again there).
  const here = window.location.hostname
  if (!error.value && here.endsWith('.local') && here !== hostname.value + '.local') {
    window.location.href = `${panelUrl(hostname.value + '.local')}/setup`
  }
}

async function finish(installSupla) {
  error.value = ''
  if (installSupla) {
    try {
      const result = await runJob('POST', '/addons/supla/install', { values: {} })
      if (result.state !== 'done') return
    } catch (e) {
      error.value = e.message
      return
    }
  }
  await api('PUT', '/settings', { setupDone: true })
  session.setupDone = true
  router.push(installSupla ? '/addons/supla' : '/')
}
</script>

<template>
  <div class="narrow card">
    <h1>{{ $t('setup.title') }}</h1>
    <div class="steps"><span v-for="n in 3" :key="n" :class="{ done: n <= step }"></span></div>
    <div v-if="error" class="alert error">{{ error }}</div>

    <form v-if="step === 1" @submit.prevent="saveLanguage">
      <p>{{ $t('setup.langIntro') }}</p>
      <div class="field">
        <label v-for="(name, code) in languages" :key="code" class="check">
          <input v-model="lang" type="radio" :value="code" @change="setLanguage(code)"> {{ name }}
        </label>
      </div>
      <button class="primary" :disabled="busy">{{ $t('common.next') }}</button>
    </form>

    <form v-else-if="step === 2" @submit.prevent="saveSystem">
      <p>{{ $t('setup.systemIntro') }}</p>
      <div class="field">
        <label for="tz">{{ $t('setup.timezone') }}</label>
        <select id="tz" v-model="timezone">
          <option v-for="z in timezones" :key="z" :value="z">{{ z }}</option>
        </select>
      </div>
      <div class="field">
        <label for="host">{{ $t('setup.hostname') }}</label>
        <input id="host" v-model="hostname" pattern="[a-z0-9]([a-z0-9-]*[a-z0-9])?" required>
        <div class="help">{{ $t('setup.hostnameHelp', { name: hostname }) }}</div>
      </div>
      <div class="row">
        <button type="button" @click="step = 1">{{ $t('common.back') }}</button>
        <button class="primary" :disabled="busy">{{ $t('common.next') }}</button>
      </div>
    </form>

    <div v-else>
      <p>{{ $t('setup.suplaIntro') }}</p>
      <div class="row">
        <button @click="step = 2">{{ $t('common.back') }}</button>
        <span class="spacer"></span>
        <button @click="finish(false)">{{ $t('setup.skip') }}</button>
        <button class="primary" @click="finish(true)">{{ $t('setup.installSupla') }}</button>
      </div>
    </div>
  </div>
</template>
