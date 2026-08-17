<script setup lang="ts">
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { GripVertical, Pencil, Trash2 } from "@lucide/vue";
import type { DashboardWidget, WidgetData } from "@/services/api";
import { Line, Bar, Pie } from "@/lib/charts";
import { useDashboardUtils } from "./useDashboardUtils";

const props = defineProps<{
  widget: DashboardWidget;
  data: WidgetData | undefined;
  isLoading: boolean;
  isDragMode: boolean;
  canEditWidget: boolean;
  canDeleteWidget: boolean;
}>();

const emit = defineEmits<{
  (e: "edit", widget: DashboardWidget): void;
  (e: "delete", widget: DashboardWidget): void;
}>();

const {
  getWidgetColor,
  getWidgetIcon,
  getChartComponentData,
  lineBarChartOptions,
  pieChartOptions,
} = useDashboardUtils();
</script>

<template>
  <div class="group relative h-full flex flex-col card-depth rounded-xl border border-border bg-card p-6 hover:bg-accent transition-colors overflow-hidden">
    <!-- Drag handle indicator -->
    <div
      v-if="isDragMode"
      class="widget-drag-handle absolute top-2 left-2 text-foreground/20 cursor-grab active:cursor-grabbing z-10"
    >
      <GripVertical class="size-4" />
    </div>

    <div class="flex flex-row items-center justify-between pb-2">
      <div>
        <span class="font-medium text-foreground/50">{{ widget.name }}</span>
        <p
          v-if="widget.description"
          class="text-foreground/30 mt-0.5"
        >
          {{ widget.description }}
        </p>
      </div>
      <div class="flex items-center gap-2">
        <div
          v-if="!isDragMode && (canEditWidget || canDeleteWidget)"
          class="flex items-center gap-1 opacity-0 group-hover:opacity-100"
        >
          <Button
            v-if="canEditWidget"
            variant="ghost"
            size="icon"
            class="size-6 text-foreground/20 hover:text-foreground hover:bg-accent"
            @click.stop="emit('edit', widget)"
            :title="$t('dashboard.editWidgetTooltip')"
          >
            <Pencil class="size-3" />
          </Button>
          <Button
            v-if="canDeleteWidget"
            variant="ghost"
            size="icon"
            class="size-6 text-foreground/20 hover:text-destructive hover:bg-destructive/10"
            @click.stop="emit('delete', widget)"
            :title="$t('dashboard.deleteWidgetTooltip')"
          >
            <Trash2 class="size-3" />
          </Button>
        </div>
        <div
          :class="[
            'size-10 rounded-lg flex items-center justify-center',
            getWidgetColor(widget.color).bg,
          ]"
        >
          <component
            :is="getWidgetIcon(widget.data_source)"
            :class="[
              'size-5',
              getWidgetColor(widget.color).text,
            ]"
          />
        </div>
      </div>
    </div>
    <div class="flex-1 min-h-0 pt-2">
      <template v-if="isLoading">
        <Skeleton class="size-full bg-muted" />
      </template>
      <template
        v-else-if="
          (data?.chart_data?.length || 0) > 0 ||
          (data?.data_points?.length || 0) > 0 ||
          (data?.grouped_series?.datasets?.length || 0) > 0
        "
      >
        <Line
          v-if="widget.chart_type === 'line'"
          :data="getChartComponentData(widget, { [widget.id]: data! })"
          :options="lineBarChartOptions"
        />
        <Bar
          v-else-if="widget.chart_type === 'bar'"
          :data="getChartComponentData(widget, { [widget.id]: data! })"
          :options="lineBarChartOptions"
        />
        <Pie
          v-else-if="widget.chart_type === 'pie'"
          :data="getChartComponentData(widget, { [widget.id]: data! })"
          :options="pieChartOptions"
        />
      </template>
      <template v-else>
        <div class="h-full flex items-center justify-center text-foreground/40">
          {{ $t("common.noData") }}
        </div>
      </template>
    </div>
  </div>
</template>
