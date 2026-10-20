<script setup>
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'

const props = defineProps({ addon: String })
const lines = ref([])
const pre = ref(null)
let es = null

onMounted(() => {
  es = new EventSource(`/api/addons/${props.addon}/logs?tail=200`)
  es.addEventListener('line', async (e) => {
    const box = pre.value
    const atEnd = box && box.scrollTop + box.clientHeight >= box.scrollHeight - 20
    lines.value.push(e.data)
    if (lines.value.length > 2000) lines.value.splice(0, lines.value.length - 2000)
    await nextTick()
    if (atEnd && pre.value) pre.value.scrollTop = pre.value.scrollHeight
  })
  es.addEventListener('end', () => es.close())
})
onBeforeUnmount(() => es && es.close())
</script>

<template>
  <pre ref="pre" class="log">{{ lines.join('\n') || '...' }}</pre>
</template>
