<script setup>
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, session } from '../api'
import { ask } from './dialog'

const { t } = useI18n()
const keys = ref(null)
const key = ref('')
const error = ref('')
const busy = ref(false)

async function load() {
  keys.value = await api('GET', '/ssh-keys')
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

const add = () => call(async () => {
  await api('POST', '/ssh-keys', { key: key.value })
  key.value = ''
  await load()
})

const remove = (k) => call(async () => {
  if (!(await ask(t('sshKeys.deleteConfirm', { name: k.comment || k.fingerprint }), { ok: t('common.delete'), danger: true }))) return
  await api('POST', '/ssh-keys/delete', { fingerprint: k.fingerprint })
  await load()
})
</script>

<template>
  <div v-if="keys" class="card">
    <h2>{{ $t('sshKeys.title') }}</h2>
    <p>{{ $t('sshKeys.intro', { user: session.user }) }}</p>
    <div v-if="error" class="alert error">{{ error }}</div>
    <p v-if="!keys.length" class="muted">{{ $t('sshKeys.none') }}</p>
    <table v-else>
      <tr v-for="k in keys" :key="k.fingerprint">
        <td>
          {{ k.comment || $t('sshKeys.noComment') }} <span class="badge">{{ k.type }}</span>
          <div class="muted small fingerprint">{{ k.fingerprint }}</div>
        </td>
        <td style="text-align: right">
          <button class="link danger" :disabled="busy" @click="remove(k)">{{ $t('common.delete') }}</button>
        </td>
      </tr>
    </table>
    <form class="mt" @submit.prevent="add">
      <div class="field">
        <label for="ssh-key">{{ $t('sshKeys.add') }}</label>
        <textarea id="ssh-key" v-model="key" rows="3" required spellcheck="false" placeholder="ssh-ed25519 AAAA... me@laptop"></textarea>
        <div class="help">{{ $t('sshKeys.help') }}</div>
      </div>
      <button class="primary" :disabled="busy">{{ $t('sshKeys.addButton') }}</button>
    </form>
  </div>
</template>
