<script setup lang="ts">
import { cn } from '@/lib/utils'

const props = defineProps<{
  class?: string
}>()
</script>

<template>
  <div
    :class="cn(
      'rounded-xl border border-white/6 bg-white/2 text-card-foreground backdrop-blur-sm shadow-xl shadow-black/20 transition-all duration-300 hover:border-white/15 hover:bg-white/4 hover:shadow-2xl hover:shadow-emerald-500/5 light:bg-white light:border-gray-200 light:shadow-gray-200/50 hover:light:border-gray-300 hover:light:shadow-lg hover:light:shadow-gray-300/50',
      props.class
    )"
  >
    <slot />
  </div>
</template>
