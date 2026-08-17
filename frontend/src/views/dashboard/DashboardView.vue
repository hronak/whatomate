<script setup lang="ts">
import { ref, shallowRef, onMounted, computed, watch } from "vue";
import { useI18n } from "vue-i18n";
import { GridLayout, GridItem } from "grid-layout-plus";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Switch } from "@/components/ui/switch";
import {
  widgetsService,
  type DashboardWidget,
  type WidgetData,
  type LayoutItem,
} from "@/services/api";
import { useAuthStore } from "@/stores/auth";
import {
  LayoutDashboard,
  Plus,
  X,
  GripVertical,
} from "@lucide/vue";
// Centralized Chart.js setup (registered once)
import { DateRangePicker } from "@/components/shared";
import { useDateRange } from "@/composables/useDateRange";
import { useAppToast } from "@/composables/useAppToast";
import WidgetNumberCard from "./WidgetNumberCard.vue";
import WidgetChartCard from "./WidgetChartCard.vue";
import WidgetTableCard from "./WidgetTableCard.vue";
import WidgetShortcutsCard from "./WidgetShortcutsCard.vue";
import { useDashboardUtils } from "./useDashboardUtils";

const { success, error: showError } = useAppToast();
const { t } = useI18n();
const authStore = useAuthStore();

// Permission checks
const canCreateWidget = computed(() =>
  authStore.hasPermission("analytics", "write"),
);
const canEditWidget = computed(() =>
  authStore.hasPermission("analytics", "write"),
);
const canDeleteWidget = computed(() =>
  authStore.hasPermission("analytics", "delete"),
);

// Widgets state
const widgets = ref<DashboardWidget[]>([]);
const widgetData = shallowRef<Record<string, WidgetData>>({});

const isLoading = ref(true);
const isWidgetDataLoading = ref(false);

// Widget builder state
const isWidgetDialogOpen = ref(false);
const isEditMode = ref(false);
const editingWidgetId = ref<string | null>(null);
const isSavingWidget = ref(false);

// Delete dialog state
const deleteDialogOpen = ref(false);
const widgetToDelete = ref<DashboardWidget | null>(null);

const dataSources = ref<
  Array<{ name: string; label: string; fields: string[] }>
>([]);
const metrics = ref<string[]>([]);
const displayTypes = ref<string[]>([]);
const operators = ref<Array<{ value: string; label: string }>>([]);

const widgetForm = ref({
  name: "",
  description: "",
  data_source: "",
  metric: "count",
  field: "",
  filters: [] as Array<{ field: string; operator: string; value: string }>,
  display_type: "number",
  chart_type: "",
  group_by_field: "",
  show_change: true,
  color: "blue",
  size: "small",
  config: {} as Record<string, any>,
  is_shared: false,
});

const selectedShortcuts = ref<string[]>([]);

// Time range filter
const {
  selectedRange,
  customDateRange,
  isDatePickerOpen,
  dateRange,
  formatDateRangeDisplay,
  applyCustomRange: applyCustomRangeBase,
} = useDateRange({ storageKey: "dashboard" });

const {
  SHORTCUT_REGISTRY,
  colorOptions,
  chartTypeOptions,
  comparisonPeriodLabel,
} = useDashboardUtils(selectedRange);

// Grid layout state
const GRID_COLS = 12;
const GRID_ROW_HEIGHT = 40;
const GRID_MARGIN: [number, number] = [16, 16];

const isDragMode = ref(false);
const gridLayout = ref<
  Array<{ i: string; x: number; y: number; w: number; h: number }>
>([]);

const isChartWidget = (widget: DashboardWidget) =>
  widget.display_type === "chart";
const isTableWidget = (widget: DashboardWidget) =>
  widget.display_type === "table";
const isShortcutsWidget = (widget: DashboardWidget) =>
  widget.display_type === "shortcuts";
const isNumberWidget = (widget: DashboardWidget) =>
  !isChartWidget(widget) &&
  !isTableWidget(widget) &&
  !isShortcutsWidget(widget);

const getWidgetById = (id: string): DashboardWidget | undefined => {
  return widgets.value.find((w) => w.id === id);
};

const computeGridLayout = (widgetList: DashboardWidget[]) => {
  const layout: Array<{
    i: string;
    x: number;
    y: number;
    w: number;
    h: number;
  }> = [];

  // Separate positioned (grid_w > 0) from legacy (grid_w === 0) widgets
  const positioned = widgetList.filter((w) => w.grid_w > 0);
  const legacy = widgetList.filter((w) => w.grid_w === 0);

  // Add positioned widgets as-is
  for (const w of positioned) {
    layout.push({
      i: w.id,
      x: w.grid_x,
      y: w.grid_y,
      w: w.grid_w,
      h: w.grid_h,
    });
  }

  // Auto-position legacy widgets
  if (legacy.length > 0) {
    // Find the max y used by positioned widgets to place legacy below
    let nextY = 0;
    if (positioned.length > 0) {
      nextY = Math.max(...positioned.map((w) => w.grid_y + w.grid_h));
    }

    let curX = 0;
    let curY = nextY;

    // Number widgets first, then chart/table/shortcuts widgets
    const legacyNumber = legacy.filter(
      (w) => !["chart", "table", "shortcuts"].includes(w.display_type),
    );
    const legacyLarge = legacy.filter((w) =>
      ["chart", "table", "shortcuts"].includes(w.display_type),
    );

    for (const w of legacyNumber) {
      const itemW = 3;
      const itemH = 3;
      if (curX + itemW > GRID_COLS) {
        curX = 0;
        curY += itemH;
      }
      layout.push({ i: w.id, x: curX, y: curY, w: itemW, h: itemH });
      curX += itemW;
    }

    // Move to next row for large widgets
    if (legacyNumber.length > 0 && legacyLarge.length > 0) {
      curX = 0;
      curY += 3;
    }

    for (const w of legacyLarge) {
      let itemW = 6;
      let itemH = 5;
      if (w.display_type === "table" || w.display_type === "shortcuts") {
        itemW = 6;
        itemH = 8;
      }
      if (curX + itemW > GRID_COLS) {
        curX = 0;
        curY += itemH;
      }
      layout.push({ i: w.id, x: curX, y: curY, w: itemW, h: itemH });
      curX += itemW;
    }
  }

  return layout;
};

// Rebuild grid layout when widgets change
watch(
  widgets,
  (val) => {
    gridLayout.value = computeGridLayout(val);
  },
  { immediate: true },
);

// Debounced layout save
let layoutSaveTimer: ReturnType<typeof setTimeout> | null = null;

const persistLayout = async () => {
  const layoutItems: LayoutItem[] = gridLayout.value.map((item) => ({
    id: item.i,
    grid_x: item.x,
    grid_y: item.y,
    grid_w: item.w,
    grid_h: item.h,
  }));
  try {
    await widgetsService.saveLayout(layoutItems);
  } catch (error: any) {
    showError(
      t("common.error"),
      error.response?.data?.message || t("dashboard.saveLayoutFailed"),
    );
  }
};

const onLayoutUpdate = (
  newLayout: Array<{ i: string; x: number; y: number; w: number; h: number }>,
) => {
  gridLayout.value = newLayout;
  if (!isDragMode.value) return;
  if (layoutSaveTimer) clearTimeout(layoutSaveTimer);
  layoutSaveTimer = setTimeout(persistLayout, 500);
};

// Save immediately when exiting drag mode
watch(isDragMode, (newVal, oldVal) => {
  if (oldVal && !newVal) {
    // Toggled off — save now
    if (layoutSaveTimer) {
      clearTimeout(layoutSaveTimer);
      layoutSaveTimer = null;
    }
    persistLayout();
  }
});

const availableFields = computed(() => {
  if (!widgetForm.value.data_source) return [];
  const source = dataSources.value.find(
    (s) => s.name === widgetForm.value.data_source,
  );
  return source?.fields || [];
});

// Fetch data
const fetchWidgets = async () => {
  try {
    const response = await widgetsService.list();
    widgets.value = response.data?.widgets || [];
  } catch (error) {
    console.error("Failed to load widgets:", error);
    widgets.value = [];
  }
};

const fetchWidgetData = async () => {
  if (widgets.value.length === 0) return;

  isWidgetDataLoading.value = true;
  try {
    const { from, to } = dateRange.value;
    const response = await widgetsService.getAllData({ from, to });
    widgetData.value = response.data?.data || {};
  } catch (error) {
    console.error("Failed to load widget data:", error);
    widgetData.value = {};
  } finally {
    isWidgetDataLoading.value = false;
  }
};

const fetchDataSources = async () => {
  try {
    const response = await widgetsService.getDataSources();
    const data = response.data;
    dataSources.value = data.data_sources || [];
    metrics.value = data.metrics || [];
    displayTypes.value = data.display_types || [];
    operators.value = data.operators || [];
  } catch (error) {
    console.error("Failed to load data sources:", error);
  }
};

const fetchDashboardData = async () => {
  isLoading.value = true;
  try {
    await Promise.all([fetchWidgets(), fetchDataSources()]);
    await fetchWidgetData();
  } finally {
    isLoading.value = false;
  }
};

const applyCustomRange = () => {
  applyCustomRangeBase();
  fetchWidgetData();
};

// Widget CRUD
const openAddWidgetDialog = () => {
  isEditMode.value = false;
  editingWidgetId.value = null;
  widgetForm.value = {
    name: "",
    description: "",
    data_source: "",
    metric: "count",
    field: "",
    filters: [],
    display_type: "number",
    chart_type: "",
    group_by_field: "",
    show_change: true,
    color: "blue",
    size: "small",
    config: {},
    is_shared: false,
  };
  selectedShortcuts.value = [];
  isWidgetDialogOpen.value = true;
};

const openEditWidgetDialog = (widget: DashboardWidget) => {
  isEditMode.value = true;
  editingWidgetId.value = widget.id;
  widgetForm.value = {
    name: widget.name,
    description: widget.description,
    data_source: widget.data_source,
    metric: widget.metric,
    field: widget.field,
    filters: [...widget.filters],
    display_type: widget.display_type,
    chart_type: widget.chart_type,
    group_by_field: widget.group_by_field || "",
    show_change: widget.show_change,
    color: widget.color || "blue",
    size: widget.size,
    config: widget.config || {},
    is_shared: widget.is_shared,
  };
  // Populate selectedShortcuts from config
  if (widget.display_type === "shortcuts" && widget.config?.shortcuts) {
    selectedShortcuts.value = [...(widget.config.shortcuts as string[])];
  } else {
    selectedShortcuts.value = [];
  }
  isWidgetDialogOpen.value = true;
};

const addFilter = () => {
  widgetForm.value.filters.push({ field: "", operator: "equals", value: "" });
};

const removeFilter = (index: number) => {
  widgetForm.value.filters.splice(index, 1);
};

const saveWidget = async () => {
  const isShortcuts = widgetForm.value.display_type === "shortcuts";

  if (!widgetForm.value.name) {
    showError(t("dashboard.validationError"), t("dashboard.nameRequired"));
    return;
  }

  if (!isShortcuts && !widgetForm.value.data_source) {
    showError(
      t("dashboard.validationError"),
      t("dashboard.dataSourceRequired"),
    );
    return;
  }

  // Clean up empty filters
  const cleanFilters = widgetForm.value.filters.filter(
    (f) => f.field && f.operator && f.value,
  );

  // Build config
  let config: Record<string, any> = { ...widgetForm.value.config };
  if (isShortcuts) {
    config = { shortcuts: [...selectedShortcuts.value] };
  }

  const payload = {
    name: widgetForm.value.name,
    description: widgetForm.value.description,
    data_source: widgetForm.value.data_source,
    metric: widgetForm.value.metric,
    field: widgetForm.value.field,
    filters: cleanFilters,
    display_type: widgetForm.value.display_type,
    chart_type: widgetForm.value.chart_type,
    group_by_field: widgetForm.value.group_by_field,
    show_change: widgetForm.value.show_change,
    color: widgetForm.value.color,
    size: widgetForm.value.size,
    config,
    is_shared: widgetForm.value.is_shared,
  };

  isSavingWidget.value = true;
  try {
    if (isEditMode.value && editingWidgetId.value) {
      await widgetsService.update(editingWidgetId.value, payload);
      success(t("common.updatedSuccess", { resource: t("resources.Widget") }));
    } else {
      await widgetsService.create(payload);
      success(t("common.createdSuccess", { resource: t("resources.Widget") }));
    }
    isWidgetDialogOpen.value = false;
    await fetchWidgets();
    await fetchWidgetData();
  } catch (error: any) {
    showError(
      t("common.error"),
      error.response?.data?.message ||
        t("common.failedSave", { resource: t("resources.widget") }),
    );
  } finally {
    isSavingWidget.value = false;
  }
};

const openDeleteDialog = (widget: DashboardWidget) => {
  widgetToDelete.value = widget;
  deleteDialogOpen.value = true;
};

const confirmDeleteWidget = async () => {
  if (!widgetToDelete.value) return;

  try {
    await widgetsService.delete(widgetToDelete.value.id);
    success(t("common.deletedSuccess", { resource: t("resources.Widget") }));
    deleteDialogOpen.value = false;
    widgetToDelete.value = null;
    await fetchWidgets();
    await fetchWidgetData();
  } catch (error: any) {
    showError(
      t("common.error"),
      error.response?.data?.message ||
        t("common.failedDelete", { resource: t("resources.widget") }),
    );
  }
};

// Watch for range changes
watch(selectedRange, (newValue) => {
  if (newValue !== "custom") {
    fetchWidgetData();
  }
});

// Set default chart_type when display_type changes to chart
watch(
  () => widgetForm.value.display_type,
  (newVal) => {
    if (newVal === "chart" && !widgetForm.value.chart_type) {
      widgetForm.value.chart_type = "line";
    }
    if (newVal !== "chart") {
      widgetForm.value.chart_type = "";
    }
    if (newVal !== "chart" && newVal !== "table") {
      widgetForm.value.group_by_field = "";
    }
    if (newVal === "shortcuts") {
      widgetForm.value.data_source = "";
      widgetForm.value.metric = "count";
    }
  },
);

onMounted(() => {
  fetchDashboardData();
});
</script>

<template>
  <div class="flex flex-col h-full bg-background">
    <!-- Header -->
    <header class="border-b border-border bg-background/95 backdrop-blur-sm">
      <div class="flex h-16 items-center px-6">
        <div
          class="size-8 rounded-lg border bg-muted text-foreground flex items-center justify-center mr-3"
        >
          <LayoutDashboard class="size-4" />
        </div>
        <div class="flex-1">
          <h1 class="text-xl font-semibold text-foreground">
            {{ $t("dashboard.title") }}
          </h1>
          <p class="text-foreground/50">{{ $t("dashboard.subtitle") }}</p>
        </div>

        <!-- Time Range Filter -->
        <div class="flex items-center gap-2">
          <Button
            v-if="canCreateWidget"
            variant="outline"
            size="sm"
            @click="openAddWidgetDialog"
            class="bg-card border-border text-foreground/70 hover:bg-accent hover:text-foreground"
          >
            <Plus class="size-4 mr-2" />
            {{ $t("dashboard.addWidget") }}
          </Button>

          <Button
            v-if="canEditWidget && widgets.length > 1"
            variant="outline"
            size="sm"
            @click="isDragMode = !isDragMode"
            :class="[
              isDragMode
                ? 'bg-primary text-primary-foreground border-transparent hover:bg-primary/90 hover:text-primary-foreground'
                : 'bg-card border-border text-foreground/70 hover:bg-accent hover:text-foreground',
            ]"
          >
            <GripVertical class="size-4 mr-2" />
            {{ isDragMode ? $t("common.done") : $t("dashboard.editLayout") }}
          </Button>

          <DateRangePicker
            v-model:selected-range="selectedRange"
            v-model:custom-date-range="customDateRange"
            v-model:is-date-picker-open="isDatePickerOpen"
            :format-date-range-display="formatDateRangeDisplay"
            @apply-custom="applyCustomRange"
          />
        </div>
      </div>
    </header>

    <!-- Content -->
    <ScrollArea class="flex-1">
      <div class="p-6 gap-y-6">
        <!-- Loading Skeleton -->
        <div v-if="isLoading" class="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
          <div
            v-for="i in 4"
            :key="i"
            class="rounded-xl border border-border bg-card p-6"
          >
            <div
              class="flex flex-row items-center justify-between gap-y-0 pb-2"
            >
              <Skeleton class="h-4 w-24 bg-muted" />
              <Skeleton class="size-10 rounded-lg bg-muted" />
            </div>
            <div class="pt-2">
              <Skeleton class="h-8 w-20 mb-2 bg-muted" />
              <Skeleton class="h-3 w-32 bg-muted" />
            </div>
          </div>
        </div>

        <!-- Widget Grid Layout -->
        <GridLayout
          v-if="!isLoading && gridLayout.length > 0"
          :layout="gridLayout"
          :col-num="GRID_COLS"
          :row-height="GRID_ROW_HEIGHT"
          :margin="GRID_MARGIN"
          :is-draggable="isDragMode"
          :is-resizable="isDragMode"
          :vertical-compact="true"
          :use-css-transforms="true"
          @layout-updated="onLayoutUpdate"
        >
          <GridItem
            v-for="item in gridLayout"
            :key="item.i"
            :i="item.i"
            :x="item.x"
            :y="item.y"
            :w="item.w"
            :h="item.h"
            :min-w="2"
            :min-h="2"
            drag-allow-from=".widget-drag-handle"
          >
            <!-- Number widget card -->
            <WidgetNumberCard
              v-if="getWidgetById(item.i) && isNumberWidget(getWidgetById(item.i)!)"
              :widget="getWidgetById(item.i)!"
              :data="widgetData[item.i]"
              :is-loading="isWidgetDataLoading"
              :is-drag-mode="isDragMode"
              :can-edit-widget="canEditWidget"
              :can-delete-widget="canDeleteWidget"
              :comparison-period-label="comparisonPeriodLabel"
              @edit="openEditWidgetDialog"
              @delete="openDeleteDialog"
            />
            <WidgetChartCard
              v-else-if="getWidgetById(item.i) && isChartWidget(getWidgetById(item.i)!)"
              :widget="getWidgetById(item.i)!"
              :data="widgetData[item.i]"
              :is-loading="isWidgetDataLoading"
              :is-drag-mode="isDragMode"
              :can-edit-widget="canEditWidget"
              :can-delete-widget="canDeleteWidget"
              @edit="openEditWidgetDialog"
              @delete="openDeleteDialog"
            />
            <WidgetTableCard
              v-else-if="getWidgetById(item.i) && isTableWidget(getWidgetById(item.i)!)"
              :widget="getWidgetById(item.i)!"
              :data="widgetData[item.i]"
              :is-loading="isWidgetDataLoading"
              :is-drag-mode="isDragMode"
              :can-edit-widget="canEditWidget"
              :can-delete-widget="canDeleteWidget"
              @edit="openEditWidgetDialog"
              @delete="openDeleteDialog"
            />
            <WidgetShortcutsCard
              v-else-if="getWidgetById(item.i) && isShortcutsWidget(getWidgetById(item.i)!)"
              :widget="getWidgetById(item.i)!"
              :is-drag-mode="isDragMode"
              :can-edit-widget="canEditWidget"
              :can-delete-widget="canDeleteWidget"
              :item-w="item.w"
              @edit="openEditWidgetDialog"
              @delete="openDeleteDialog"
            />
          </GridItem>
        </GridLayout>
      </div>
    </ScrollArea>

    <!-- Widget Dialog -->
    <Dialog v-model:open="isWidgetDialogOpen">
      <DialogContent class="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle>{{
            isEditMode
              ? $t("dashboard.editWidget")
              : $t("dashboard.createWidget")
          }}</DialogTitle>
          <DialogDescription class="text-foreground/50">
            {{ $t("dashboard.widgetDialogDesc") }}
          </DialogDescription>
        </DialogHeader>

        <div class="gap-y-4 py-4">
          <!-- Name -->
          <div class="gap-y-2">
            <Label class="text-foreground/70"
              >{{ $t("dashboard.widgetName") }} *</Label
            >
            <Input
              v-model="widgetForm.name"
              :placeholder="$t('dashboard.widgetNamePlaceholder')"
              class="bg-card border-border text-foreground placeholder:text-muted-foreground"
            />
          </div>

          <!-- Description -->
          <div class="gap-y-2">
            <Label class="text-foreground/70">{{
              $t("dashboard.widgetDescription")
            }}</Label>
            <Textarea
              v-model="widgetForm.description"
              :placeholder="$t('dashboard.widgetDescriptionPlaceholder')"
              class="bg-card border-border text-foreground placeholder:text-muted-foreground"
              :rows="2"
            />
          </div>

          <!-- Data Source (hidden for shortcuts) -->
          <div v-if="widgetForm.display_type !== 'shortcuts'" class="gap-y-2">
            <Label class="text-foreground/70"
              >{{ $t("dashboard.dataSource") }} *</Label
            >
            <Select
              :model-value="widgetForm.data_source"
              @update:model-value="
                (val) => (widgetForm.data_source = String(val))
              "
            >
              <SelectTrigger>
                <SelectValue :placeholder="$t('dashboard.selectDataSource')" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem
                  v-for="source in dataSources"
                  :key="source.name"
                  :value="source.name"
                >
                  {{ source.label }}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>

          <!-- Metric (hidden for shortcuts and table) -->
          <div
            v-if="
              widgetForm.display_type !== 'shortcuts' &&
              widgetForm.display_type !== 'table'
            "
            class="gap-y-2"
          >
            <Label class="text-foreground/70">{{
              $t("dashboard.metric")
            }}</Label>
            <Select
              :model-value="widgetForm.metric"
              @update:model-value="(val) => (widgetForm.metric = String(val))"
            >
              <SelectTrigger>
                <SelectValue :placeholder="$t('dashboard.selectMetric')" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="count">{{
                  $t("dashboard.metricCount")
                }}</SelectItem>
                <SelectItem value="sum">{{
                  $t("dashboard.metricSum")
                }}</SelectItem>
                <SelectItem value="avg">{{
                  $t("dashboard.metricAverage")
                }}</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <!-- Display Type -->
          <div class="gap-y-2">
            <Label class="text-foreground/70">{{
              $t("dashboard.displayType")
            }}</Label>
            <Select
              :model-value="widgetForm.display_type"
              @update:model-value="
                (val) => (widgetForm.display_type = String(val))
              "
            >
              <SelectTrigger>
                <SelectValue :placeholder="$t('dashboard.selectDisplayType')" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="number">{{
                  $t("dashboard.displayNumber")
                }}</SelectItem>
                <SelectItem value="chart">{{
                  $t("dashboard.displayChart")
                }}</SelectItem>
                <SelectItem value="table">{{
                  $t("dashboard.displayTable")
                }}</SelectItem>
                <SelectItem value="shortcuts">{{
                  $t("dashboard.displayShortcuts")
                }}</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <!-- Chart Type (visible when display type is chart) -->
          <div v-if="widgetForm.display_type === 'chart'" class="gap-y-2">
            <Label class="text-foreground/70">{{
              $t("dashboard.chartType")
            }}</Label>
            <Select
              :model-value="widgetForm.chart_type"
              @update:model-value="
                (val) => (widgetForm.chart_type = String(val))
              "
            >
              <SelectTrigger>
                <SelectValue :placeholder="$t('dashboard.selectChartType')" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem
                  v-for="ct in chartTypeOptions"
                  :key="ct.value"
                  :value="ct.value"
                >
                  {{ ct.label }}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>

          <!-- Group By (visible when display type is chart or table, and data source is selected) -->
          <div
            v-if="
              (widgetForm.display_type === 'chart' ||
                widgetForm.display_type === 'table') &&
              widgetForm.data_source
            "
            class="gap-y-2"
          >
            <Label class="text-foreground/70">{{
              $t("dashboard.groupBy")
            }}</Label>
            <Select
              :model-value="widgetForm.group_by_field || 'none'"
              @update:model-value="
                (val) =>
                  (widgetForm.group_by_field =
                    val === 'none' ? '' : String(val))
              "
            >
              <SelectTrigger>
                <SelectValue :placeholder="$t('dashboard.noneTimeSeries')" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="none">
                  {{ $t("dashboard.noneTimeSeries") }}
                </SelectItem>
                <SelectItem
                  v-for="field in availableFields"
                  :key="field"
                  :value="field"
                >
                  {{ field }}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>

          <!-- Shortcuts selector (only for shortcuts display type) -->
          <div v-if="widgetForm.display_type === 'shortcuts'" class="gap-y-2">
            <Label class="text-foreground/70">{{
              $t("dashboard.selectShortcuts")
            }}</Label>
            <div class="gap-y-2 max-h-64 overflow-y-auto pr-1">
              <label
                v-for="(shortcut, key) in SHORTCUT_REGISTRY"
                :key="key"
                class="flex items-center gap-3 p-2 rounded-lg hover:bg-accent cursor-pointer"
              >
                <input
                  type="checkbox"
                  :value="key"
                  v-model="selectedShortcuts"
                  class="rounded border-input accent-primary"
                />
                <div class="flex items-center gap-2">
                  <div
                    class="size-8 rounded-lg border bg-muted text-foreground flex items-center justify-center"
                  >
                    <component :is="shortcut.icon" class="size-4" />
                  </div>
                  <span class="text-foreground/70">{{ shortcut.label }}</span>
                </div>
              </label>
            </div>
          </div>

          <!-- Filters (hidden for shortcuts) -->
          <div v-if="widgetForm.display_type !== 'shortcuts'" class="gap-y-2">
            <div class="flex items-center justify-between">
              <Label class="text-foreground/70"
                >{{ $t("dashboard.filters") }} ({{
                  widgetForm.filters.length
                }})</Label
              >
              <Button
                type="button"
                variant="outline"
                size="sm"
                @click.stop.prevent="addFilter"
                class="border-border text-foreground hover:bg-accent"
              >
                <Plus class="size-4 mr-1" />
                {{ $t("dashboard.addFilter") }}
              </Button>
            </div>
            <p
              v-if="!widgetForm.data_source && widgetForm.filters.length === 0"
              class="text-foreground/40"
            >
              {{ $t("dashboard.selectDataSourceFirst") }}
            </p>
            <div
              v-for="(filter, index) in widgetForm.filters"
              :key="index"
              class="flex items-center gap-2"
            >
              <div class="flex-1">
                <Select
                  :model-value="filter.field"
                  @update:model-value="(val) => (filter.field = String(val))"
                >
                  <SelectTrigger>
                    <SelectValue :placeholder="$t('dashboard.field')" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem
                      v-for="field in availableFields"
                      :key="field"
                      :value="field"
                    >
                      {{ field }}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div class="w-36">
                <Select
                  :model-value="filter.operator"
                  @update:model-value="(val) => (filter.operator = String(val))"
                >
                  <SelectTrigger>
                    <SelectValue :placeholder="$t('dashboard.operator')" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem
                      v-for="op in operators"
                      :key="op.value"
                      :value="op.value"
                    >
                      {{ op.label }}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <Input
                v-model="filter.value"
                :placeholder="$t('dashboard.value')"
                class="flex-1 bg-card border-border text-foreground placeholder:text-muted-foreground"
              />
              <Button
                variant="ghost"
                size="icon"
                @click="removeFilter(index)"
                class="text-foreground/50 hover:text-destructive shrink-0"
              >
                <X class="size-4" />
              </Button>
            </div>
          </div>

          <!-- Color (hidden for shortcuts and table) -->
          <div
            v-if="
              widgetForm.display_type !== 'shortcuts' &&
              widgetForm.display_type !== 'table'
            "
            class="gap-y-2"
          >
            <Label class="text-foreground/70">{{
              $t("dashboard.color")
            }}</Label>
            <Select
              :model-value="widgetForm.color"
              @update:model-value="(val) => (widgetForm.color = String(val))"
            >
              <SelectTrigger>
                <SelectValue :placeholder="$t('dashboard.selectColor')" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem
                  v-for="color in colorOptions"
                  :key="color.value"
                  :value="color.value"
                >
                  <span class="flex items-center gap-2">
                    <span
                      :class="['inline-block size-3 rounded-full', color.bg]"
                    ></span>
                    {{ color.label }}
                  </span>
                </SelectItem>
              </SelectContent>
            </Select>
          </div>

          <!-- Options -->
          <div class="flex items-center justify-between">
            <div
              v-if="
                widgetForm.display_type === 'number' ||
                widgetForm.display_type === 'percentage'
              "
              class="flex items-center gap-2"
            >
              <Switch v-model:checked="widgetForm.show_change" />
              <Label class="text-foreground/70">{{
                $t("dashboard.showPercentChange")
              }}</Label>
            </div>
            <div class="flex items-center gap-2">
              <Switch v-model:checked="widgetForm.is_shared" />
              <Label class="text-foreground/70">{{
                $t("dashboard.shareWithTeam")
              }}</Label>
            </div>
          </div>
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            @click="isWidgetDialogOpen = false"
            class="border-border text-foreground/70 hover:bg-accent"
          >
            {{ $t("common.cancel") }}
          </Button>
          <Button @click="saveWidget" :disabled="isSavingWidget">
            {{
              isSavingWidget
                ? $t("common.saving") + "..."
                : isEditMode
                  ? $t("common.update")
                  : $t("common.create")
            }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- Delete Confirmation Dialog -->
    <AlertDialog v-model:open="deleteDialogOpen">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle class="text-foreground">{{
            $t("dashboard.deleteWidgetTitle")
          }}</AlertDialogTitle>
          <AlertDialogDescription class="text-foreground/60">
            {{
              $t("dashboard.deleteWidgetConfirm", {
                name: widgetToDelete?.name,
              })
            }}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel
            class="bg-transparent border-border text-foreground/70 hover:bg-accent"
          >
            {{ $t("common.cancel") }}
          </AlertDialogCancel>
          <AlertDialogAction
            @click="confirmDeleteWidget"
            class="bg-red-600 text-white hover:bg-red-700"
          >
            {{ $t("common.delete") }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>

<style>
/* The grid is grid-layout-plus, which renders `vgl-*` class names. These rules
   were originally written against vue-grid-layout's `.vue-grid-item` /
   `.vue-resizable-handle`, which this library never emits — so none of the
   intended styling below had been applying.
   Rules are scoped under .vgl-layout so they out-specify the library's own
   stylesheet regardless of injection order. */

/* Drop placeholder. The library's defaults are a red fill at 20% opacity;
   these vars are its supported styling hook. Opacity must go back to 100%
   or it washes out the dashed border too. */
.vgl-layout {
  --vgl-placeholder-bg: transparent;
  --vgl-placeholder-opacity: 100%;
}

.vgl-layout .vgl-item--placeholder {
  border: 2px dashed color-mix(in oklab, var(--primary) 40%, transparent);
  border-radius: 0.75rem;
}

/* Resize handle: 20px hit area, 2px corner chevron. */
.vgl-layout .vgl-item__resizer {
  --vgl-resizer-size: 20px;
  --vgl-resizer-border-color: rgba(0, 0, 0, 0.2);
  --vgl-resizer-border-width: 2px;
  right: 4px;
  bottom: 4px;
}

.dark .vgl-layout .vgl-item__resizer {
  --vgl-resizer-border-color: rgba(255, 255, 255, 0.2);
}

/* The library draws the chevron across the full resizer box; shrink it to the
   8px corner mark this dashboard was designed with. */
.vgl-layout .vgl-item__resizer::before {
  top: auto;
  left: auto;
  right: 4px;
  bottom: 4px;
  width: 8px;
  height: 8px;
  border-radius: 0 0 2px 0;
}

/* Motion is removed app-wide. grid-layout-plus animates widget position via
   `transition-property: transform` on .vgl-item, the container height on
   .vgl-layout, and .1s on the placeholder — so widgets glide to their new slot
   after a drag. Kill all three; dragging still tracks the cursor (that is a
   live transform, not an interpolated one), the widget just snaps on drop. */
.vgl-item,
.vgl-item--placeholder,
.vgl-layout {
  transition: none !important;
}
</style>
