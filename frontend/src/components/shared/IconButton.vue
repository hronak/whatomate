<script setup lang="ts">
import type { Component, HTMLAttributes } from 'vue'
import { Button, type ButtonVariants } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

withDefaults(defineProps<{
  icon?: Component
  label: string
  variant?: ButtonVariants['variant']
  size?: ButtonVariants['size']
  class?: HTMLAttributes['class']
  disabled?: boolean
  loading?: boolean
}>(), {
  variant: 'ghost',
  size: 'icon',
  disabled: false,
  loading: false,
})

defineEmits<{
  click: [e: MouseEvent]
}>()
</script>

<template>
  <Tooltip>
    <TooltipTrigger as-child>
      <Button
        :variant="variant"
        :size="size"
        :class="$props.class"
        :disabled="disabled"
        :loading="loading"
        :aria-label="label"
        @click="$emit('click', $event)"
      >
        <slot>
          <component v-if="icon" :is="icon" class="size-4" />
        </slot>
      </Button>
    </TooltipTrigger>
    <TooltipContent>{{ label }}</TooltipContent>
  </Tooltip>
</template>
