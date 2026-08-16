import type { Component, Ref } from "vue"
import { createContext } from "reka-ui"

export { default as ChartContainer } from "./ChartContainer.vue"
export { default as ChartLegendContent } from "./ChartLegendContent.vue"
export { default as ChartTooltipContent } from "./ChartTooltipContent.vue"
export { componentToString } from "./utils"

// Format: { THEME_NAME: CSS_SELECTOR }
export const THEMES = { light: "", dark: ".dark" } as const

export type ChartConfig = {
  [k in string]: {
    label?: string | Component
    icon?: string | Component
  } & (
    | { color?: string, theme?: never }
    | { color?: never, theme: Record<keyof typeof THEMES, string> }
  )
}

/**
 * The chart.js-shaped dataset the Line/Bar/Pie/Doughnut wrappers accept.
 *
 * `label` is optional and `backgroundColor` takes either form deliberately,
 * because chart.js is permissive in exactly those two places and callers rely
 * on it: a pie/doughnut dataset omits the per-dataset label (slice names come
 * from `labels`) and carries one colour per slice, while line/bar datasets name
 * themselves and usually carry a single colour. A builder that returns a
 * different shape per chart type produces a union of all of them, so requiring
 * `label` here would reject the union at every call site even though the
 * wrappers already fall back with `ds.label || \`Dataset ${i}\``.
 */
export interface ChartDataset {
  label?: string
  data: number[]
  backgroundColor?: string | string[]
  borderColor?: string
  borderWidth?: number
  fill?: boolean
  tension?: number
}

export interface ChartData {
  labels: string[]
  datasets: ChartDataset[]
}

interface ChartContextProps {
  id: string
  config: Ref<ChartConfig>
}

export const [useChart, provideChartContext] = createContext<ChartContextProps>("Chart")

export { VisCrosshair as ChartCrosshair, VisTooltip as ChartTooltip } from "@unovis/vue"
