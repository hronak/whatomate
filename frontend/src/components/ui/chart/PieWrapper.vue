<script setup lang="ts">
import DonutWrapper from './DonutWrapper.vue'
import type { ChartData } from '@/components/ui/chart'

// A pie is a donut with no hole; DonutWrapper renders it via arcWidth=0.
defineProps<{
  data: ChartData
  options?: any
}>()
</script>

<template>
  <DonutWrapper :data="data" :options="options" :isPie="true" />
</template>
