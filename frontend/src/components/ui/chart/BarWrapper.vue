<script setup lang="ts">
import { computed } from 'vue'
import { VisXYContainer, VisGroupedBar, VisAxis } from '@unovis/vue'
import { ChartContainer, ChartTooltip, ChartTooltipContent } from '@/components/ui/chart'
import type { ChartData } from '@/components/ui/chart'

const props = defineProps<{
  data: ChartData
  options?: any
}>()

const chartConfig = computed(() => {
  const config: Record<string, any> = {}
  if (!props.data?.datasets) return config
  
  props.data.datasets.forEach((ds, i) => {
    config[`dataset_${i}`] = {
      label: ds.label || `Dataset ${i}`,
      // if array of colors is passed, we take the first for the legend config
      color: Array.isArray(ds.backgroundColor) ? ds.backgroundColor[0] : ds.backgroundColor || `hsl(var(--primary))`
    }
  })
  return config
})

const unovisData = computed(() => {
  if (!props.data?.labels) return []
  return props.data.labels.map((label, index) => {
    const dataPoint: any = { label, index }
    props.data.datasets.forEach((ds, i) => {
      dataPoint[`dataset_${i}`] = ds.data[index]
    })
    return dataPoint
  })
})

// Unovis passes (datum, index); these charts are ordinal, so only the index
// matters. The underscore keeps noUnusedParameters quiet without dropping the
// parameter, which would change the accessor's arity.
const xAccessor = (_d: unknown, i: number) => i
const tickFormat = (i: number) => props.data.labels[i]
const yAccessors = computed(() => {
  if (!props.data?.datasets) return []
  return props.data.datasets.map((_, i) => (d: any) => d[`dataset_${i}`])
})
const colors = computed(() => {
  if (!props.data?.datasets) return []
  // A single dataset carrying an array of colours means one colour per bar
  // (the group_by case), which Unovis wants as an accessor rather than a list.
  // Bind the array to a const first: the closure below would otherwise lose the
  // Array.isArray narrowing and backgroundColor is optional.
  const palette = props.data.datasets[0]?.backgroundColor
  if (props.data.datasets.length === 1 && Array.isArray(palette)) {
    return (d: { index: number }) => palette[d.index]
  }
  return props.data.datasets.map((_, i) => chartConfig.value[`dataset_${i}`].color)
})

const showXAxis = computed(() => props.options?.scales?.x?.display !== false)
const showYAxis = computed(() => props.options?.scales?.y?.display !== false)
</script>

<template>
  <ChartContainer :config="chartConfig" class="h-full w-full min-h-[250px]">
    <VisXYContainer :data="unovisData">
      <VisGroupedBar 
        :x="xAccessor" 
        :y="yAccessors" 
        :color="colors"
      />
      <VisAxis type="x" :tickFormat="tickFormat" v-if="showXAxis" />
      <VisAxis type="y" v-if="showYAxis" />
      <ChartTooltip :content="ChartTooltipContent" />
    </VisXYContainer>
  </ChartContainer>
</template>
