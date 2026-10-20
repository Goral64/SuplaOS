<script setup>
import { computed } from 'vue'

const props = defineProps({ container: Object })
const kind = computed(() => {
  const c = props.container
  if (c.Health === 'unhealthy' || c.State === 'dead' || c.State === 'missing') return 'bad'
  if (c.State === 'running' && c.Health !== 'starting') return 'ok'
  return 'warn'
})
</script>

<template>
  <span class="badge" :class="kind" :title="container.Status">
    {{ container.Service }}: {{ $t('state.' + container.State) }}<template v-if="container.Health">, {{ $t('state.' + container.Health) }}</template>
  </span>
</template>
