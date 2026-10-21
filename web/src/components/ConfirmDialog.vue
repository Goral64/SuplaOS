<script setup>
import { nextTick, ref, watch } from 'vue'
import { answer, dialog } from './dialog'

const ok = ref(null)
watch(() => dialog.resolve, async (r) => {
  await nextTick()
  if (r && ok.value) ok.value.focus()
})
</script>

<template>
  <div v-if="dialog.resolve" class="modal-back" @keydown.esc="answer(false)">
    <div class="modal narrow" role="dialog">
      <p class="dialog-text">{{ dialog.message }}</p>
      <div class="row mt">
        <span class="spacer"></span>
        <button v-if="dialog.cancel" @click="answer(false)">{{ $t('common.cancel') }}</button>
        <button ref="ok" :class="dialog.danger ? 'danger filled' : 'primary'" @click="answer(true)">{{ dialog.ok || 'OK' }}</button>
      </div>
    </div>
  </div>
</template>
