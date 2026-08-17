<script setup lang="ts">
import { Button } from "@/components/ui/button";
import { GripVertical, Pencil, Trash2 } from "@lucide/vue";
import type { DashboardWidget } from "@/services/api";
import { useDashboardUtils } from "./useDashboardUtils";

const props = defineProps<{
  widget: DashboardWidget;
  isDragMode: boolean;
  canEditWidget: boolean;
  canDeleteWidget: boolean;
  itemW: number;
}>();

const emit = defineEmits<{
  (e: "edit", widget: DashboardWidget): void;
  (e: "delete", widget: DashboardWidget): void;
}>();

const { SHORTCUT_REGISTRY } = useDashboardUtils();
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

    <div class="flex-1 min-h-0 overflow-y-auto px-6 pb-6">
      <div
        :class="[
          'grid gap-3 pt-1',
          itemW >= 8 ? 'grid-cols-3' : 'grid-cols-2',
        ]"
      >
        <template
          v-for="key in widget.config?.shortcuts || []"
          :key="key"
        >
          <RouterLink
            v-if="SHORTCUT_REGISTRY[key as keyof typeof SHORTCUT_REGISTRY]"
            :to="SHORTCUT_REGISTRY[key as keyof typeof SHORTCUT_REGISTRY].to"
            class="card-interactive flex flex-col items-center justify-center p-4 rounded-xl border border-border bg-muted"
          >
            <div
              class="size-12 rounded-lg border bg-background text-foreground flex items-center justify-center mb-2"
            >
              <component
                :is="SHORTCUT_REGISTRY[key as keyof typeof SHORTCUT_REGISTRY].icon"
                class="size-6"
              />
            </div>
            <span class="font-medium text-foreground">{{
              SHORTCUT_REGISTRY[key as keyof typeof SHORTCUT_REGISTRY].label
            }}</span>
          </RouterLink>
        </template>
      </div>
    </div>
  </div>
</template>
