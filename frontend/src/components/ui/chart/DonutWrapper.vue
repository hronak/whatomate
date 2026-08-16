<script setup lang="ts">
import { computed } from 'vue'
import { VisSingleContainer, VisDonut } from '@unovis/vue'
import { ChartContainer, ChartTooltip, ChartTooltipContent } from '@/components/ui/chart'

const props = defineProps<{
  data: {
    labels: string[],
    datasets: { data: number[], backgroundColor?: string[] }[]
  }
  options?: any
  isPie?: boolean
}>()

const unovisData = computed(() => {
  if (!props.data?.labels || !props.data.datasets?.length) return []
  return props.data.labels.map((label, index) => {
    return {
      label,
      value: props.data.datasets[0].data[index],
      color: props.data.datasets[0].backgroundColor?.[index] || `hsl(var(--primary))`
    }
  })
})

const chartConfig = computed(() => {
  const config: Record<string, any> = {
    value: { label: 'Value' }
  }
  if (!props.data?.labels || !props.data.datasets?.length) return config
  
  props.data.labels.forEach((label, i) => {
    config[`color_${i}`] = {
      label,
      color: props.data.datasets[0].backgroundColor?.[i] || `hsl(var(--primary))`
    }
  })
  
  return config
})
</script>

<template>
  <ChartContainer :config="chartConfig" class="h-full w-full min-h-[250px]">
    <VisSingleContainer :data="unovisData">
      <VisDonut 
        :value="(d) => d.value" 
        :color="(d) => d.color"
        :arcWidth="isPie ? 0 : undefined"
      />
      <ChartTooltip :content="ChartTooltipContent" />
    </VisSingleContainer>
  </ChartContainer>
</template>
