<script setup lang="ts">
import { computed } from 'vue'
import { VisSingleContainer, VisDonut } from '@unovis/vue'
import { ChartContainer, ChartTooltip, ChartTooltipContent } from '@/components/ui/chart'
import type { ChartData } from '@/components/ui/chart'

const props = defineProps<{
  data: ChartData
  options?: any
  isPie?: boolean
}>()

interface Slice {
  label: string
  value: number
  color: string
}

// backgroundColor is one colour per slice for a donut, but the shared type also
// permits a single string; fall back to using it for every slice rather than
// indexing into a string and rendering one character.
const sliceColor = (index: number) => {
  const bg = props.data.datasets[0]?.backgroundColor
  return (Array.isArray(bg) ? bg[index] : bg) || `hsl(var(--primary))`
}

const unovisData = computed<Slice[]>(() => {
  if (!props.data?.labels || !props.data.datasets?.length) return []
  return props.data.labels.map((label, index) => ({
    label,
    value: props.data.datasets[0].data[index],
    color: sliceColor(index)
  }))
})

const chartConfig = computed(() => {
  const config: Record<string, any> = {
    value: { label: 'Value' }
  }
  if (!props.data?.labels || !props.data.datasets?.length) return config
  
  props.data.labels.forEach((label, i) => {
    config[`color_${i}`] = { label, color: sliceColor(i) }
  })

  return config
})

// Declared here rather than inline in the template: an inline arrow in a
// binding has no contextual type, so its parameter is an implicit any.
const valueAccessor = (d: Slice) => d.value
const colorAccessor = (d: Slice) => d.color
</script>

<template>
  <ChartContainer :config="chartConfig" class="h-full w-full min-h-[250px]">
    <VisSingleContainer :data="unovisData">
      <VisDonut
        :value="valueAccessor"
        :color="colorAccessor"
        :arcWidth="isPie ? 0 : undefined"
      />
      <ChartTooltip :content="ChartTooltipContent" />
    </VisSingleContainer>
  </ChartContainer>
</template>
