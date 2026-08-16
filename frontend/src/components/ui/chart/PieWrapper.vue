<script setup lang="ts">
import DonutWrapper from './DonutWrapper.vue'

defineProps<{
  data: any
  options?: any
}>()
</script>

<template>
  <DonutWrapper :data="data" :options="options" :isPie="true" />
</template>
