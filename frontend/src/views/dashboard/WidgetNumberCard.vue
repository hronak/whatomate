<script setup lang="ts">
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { GripVertical, Pencil, Trash2, TrendingUp, TrendingDown, Minus } from "@lucide/vue";
import type { DashboardWidget, WidgetData } from "@/services/api";
import { useDashboardUtils } from "./useDashboardUtils";

const props = defineProps<{
  widget: DashboardWidget;
  data: WidgetData | undefined;
  isLoading: boolean;
  isDragMode: boolean;
  canEditWidget: boolean;
  canDeleteWidget: boolean;
  comparisonPeriodLabel: string;
}>();

const emit = defineEmits<{
  (e: "edit", widget: DashboardWidget): void;
  (e: "delete", widget: DashboardWidget): void;
}>();

const { getWidgetColor, getWidgetIcon, formatNumber } = useDashboardUtils();
</script>

<template>
  <div class="group relative h-full card-depth rounded-xl border border-border bg-card p-6 hover:bg-accent transition-colors overflow-hidden">
    <!-- Gradient accent bar -->
    <div
      :class="[
        'absolute top-0 inset-x-0 h-0.5',
        getWidgetColor(widget.color).gradient,
      ]"
    />

    <!-- Drag handle indicator -->
    <div
      v-if="isDragMode"
      class="widget-drag-handle absolute top-2 left-2 text-foreground/20 cursor-grab active:cursor-grabbing z-10"
    >
      <GripVertical class="size-4" />
    </div>

    <div class="flex flex-row items-start justify-between gap-y-0 pb-2">
      <div class="flex-1">
        <span class="font-medium text-foreground/50">
          {{ widget.name }}
        </span>
      </div>
      <div class="flex items-center gap-2">
        <!-- Actions - hidden in drag mode -->
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
        <!-- Icon -->
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

    <div class="pt-2">
      <div class="text-3xl font-bold text-foreground">
        <template v-if="isLoading">
          <Skeleton class="h-8 w-20 bg-muted" />
        </template>
        <template v-else>
          <span>{{ formatNumber(data?.value || 0) }}</span>
        </template>
      </div>
      <div
        v-if="widget.show_change && data"
        class="flex items-center text-foreground/40 mt-1"
      >
        <component
          :is="
            data.change > 0
              ? TrendingUp
              : data.change < 0
                ? TrendingDown
                : Minus
          "
          :class="[
            'size-3 mr-1',
            data.change > 0
              ? 'text-success'
              : data.change < 0
                ? 'text-destructive'
                : 'text-foreground/30',
          ]"
        />
        <span
          :class="
            data.change > 0
              ? 'text-success'
              : data.change < 0
                ? 'text-destructive'
                : 'text-foreground/30'
          "
        >
          {{ Math.abs(data.change || 0).toFixed(1) }}%
        </span>
        <span class="ml-1">{{ comparisonPeriodLabel }}</span>
      </div>
    </div>
  </div>
</template>
