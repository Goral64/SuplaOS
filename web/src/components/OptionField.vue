<script setup>
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, text } from '../api'

const props = defineProps({ option: Object, installed: Boolean })
const model = defineModel()
const { locale } = useI18n()

const label = computed(() => text(props.option.label, locale.value) || props.option.key)
const help = computed(() => text(props.option.help, locale.value))
const reveal = ref(false)
const inputType = computed(() => {
  if (props.option.type === 'password') return reveal.value ? 'text' : 'password'
  return { email: 'email', port: 'number' }[props.option.type] || 'text'
})
const choiceLabel = (c) => text(props.option.choiceLabels?.[c], locale.value) || c

const devices = ref([])
const other = ref(false)
async function loadDevices() {
  devices.value = await api('GET', '/serial')
  if (model.value && !devices.value.some((d) => d.path === model.value)) other.value = true
}
onMounted(() => {
  if (props.option.type === 'serial') loadDevices()
})
function pickDevice(v) {
  if (v === '__other') {
    other.value = true
    model.value = ''
  } else {
    model.value = v
  }
}
</script>

<template>
  <div class="field">
    <label :for="option.key" :class="{ required: option.required }">{{ label }}</label>
    <select v-if="option.type === 'select'" :id="option.key" v-model="model">
      <option v-for="c in option.choices" :key="c" :value="c">{{ choiceLabel(c) }}</option>
    </select>
    <template v-else-if="option.type === 'serial'">
      <div class="row">
        <select v-if="!other" :id="option.key" :value="model" style="flex: 1" @change="pickDevice($event.target.value)">
          <option value="" disabled>{{ $t('addon.chooseDevice') }}</option>
          <option v-for="d in devices" :key="d.path" :value="d.path">{{ d.path }}</option>
          <option value="__other">{{ $t('addon.otherDevice') }}</option>
        </select>
        <input v-else :id="option.key" v-model="model" placeholder="/dev/serial/by-id/..." style="flex: 1">
        <button type="button" @click="loadDevices">{{ $t('common.refresh') }}</button>
      </div>
      <div v-if="!devices.length" class="help">{{ $t('addon.noDevices') }}</div>
    </template>
    <div v-else-if="option.type === 'password'" class="row">
      <!-- new-password: browsers ignore off next to a password and would put a
           saved login into this and the field before it. -->
      <input :id="option.key" v-model="model" :type="inputType" :required="option.required && !option.generate" autocomplete="new-password" style="flex: 1">
      <button type="button" @click="reveal = !reveal">{{ reveal ? $t('addon.hide') : $t('addon.show') }}</button>
    </div>
    <input v-else :id="option.key" v-model="model" :type="inputType" :required="option.required" autocomplete="off">
    <div v-if="help" class="help">{{ help }}</div>
    <div v-if="option.generate" class="help">{{ $t('addon.generated') }}</div>
  </div>
</template>
