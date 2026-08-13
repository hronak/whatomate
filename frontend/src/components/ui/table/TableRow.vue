<script setup lang="ts">
import type { HTMLAttributes } from "vue"
import { cn } from "@/lib/utils"

const props = defineProps<{
  class?: HTMLAttributes["class"]
}>()
</script>

<template>
  <tr :class="cn('border-b transition-colors duration-150 hover:bg-white/3 even:bg-white/1 data-[state=selected]:bg-muted hover:light:bg-gray-50 even:light:bg-gray-50/50', props.class)">
    <slot />
  </tr>
</template>
