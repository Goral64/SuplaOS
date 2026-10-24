<script setup>
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, text } from '../api'
import { ask } from './dialog'
import { runJob } from './jobs'

const { t, locale } = useI18n()
const view = ref(null)
const form = ref(null)
const error = ref('')
const busy = ref(false)

const help = computed(() => 'https://github.com/Goral64/SuplaOS/blob/main/docs/' +
  (locale.value === 'pl' ? 'pl/' : '') + 'domain.md')
const provider = computed(() => view.value?.providers.find((p) => p.name === form.value?.provider))
const when = (d) => (d ? new Date(d).toLocaleString(locale.value) : '')

async function load() {
  view.value = await api('GET', '/domain')
}
onMounted(load)

function edit() {
  const v = view.value
  form.value = {
    domain: v.domain,
    provider: v.provider || 'cloudflare',
    fields: { ...v.fields },
    email: v.email,
    staging: v.staging,
  }
}

// A saved secret is not shown: left empty it stays as it is, for the same
// provider only.
const saved = (f) => form.value.provider === view.value.provider && view.value.secrets.includes(f.env)

async function job(method, path, body) {
  busy.value = true
  error.value = ''
  try {
    const result = await runJob(method, path, body)
    await load()
    return result
  } catch (e) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}

async function save() {
  const fields = {}
  for (const f of provider.value.fields) fields[f.env] = form.value.fields[f.env] || ''
  const result = await job('PUT', '/domain', { ...form.value, fields })
  if (result && result.state === 'done') form.value = null
}

const renew = () => job('POST', '/domain/renew')

async function disable() {
  if (!(await ask(t('domain.disableConfirm'), { ok: t('domain.disable'), danger: true }))) return
  await job('DELETE', '/domain')
  form.value = null
}
</script>

<template>
  <div v-if="view" class="card">
    <h2>{{ $t('domain.title') }}</h2>
    <div v-if="error" class="alert error">{{ error }}</div>

    <template v-if="!form">
      <template v-if="view.status">
        <p><code>{{ view.status.domain }}</code></p>
        <p v-if="view.status.notAfter" class="muted small">
          {{ $t('domain.issued', { issuer: view.status.issuer, date: when(view.status.notAfter) }) }}
        </p>
        <p v-else class="muted small">{{ $t('domain.none') }}</p>
        <div v-if="view.status.lastError" class="alert error">
          {{ $t('domain.failed', { date: when(view.status.lastAttempt) }) }}<br>
          <code>{{ view.status.lastError }}</code>
          <template v-if="view.status.nextAttempt"><br>{{ $t('domain.next', { date: when(view.status.nextAttempt) }) }}</template>
        </div>
        <div class="row">
          <button :disabled="busy" @click="renew">{{ $t('domain.renew') }}</button>
          <button :disabled="busy" @click="edit">{{ $t('domain.change') }}</button>
          <button class="danger" :disabled="busy" @click="disable">{{ $t('domain.disable') }}</button>
        </div>
      </template>
      <template v-else>
        <p>{{ $t('domain.intro') }}</p>
        <div class="row">
          <button class="primary" @click="edit">{{ $t('domain.set') }}</button>
          <a :href="help" target="_blank" rel="noopener">{{ $t('domain.how') }}</a>
        </div>
      </template>
    </template>

    <form v-else @submit.prevent="save">
      <div class="field">
        <label for="domain" class="required">{{ $t('domain.domain') }}</label>
        <input id="domain" v-model="form.domain" required placeholder="supla.example.com" autocomplete="off">
        <div class="help">{{ $t('domain.domainHelp') }}</div>
      </div>
      <div class="field">
        <label for="provider">{{ $t('domain.provider') }}</label>
        <select id="provider" v-model="form.provider">
          <option v-for="p in view.providers" :key="p.name" :value="p.name">{{ p.label }}</option>
        </select>
        <div class="help">
          {{ $t('domain.providerHelp') }}
          <a :href="help" target="_blank" rel="noopener">{{ $t('domain.how') }}</a>
        </div>
      </div>
      <div v-for="f in provider.fields" :key="f.env" class="field">
        <label :for="f.env" :class="{ required: !f.optional && !saved(f) }">{{ text(f.label, locale) }}</label>
        <input :id="f.env" v-model="form.fields[f.env]" :type="f.secret ? 'password' : 'text'"
          :required="!f.optional && !saved(f)" :placeholder="saved(f) ? $t('domain.savedSecret') : f.default"
          autocomplete="new-password">
      </div>
      <div class="field">
        <label for="email">{{ $t('domain.email') }} ({{ $t('common.optional') }})</label>
        <input id="email" v-model="form.email" type="email" autocomplete="off">
        <div class="help">{{ $t('domain.emailHelp') }}</div>
      </div>
      <div class="field">
        <label class="check"><input v-model="form.staging" type="checkbox"> {{ $t('domain.staging') }}</label>
      </div>
      <div class="row">
        <button class="primary" :disabled="busy">{{ $t('domain.save') }}</button>
        <button type="button" :disabled="busy" @click="form = null">{{ $t('common.cancel') }}</button>
      </div>
    </form>
  </div>
</template>
