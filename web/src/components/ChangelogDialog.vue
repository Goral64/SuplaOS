<script setup>
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { builtIn, fetchChangelog, parseChangelog } from '../changelog'

// The changes of SuplaOS. With newer (a version to update to) they come
// from GitHub, so they include that version, and the versions after the
// installed one are marked new.
const props = defineProps({ installed: String, newer: String })
const emit = defineEmits(['close'])
const { locale } = useI18n()

const md = ref(null)
onMounted(async () => {
  md.value = (props.newer && (await fetchChangelog(props.newer, locale.value))) || builtIn(locale.value)
})
const sections = computed(() => {
  if (!md.value) return []
  const list = parseChangelog(md.value, locale.value)
  const i = list.findIndex((s) => s.version === props.installed)
  return list.map((s, n) => ({ ...s, current: n === i, new: i > 0 && n < i }))
})
</script>

<template>
  <div class="modal-back" @keydown.esc="emit('close')">
    <div class="modal" role="dialog">
      <div class="row">
        <h2 style="margin: 0">{{ $t('changelog.title') }}</h2>
        <span class="spacer"></span>
        <button @click="emit('close')">{{ $t('common.close') }}</button>
      </div>
      <p v-if="!md" class="muted mt">{{ $t('common.loading') }}</p>
      <div v-for="s in sections" :key="s.version" class="changelog mt">
        <h3>
          {{ s.version }}
          <span v-if="s.new" class="badge ok">{{ $t('changelog.new') }}</span>
          <span v-if="s.current" class="badge">{{ $t('changelog.installed') }}</span>
        </h3>
        <div v-html="s.html"></div>
      </div>
    </div>
  </div>
</template>
