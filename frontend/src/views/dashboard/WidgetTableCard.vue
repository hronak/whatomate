<script setup lang="ts">
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { GripVertical, Pencil, Trash2, Clock } from "@lucide/vue";
import type { DashboardWidget, WidgetData } from "@/services/api";
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

const { formatTime } = useDashboardUtils();
</script>

<template>
  <div class="group relative h-full flex flex-col card-depth rounded-xl border border-border bg-card hover:bg-accent transition-colors overflow-hidden">
    <!-- Drag handle -->
    <div
      v-if="isDragMode"
      class="widget-drag-handle absolute top-2 left-2 text-foreground/20 cursor-grab active:cursor-grabbing z-10"
    >
      <GripVertical class="size-4" />
    </div>

    <div class="p-6 pb-3 flex flex-row items-center justify-between">
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
      </div>
    </div>

    <div class="flex-1 min-h-0 overflow-auto px-6 pb-6">
      <template v-if="isLoading">
        <Skeleton class="size-full bg-muted" />
      </template>
      <!-- Grouped table (group_by set) -->
      <template v-else-if="widget.group_by_field && data?.data_points?.length">
        <table class="w-full">
          <thead>
            <tr class="border-b border-border">
              <th class="text-left py-2 font-medium text-foreground/40 uppercase">
                {{ widget.group_by_field }}
              </th>
              <th class="text-right py-2 font-medium text-foreground/40 uppercase">
                {{ $t("dashboard.count") }}
              </th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="dp in data.data_points"
              :key="dp.label"
              class="border-b border-border"
            >
              <td class="py-2 text-foreground/70">{{ dp.label }}</td>
              <td class="py-2 text-right text-foreground font-medium">
                {{ dp.value }}
              </td>
            </tr>
          </tbody>
        </table>
      </template>
      <!-- Row list (no group_by) -->
      <template v-else-if="data?.table_rows?.length">
        <div class="gap-y-3">
          <div
            v-for="row in data.table_rows"
            :key="row.id"
            class="flex items-start gap-3 p-3 rounded-lg hover:bg-accent transition-colors"
          >
            <div
              :class="[
                'size-10 rounded-lg flex items-center justify-center font-medium shrink-0',
                row.direction === 'incoming'
                  ? 'bg-success/15 text-success'
                  : 'bg-info/15 text-info',
              ]"
            >
              {{
                row.label
                  .split('')
                  .map((n: string) => n[0])
                  .join('')
                  .slice(0, 2)
                  .toUpperCase()
              }}
            </div>
            <div class="flex-1 min-w-0">
              <div class="flex items-center justify-between">
                <p class="font-medium truncate text-foreground">
                  {{ row.label }}
                </p>
                <span class="text-foreground/40 flex items-center gap-1 shrink-0">
                  <Clock class="size-3" />
                  {{ formatTime(row.created_at) }}
                </span>
              </div>
              <p class="text-foreground/50 truncate">
                {{ row.sub_label }}
              </p>
              <div class="flex items-center gap-2 mt-1">
                <span
                  v-if="row.direction"
                  :class="[
                    'px-1.5 py-0.5 rounded-full font-medium',
                    row.direction === 'incoming'
                      ? 'bg-success/20 text-success'
                      : 'bg-info/20 text-info',
                  ]"
                >
                  {{ row.direction }}
                </span>
                <span
                  v-if="row.status"
                  :class="[
                    'px-1.5 py-0.5 rounded-full font-medium',
                    row.status === 'delivered'
                      ? 'bg-info/20 text-info'
                      : row.status === 'read'
                        ? 'bg-success/20 text-success'
                        : row.status === 'failed'
                          ? 'bg-destructive/20 text-destructive'
                          : 'bg-muted text-foreground/50',
                  ]"
                >
                  {{ row.status }}
                </span>
              </div>
            </div>
          </div>
        </div>
      </template>
      <template v-else>
        <div class="h-full flex items-center justify-center text-foreground/40">
          {{ $t("common.noData") }}
        </div>
      </template>
    </div>
  </div>
</template>
