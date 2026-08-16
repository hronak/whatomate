<script setup lang="ts">
import { computed } from 'vue'
import { VisXYContainer, VisGroupedBar, VisAxis } from '@unovis/vue'
import { ChartContainer, ChartTooltip, ChartTooltipContent } from '@/components/ui/chart'

const props = defineProps<{
  data: {
    labels: string[],
    datasets: { label: string, data: number[], backgroundColor?: string | string[] }[]
  }
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

const xAccessor = (d: any, i: number) => i
const tickFormat = (i: number) => props.data.labels[i]
const yAccessors = computed(() => {
  if (!props.data?.datasets) return []
  return props.data.datasets.map((_, i) => (d: any) => d[`dataset_${i}`])
})
const colors = computed(() => {
  if (!props.data?.datasets) return []
  // If the first dataset has an array of colors (e.g. Bar chart with one dataset but many colors)
  // we can use a color accessor function for Unovis.
  const firstDs = props.data.datasets[0]
  if (props.data.datasets.length === 1 && Array.isArray(firstDs.backgroundColor)) {
     return (d: any, i: number) => firstDs.backgroundColor[d.index]
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
