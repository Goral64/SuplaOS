<script setup>
import { nextTick, ref, watch } from 'vue'
import { activeJob, closeJob } from './jobs'

const log = ref(null)
watch(() => activeJob.lines.length, async () => {
  await nextTick()
  if (log.value) log.value.scrollTop = log.value.scrollHeight
})
</script>

<template>
  <div v-if="activeJob.job" class="modal-back">
    <div class="modal">
      <h2>{{ activeJob.job.title }}</h2>
      <pre ref="log" class="log">{{ activeJob.lines.join('\n') || ' ' }}</pre>
      <div class="row mt">
        <span v-if="!activeJob.result" class="badge warn">{{ $t('job.running') }}</span>
        <span v-else-if="activeJob.result.state === 'done'" class="badge ok">{{ $t('job.done') }}</span>
        <span v-else class="badge bad">{{ $t('job.failed', { error: activeJob.result.error }) }}</span>
        <span class="spacer"></span>
        <button class="primary" :disabled="!activeJob.result" @click="closeJob">{{ $t('common.close') }}</button>
      </div>
    </div>
  </div>
</template>
