<script setup lang="ts">
import type { HTMLAttributes } from "vue"
import { cn } from "@/lib/utils"

const props = defineProps<{
  class?: HTMLAttributes["class"]
}>()
</script>

<template>
  <tr :class="cn('border-b transition-colors duration-150 hover:bg-gray-50 even:bg-gray-50/50 data-[state=selected]:bg-muted dark:hover:bg-white/3 dark:even:bg-white/1', props.class)">
    <slot />
  </tr>
</template>
