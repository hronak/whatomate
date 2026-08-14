<script setup lang="ts">
import { Input } from '@/components/ui/input'
import { Search, X } from '@lucide/vue'

const model = defineModel<string>({ default: '' })

withDefaults(defineProps<{
  placeholder?: string
  class?: string
}>(), {
  placeholder: 'Search...',
})

function clear() {
  model.value = ''
}
</script>

<template>
  <div class="relative" :class="$props.class">
    <Search class="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
    <Input
      v-model="model"
      :placeholder="placeholder"
      class="pl-9 pr-8"
    />
    <button
      v-if="model"
      type="button"
      class="absolute right-2 top-1/2 -translate-y-1/2 size-5 rounded-full hover:bg-muted flex items-center justify-center"
      @click="clear"
    >
      <X class="size-3 text-muted-foreground" />
    </button>
  </div>
</template>
