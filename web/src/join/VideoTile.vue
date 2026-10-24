<script setup>
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'

const props = defineProps({
  stream: { type: Object, default: null },
  name: { type: String, default: '' },
  muted: { type: Boolean, default: false }, // local preview: never play our own audio
  mirror: { type: Boolean, default: false },
  badge: { type: String, default: '' },
})

const video = ref(null)
const hasVideo = ref(false)

const attach = () => {
  if (!video.value) return
  if (video.value.srcObject !== props.stream) video.value.srcObject = props.stream
  hasVideo.value = !!props.stream?.getVideoTracks().some((t) => t.readyState === 'live' && t.enabled)
}

onMounted(attach)
watch(() => props.stream, attach)
// Track changes (camera toggled, renegotiation) don't replace the stream object.
const timer = setInterval(attach, 1000)
onBeforeUnmount(() => clearInterval(timer))
</script>

<template>
  <div class="relative rounded-xl overflow-hidden bg-slate-800 aspect-video flex items-center justify-center">
    <video
      ref="video"
      autoplay
      playsinline
      :muted="muted"
      :class="['w-full h-full object-cover', mirror ? '-scale-x-100' : '', hasVideo ? '' : 'opacity-0']"
    />
    <div v-if="!hasVideo" class="absolute inset-0 flex items-center justify-center">
      <div class="w-16 h-16 rounded-full bg-slate-600 text-white text-2xl font-semibold flex items-center justify-center">
        {{ (name || '?').slice(0, 1).toUpperCase() }}
      </div>
    </div>
    <div class="absolute bottom-2 left-2 flex items-center gap-2">
      <span class="px-2 py-0.5 rounded bg-black/60 text-white text-xs">{{ name }}</span>
      <span v-if="badge" class="px-2 py-0.5 rounded bg-emerald-600/80 text-white text-xs">{{ badge }}</span>
    </div>
  </div>
</template>
