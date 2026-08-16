<script setup lang="ts">
import { computed } from 'vue'
import { VisXYContainer, VisLine, VisAxis } from '@unovis/vue'
import { ChartContainer, ChartTooltip, ChartTooltipContent } from '@/components/ui/chart'

const props = defineProps<{
  data: {
    labels: string[],
    datasets: { label: string, data: number[], borderColor?: string, backgroundColor?: string }[]
  }
  options?: any
}>()

const chartConfig = computed(() => {
  const config: Record<string, any> = {}
  if (!props.data?.datasets) return config
  
  props.data.datasets.forEach((ds, i) => {
    config[`dataset_${i}`] = {
      label: ds.label || `Dataset ${i}`,
      color: ds.borderColor || ds.backgroundColor || `hsl(var(--primary))`
    }
  })
  return config
})

const unovisData = computed(() => {
  if (!props.data?.labels) return []
  return props.data.labels.map((label, index) => {
    const dataPoint: any = { label }
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
  return props.data.datasets.map((_, i) => chartConfig.value[`dataset_${i}`].color)
})

const showXAxis = computed(() => props.options?.scales?.x?.display !== false)
const showYAxis = computed(() => props.options?.scales?.y?.display !== false)
</script>

<template>
  <ChartContainer :config="chartConfig" class="h-full w-full min-h-[250px]">
    <VisXYContainer :data="unovisData">
      <VisLine 
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
