<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { ArrowUp } from '@lucide/vue'
import { Button } from '@/components/ui/button'

const props = withDefaults(defineProps<{
  target?: string
  threshold?: number
}>(), {
  threshold: 300,
})

const isVisible = ref(false)
let scrollEl: HTMLElement | null = null

function onScroll() {
  if (scrollEl) {
    isVisible.value = scrollEl.scrollTop > props.threshold
  }
}

function scrollToTop() {
  scrollEl?.scrollTo({ top: 0, behavior: 'smooth' })
}

onMounted(() => {
  scrollEl = props.target
    ? document.querySelector(props.target)
    : document.querySelector('main')
  scrollEl?.addEventListener('scroll', onScroll, { passive: true })
})

onUnmounted(() => {
  scrollEl?.removeEventListener('scroll', onScroll)
})
</script>

<template>
  <Transition name="page">
    <Button
      v-show="isVisible"
      variant="secondary"
      size="icon"
      class="fixed bottom-6 right-6 z-40 size-10 rounded-full shadow-lg shadow-gray-300/30 ring-1 ring-border dark:shadow-black/20"
      aria-label="Scroll to top"
      @click="scrollToTop"
    >
      <ArrowUp class="size-4" />
    </Button>
  </Transition>
</template>
