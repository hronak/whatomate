<script setup lang="ts">
import { ref, shallowRef, onMounted, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { GridLayout, GridItem } from 'grid-layout-plus'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle
} from '@/components/ui/alert-dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Switch } from '@/components/ui/switch'
import { widgetsService, type DashboardWidget, type WidgetData, type LayoutItem } from '@/services/api'
import { useAuthStore } from '@/stores/auth'
import {
  MessageSquare,
  Users,
  Bot,
  Send,
  TrendingUp,
  TrendingDown,
  Minus,
  Clock,
  LayoutDashboard,
  Plus,
  Pencil,
  Trash2,
  BarChart3,
  FileText,
  X,
  GripVertical,
  Megaphone,
  Settings,
  Contact,
  Workflow,
  Key,
  UserX,
  MessageSquareText,
  Webhook,
  ShieldCheck,
  Zap,
  Shield,
  LineChart,
  Tags
} from '@lucide/vue'
// Centralized Chart.js setup (registered once)
import { Line, Bar, Pie } from '@/lib/charts'
import { DateRangePicker } from '@/components/shared'
import { useDateRange } from '@/composables/useDateRange'
import { useAppToast } from '@/composables/useAppToast'

const { success, error: showError } = useAppToast()
const { t } = useI18n()
const authStore = useAuthStore()

// Permission checks
const canCreateWidget = computed(() => authStore.hasPermission('analytics', 'write'))
const canEditWidget = computed(() => authStore.hasPermission('analytics', 'write'))
const canDeleteWidget = computed(() => authStore.hasPermission('analytics', 'delete'))

// Widgets state
const widgets = ref<DashboardWidget[]>([])
const widgetData = shallowRef<Record<string, WidgetData>>({})

const isLoading = ref(true)
const isWidgetDataLoading = ref(false)

// Widget builder state
const isWidgetDialogOpen = ref(false)
const isEditMode = ref(false)
const editingWidgetId = ref<string | null>(null)
const isSavingWidget = ref(false)

// Delete dialog state
const deleteDialogOpen = ref(false)
const widgetToDelete = ref<DashboardWidget | null>(null)

const dataSources = ref<Array<{ name: string; label: string; fields: string[] }>>([])
const metrics = ref<string[]>([])
const displayTypes = ref<string[]>([])
const operators = ref<Array<{ value: string; label: string }>>([])

const widgetForm = ref({
  name: '',
  description: '',
  data_source: '',
  metric: 'count',
  field: '',
  filters: [] as Array<{ field: string; operator: string; value: string }>,
  display_type: 'number',
  chart_type: '',
  group_by_field: '',
  show_change: true,
  color: 'blue',
  size: 'small',
  config: {} as Record<string, any>,
  is_shared: false
})

// Selected shortcuts for shortcuts widget creation
const selectedShortcuts = ref<string[]>([])

// Shortcut registry
const SHORTCUT_REGISTRY = computed(() => ({
  chat: { label: t('dashboard.startChat'), to: '/chat', icon: MessageSquare, gradient: 'from-emerald-500 to-green-600' },
  campaigns: { label: t('nav.campaigns'), to: '/campaigns', icon: Megaphone, gradient: 'from-orange-500 to-amber-600' },
  templates: { label: t('nav.templates'), to: '/templates', icon: FileText, gradient: 'from-blue-500 to-cyan-600' },
  chatbot: { label: t('nav.chatbot'), to: '/chatbot', icon: Bot, gradient: 'from-purple-500 to-pink-600' },
  contacts: { label: t('nav.contacts'), to: '/settings/contacts', icon: Contact, gradient: 'from-cyan-500 to-blue-600' },
  flows: { label: t('nav.flows'), to: '/flows', icon: Workflow, gradient: 'from-indigo-500 to-violet-600' },
  transfers: { label: t('nav.transfers'), to: '/chatbot/transfers', icon: UserX, gradient: 'from-rose-500 to-red-600' },
  agentAnalytics: { label: t('nav.agentAnalytics'), to: '/analytics/agents', icon: BarChart3, gradient: 'from-teal-500 to-cyan-600' },
  metaInsights: { label: t('nav.metaInsights'), to: '/analytics/meta-insights', icon: LineChart, gradient: 'from-sky-500 to-blue-600' },
  settings: { label: t('nav.settings'), to: '/settings', icon: Settings, gradient: 'from-gray-500 to-zinc-600' },
  accounts: { label: t('nav.accounts'), to: '/settings/accounts', icon: Users, gradient: 'from-violet-500 to-purple-600' },
  cannedResponses: { label: t('nav.cannedResponses'), to: '/settings/canned-responses', icon: MessageSquareText, gradient: 'from-amber-500 to-yellow-600' },
  tags: { label: t('nav.tags'), to: '/settings/tags', icon: Tags, gradient: 'from-pink-500 to-rose-600' },
  teams: { label: t('nav.teams'), to: '/settings/teams', icon: Users, gradient: 'from-lime-500 to-green-600' },
  users: { label: t('nav.users'), to: '/settings/users', icon: Users, gradient: 'from-fuchsia-500 to-pink-600' },
  roles: { label: t('nav.roles'), to: '/settings/roles', icon: Shield, gradient: 'from-slate-500 to-gray-600' },
  apiKeys: { label: t('nav.apiKeys'), to: '/settings/api-keys', icon: Key, gradient: 'from-yellow-500 to-orange-600' },
  webhooks: { label: t('nav.webhooks'), to: '/settings/webhooks', icon: Webhook, gradient: 'from-red-500 to-rose-600' },
  customActions: { label: t('nav.customActions'), to: '/settings/custom-actions', icon: Zap, gradient: 'from-amber-500 to-orange-600' },
  sso: { label: t('nav.sso'), to: '/settings/sso', icon: ShieldCheck, gradient: 'from-emerald-500 to-teal-600' },
}))

// Color options
const colorOptions = computed(() => [
  { value: 'blue', label: t('dashboard.colorBlue'), bg: 'bg-blue-500/20', text: 'text-blue-400' },
  { value: 'green', label: t('dashboard.colorGreen'), bg: 'bg-emerald-500/20', text: 'text-emerald-400' },
  { value: 'purple', label: t('dashboard.colorPurple'), bg: 'bg-purple-500/20', text: 'text-purple-400' },
  { value: 'orange', label: t('dashboard.colorOrange'), bg: 'bg-orange-500/20', text: 'text-orange-400' },
  { value: 'red', label: t('dashboard.colorRed'), bg: 'bg-red-500/20', text: 'text-red-400' },
  { value: 'cyan', label: t('dashboard.colorCyan'), bg: 'bg-cyan-500/20', text: 'text-cyan-400' }
])

// Chart type options
const chartTypeOptions = computed(() => [
  { value: 'line', label: t('dashboard.chartLine') },
  { value: 'bar', label: t('dashboard.chartBar') },
  { value: 'pie', label: t('dashboard.chartPie') }
])

// Chart color palette for pie charts
const chartColors = [
  'rgba(59, 130, 246, 0.8)',
  'rgba(16, 185, 129, 0.8)',
  'rgba(245, 158, 11, 0.8)',
  'rgba(139, 92, 246, 0.8)',
  'rgba(239, 68, 68, 0.8)',
  'rgba(6, 182, 212, 0.8)',
  'rgba(236, 72, 153, 0.8)',
  'rgba(234, 179, 8, 0.8)'
]

const getChartComponentData = (widget: DashboardWidget) => {
  const data = widgetData.value[widget.id]
  if (!data) return { labels: [], datasets: [] }

  const chartData = data.chart_data || []
  const dataPoints = data.data_points || []
  const groupedSeries = data.grouped_series

  // Grouped line chart: multiple datasets from grouped_series
  if (widget.chart_type === 'line' && groupedSeries && groupedSeries.datasets.length > 0) {
    return {
      labels: groupedSeries.labels,
      datasets: groupedSeries.datasets.map((ds, i) => ({
        label: ds.label,
        data: ds.data,
        borderColor: chartColors[i % chartColors.length],
        backgroundColor: chartColors[i % chartColors.length].replace('0.8)', '0.1)'),
        fill: false,
        tension: 0.3
      }))
    }
  }

  // Bar/Pie with group_by uses data_points (group → count)
  if (widget.chart_type === 'pie') {
    const source = dataPoints.length > 0 ? dataPoints : chartData
    return {
      labels: source.map((d: { label: string }) => d.label),
      datasets: [{
        data: source.map((d: { value: number }) => d.value),
        backgroundColor: chartColors.slice(0, source.length),
        borderWidth: 0
      }]
    }
  }

  if (widget.chart_type === 'bar' && dataPoints.length > 0) {
    return {
      labels: dataPoints.map((d: { label: string }) => d.label),
      datasets: [{
        label: widget.name,
        data: dataPoints.map((d: { value: number }) => d.value),
        backgroundColor: dataPoints.map((_: any, i: number) => chartColors[i % chartColors.length]),
        borderWidth: 0
      }]
    }
  }

  // Default: line and bar charts use time-series chart_data
  const colorMap: Record<string, string> = {
    blue: 'rgb(59, 130, 246)',
    green: 'rgb(16, 185, 129)',
    purple: 'rgb(139, 92, 246)',
    orange: 'rgb(245, 158, 11)',
    red: 'rgb(239, 68, 68)',
    cyan: 'rgb(6, 182, 212)'
  }
  const borderColor = colorMap[widget.color] || colorMap.blue

  return {
    labels: chartData.map((d: { label: string }) => d.label),
    datasets: [{
      label: widget.name,
      data: chartData.map((d: { value: number }) => d.value),
      borderColor,
      backgroundColor: widget.chart_type === 'bar'
        ? borderColor.replace('rgb', 'rgba').replace(')',', 0.8)')
        : borderColor.replace('rgb', 'rgba').replace(')',', 0.1)'),
      fill: widget.chart_type === 'line',
      tension: 0.3
    }]
  }
}

const lineBarChartOptions = {
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: { display: true, position: 'top' as const }
  },
  scales: {
    y: { beginAtZero: true }
  }
}

const pieChartOptions = {
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: { position: 'bottom' as const }
  }
}

// Time range filter
const {
  selectedRange,
  customDateRange,
  isDatePickerOpen,
  dateRange,
  formatDateRangeDisplay,
  applyCustomRange: applyCustomRangeBase,
} = useDateRange({ storageKey: 'dashboard' })

const comparisonPeriodLabel = computed(() => {
  switch (selectedRange.value) {
    case 'today':
      return t('dashboard.fromYesterday')
    case '7days':
      return t('dashboard.fromPrevious7Days')
    case '30days':
      return t('dashboard.fromPrevious30Days')
    case 'this_month':
      return t('dashboard.fromLastMonth')
    case 'custom':
      return t('dashboard.fromPreviousPeriod')
    default:
      return t('dashboard.fromPreviousPeriod')
  }
})


const formatNumber = (num: number): string => {
  if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M'
  if (num >= 1000) return (num / 1000).toFixed(1) + 'K'
  return Math.round(num).toString()
}

const formatTime = (dateStr: string): string => {
  const date = new Date(dateStr)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffMins = Math.floor(diffMs / 60000)
  const diffHours = Math.floor(diffMs / 3600000)
  const diffDays = Math.floor(diffMs / 86400000)

  if (diffMins < 1) return t('dashboard.justNow')
  if (diffMins < 60) return t('dashboard.minutesAgo', { count: diffMins })
  if (diffHours < 24) return t('dashboard.hoursAgo', { count: diffHours })
  return t('dashboard.daysAgo', { count: diffDays })
}

const getWidgetColor = (color: string) => {
  const gradientMap: Record<string, string> = {
    blue: 'bg-linear-to-r from-blue-500/60 to-blue-500/0',
    green: 'bg-linear-to-r from-emerald-500/60 to-emerald-500/0',
    purple: 'bg-linear-to-r from-violet-500/60 to-violet-500/0',
    orange: 'bg-linear-to-r from-amber-500/60 to-amber-500/0',
    red: 'bg-linear-to-r from-rose-500/60 to-rose-500/0',
    cyan: 'bg-linear-to-r from-cyan-500/60 to-cyan-500/0'
  }
  const colorConfig = colorOptions.value.find(c => c.value === color) || colorOptions.value[0]
  return { ...colorConfig, gradient: gradientMap[colorConfig.value] || gradientMap.blue }
}

const getWidgetIcon = (dataSource: string) => {
  switch (dataSource) {
    case 'messages':
      return MessageSquare
    case 'contacts':
      return Users
    case 'sessions':
      return Bot
    case 'campaigns':
      return Send
    case 'transfers':
      return Users
    default:
      return BarChart3
  }
}

// Grid layout state
const GRID_COLS = 12
const GRID_ROW_HEIGHT = 40
const GRID_MARGIN: [number, number] = [16, 16]

const isDragMode = ref(false)
const gridLayout = ref<Array<{ i: string; x: number; y: number; w: number; h: number }>>([])

const isChartWidget = (widget: DashboardWidget) => widget.display_type === 'chart'
const isTableWidget = (widget: DashboardWidget) => widget.display_type === 'table'
const isShortcutsWidget = (widget: DashboardWidget) => widget.display_type === 'shortcuts'
const isNumberWidget = (widget: DashboardWidget) => !isChartWidget(widget) && !isTableWidget(widget) && !isShortcutsWidget(widget)

const getWidgetById = (id: string): DashboardWidget | undefined => {
  return widgets.value.find(w => w.id === id)
}

const computeGridLayout = (widgetList: DashboardWidget[]) => {
  const layout: Array<{ i: string; x: number; y: number; w: number; h: number }> = []

  // Separate positioned (grid_w > 0) from legacy (grid_w === 0) widgets
  const positioned = widgetList.filter(w => w.grid_w > 0)
  const legacy = widgetList.filter(w => w.grid_w === 0)

  // Add positioned widgets as-is
  for (const w of positioned) {
    layout.push({ i: w.id, x: w.grid_x, y: w.grid_y, w: w.grid_w, h: w.grid_h })
  }

  // Auto-position legacy widgets
  if (legacy.length > 0) {
    // Find the max y used by positioned widgets to place legacy below
    let nextY = 0
    if (positioned.length > 0) {
      nextY = Math.max(...positioned.map(w => w.grid_y + w.grid_h))
    }

    let curX = 0
    let curY = nextY

    // Number widgets first, then chart/table/shortcuts widgets
    const legacyNumber = legacy.filter(w => !['chart', 'table', 'shortcuts'].includes(w.display_type))
    const legacyLarge = legacy.filter(w => ['chart', 'table', 'shortcuts'].includes(w.display_type))

    for (const w of legacyNumber) {
      const itemW = 3
      const itemH = 3
      if (curX + itemW > GRID_COLS) {
        curX = 0
        curY += itemH
      }
      layout.push({ i: w.id, x: curX, y: curY, w: itemW, h: itemH })
      curX += itemW
    }

    // Move to next row for large widgets
    if (legacyNumber.length > 0 && legacyLarge.length > 0) {
      curX = 0
      curY += 3
    }

    for (const w of legacyLarge) {
      let itemW = 6
      let itemH = 5
      if (w.display_type === 'table' || w.display_type === 'shortcuts') {
        itemW = 6
        itemH = 8
      }
      if (curX + itemW > GRID_COLS) {
        curX = 0
        curY += itemH
      }
      layout.push({ i: w.id, x: curX, y: curY, w: itemW, h: itemH })
      curX += itemW
    }
  }

  return layout
}

// Rebuild grid layout when widgets change
watch(widgets, (val) => {
  gridLayout.value = computeGridLayout(val)
}, { immediate: true })

// Debounced layout save
let layoutSaveTimer: ReturnType<typeof setTimeout> | null = null

const persistLayout = async () => {
  const layoutItems: LayoutItem[] = gridLayout.value.map(item => ({
    id: item.i,
    grid_x: item.x,
    grid_y: item.y,
    grid_w: item.w,
    grid_h: item.h
  }))
  try {
    await widgetsService.saveLayout(layoutItems)
  } catch (error: any) {
    showError(t('common.error'), error.response?.data?.message || t('dashboard.saveLayoutFailed'))
  }
}

const onLayoutUpdate = (newLayout: Array<{ i: string; x: number; y: number; w: number; h: number }>) => {
  gridLayout.value = newLayout
  if (!isDragMode.value) return
  if (layoutSaveTimer) clearTimeout(layoutSaveTimer)
  layoutSaveTimer = setTimeout(persistLayout, 500)
}

// Save immediately when exiting drag mode
watch(isDragMode, (newVal, oldVal) => {
  if (oldVal && !newVal) {
    // Toggled off — save now
    if (layoutSaveTimer) {
      clearTimeout(layoutSaveTimer)
      layoutSaveTimer = null
    }
    persistLayout()
  }
})

const availableFields = computed(() => {
  if (!widgetForm.value.data_source) return []
  const source = dataSources.value.find(s => s.name === widgetForm.value.data_source)
  return source?.fields || []
})

// Fetch data
const fetchWidgets = async () => {
  try {
    const response = await widgetsService.list()
    widgets.value = (response.data as any).data?.widgets || []
  } catch (error) {
    console.error('Failed to load widgets:', error)
    widgets.value = []
  }
}

const fetchWidgetData = async () => {
  if (widgets.value.length === 0) return

  isWidgetDataLoading.value = true
  try {
    const { from, to } = dateRange.value
    const response = await widgetsService.getAllData({ from, to })
    widgetData.value = (response.data as any).data?.data || {}
  } catch (error) {
    console.error('Failed to load widget data:', error)
    widgetData.value = {}
  } finally {
    isWidgetDataLoading.value = false
  }
}

const fetchDataSources = async () => {
  try {
    const response = await widgetsService.getDataSources()
    const data = (response.data as any).data || response.data
    dataSources.value = data.data_sources || []
    metrics.value = data.metrics || []
    displayTypes.value = data.display_types || []
    operators.value = data.operators || []
  } catch (error) {
    console.error('Failed to load data sources:', error)
  }
}

const fetchDashboardData = async () => {
  isLoading.value = true
  try {
    await Promise.all([
      fetchWidgets(),
      fetchDataSources()
    ])
    await fetchWidgetData()
  } finally {
    isLoading.value = false
  }
}

const applyCustomRange = () => {
  applyCustomRangeBase()
  fetchWidgetData()
}

// Widget CRUD
const openAddWidgetDialog = () => {
  isEditMode.value = false
  editingWidgetId.value = null
  widgetForm.value = {
    name: '',
    description: '',
    data_source: '',
    metric: 'count',
    field: '',
    filters: [],
    display_type: 'number',
    chart_type: '',
    group_by_field: '',
    show_change: true,
    color: 'blue',
    size: 'small',
    config: {},
    is_shared: false
  }
  selectedShortcuts.value = []
  isWidgetDialogOpen.value = true
}

const openEditWidgetDialog = (widget: DashboardWidget) => {
  isEditMode.value = true
  editingWidgetId.value = widget.id
  widgetForm.value = {
    name: widget.name,
    description: widget.description,
    data_source: widget.data_source,
    metric: widget.metric,
    field: widget.field,
    filters: [...widget.filters],
    display_type: widget.display_type,
    chart_type: widget.chart_type,
    group_by_field: widget.group_by_field || '',
    show_change: widget.show_change,
    color: widget.color || 'blue',
    size: widget.size,
    config: widget.config || {},
    is_shared: widget.is_shared
  }
  // Populate selectedShortcuts from config
  if (widget.display_type === 'shortcuts' && widget.config?.shortcuts) {
    selectedShortcuts.value = [...widget.config.shortcuts as string[]]
  } else {
    selectedShortcuts.value = []
  }
  isWidgetDialogOpen.value = true
}

const addFilter = () => {
  widgetForm.value.filters.push({ field: '', operator: 'equals', value: '' })
}

const removeFilter = (index: number) => {
  widgetForm.value.filters.splice(index, 1)
}

const saveWidget = async () => {
  const isShortcuts = widgetForm.value.display_type === 'shortcuts'

  if (!widgetForm.value.name) {
    showError(t('dashboard.validationError'), t('dashboard.nameRequired'))
    return
  }

  if (!isShortcuts && !widgetForm.value.data_source) {
    showError(t('dashboard.validationError'), t('dashboard.dataSourceRequired'))
    return
  }

  // Clean up empty filters
  const cleanFilters = widgetForm.value.filters.filter(f => f.field && f.operator && f.value)

  // Build config
  let config: Record<string, any> = { ...widgetForm.value.config }
  if (isShortcuts) {
    config = { shortcuts: [...selectedShortcuts.value] }
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
    is_shared: widgetForm.value.is_shared
  }

  isSavingWidget.value = true
  try {
    if (isEditMode.value && editingWidgetId.value) {
      await widgetsService.update(editingWidgetId.value, payload)
      success(t('common.updatedSuccess', { resource: t('resources.Widget') }))
    } else {
      await widgetsService.create(payload)
      success(t('common.createdSuccess', { resource: t('resources.Widget') }))
    }
    isWidgetDialogOpen.value = false
    await fetchWidgets()
    await fetchWidgetData()
  } catch (error: any) {
    showError(t('common.error'), error.response?.data?.message || t('common.failedSave', { resource: t('resources.widget') }))
  } finally {
    isSavingWidget.value = false
  }
}

const openDeleteDialog = (widget: DashboardWidget) => {
  widgetToDelete.value = widget
  deleteDialogOpen.value = true
}

const confirmDeleteWidget = async () => {
  if (!widgetToDelete.value) return

  try {
    await widgetsService.delete(widgetToDelete.value.id)
    success(t('common.deletedSuccess', { resource: t('resources.Widget') }))
    deleteDialogOpen.value = false
    widgetToDelete.value = null
    await fetchWidgets()
    await fetchWidgetData()
  } catch (error: any) {
    showError(t('common.error'), error.response?.data?.message || t('common.failedDelete', { resource: t('resources.widget') }))
  }
}

// Watch for range changes
watch(selectedRange, (newValue) => {
  if (newValue !== 'custom') {
    fetchWidgetData()
  }
})

// Set default chart_type when display_type changes to chart
watch(() => widgetForm.value.display_type, (newVal) => {
  if (newVal === 'chart' && !widgetForm.value.chart_type) {
    widgetForm.value.chart_type = 'line'
  }
  if (newVal !== 'chart') {
    widgetForm.value.chart_type = ''
  }
  if (newVal !== 'chart' && newVal !== 'table') {
    widgetForm.value.group_by_field = ''
  }
  if (newVal === 'shortcuts') {
    widgetForm.value.data_source = ''
    widgetForm.value.metric = 'count'
  }
})

onMounted(() => {
  fetchDashboardData()
})
</script>

<template>
  <div class="flex flex-col h-full bg-background">
    <!-- Header -->
    <header class="border-b border-border bg-white/95 dark:bg-[#0a0a0b]/95 backdrop-blur-sm">
      <div class="flex h-16 items-center px-6">
        <div class="size-8 rounded-lg bg-linear-to-br from-emerald-500 to-green-600 flex items-center justify-center mr-3 shadow-lg shadow-emerald-500/20">
          <LayoutDashboard class="size-4 text-white" />
        </div>
        <div class="flex-1">
          <h1 class="text-xl font-semibold text-foreground">{{ $t('dashboard.title') }}</h1>
          <p class="text-foreground/50">{{ $t('dashboard.subtitle') }}</p>
        </div>

        <!-- Time Range Filter -->
        <div class="flex items-center gap-2">
          <Button v-if="canCreateWidget" variant="outline" size="sm" @click="openAddWidgetDialog" class="bg-card border-border text-foreground/70 hover:bg-accent hover:text-foreground">
            <Plus class="size-4 mr-2" />
            {{ $t('dashboard.addWidget') }}
          </Button>

          <Button
            v-if="canEditWidget && widgets.length > 1"
            variant="outline"
            size="sm"
            @click="isDragMode = !isDragMode"
            :class="[ isDragMode ? 'bg-emerald-500/20 border-emerald-500/40 text-emerald-400 hover:bg-emerald-500/30 hover:text-emerald-300' : 'bg-card border-border text-foreground/70 hover:bg-accent hover:text-foreground' ]"
          >
            <GripVertical class="size-4 mr-2" />
            {{ isDragMode ? $t('common.done') : $t('dashboard.editLayout') }}
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
      <div class="p-6 space-y-6">
        <!-- Loading Skeleton -->
        <div v-if="isLoading" class="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
          <div v-for="i in 4" :key="i" class="rounded-xl border border-border bg-card p-6">
            <div class="flex flex-row items-center justify-between space-y-0 pb-2">
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
            <div
              v-if="getWidgetById(item.i) && isNumberWidget(getWidgetById(item.i)!)"
              class="group relative h-full card-depth rounded-xl border border-border bg-card p-6 hover:bg-accent transition-colors overflow-hidden"
            >
              <!-- Gradient accent bar -->
              <div :class="['absolute top-0 inset-x-0 h-0.5', getWidgetColor(getWidgetById(item.i)!.color).gradient]" />

              <!-- Drag handle indicator -->
              <div v-if="isDragMode" class="widget-drag-handle absolute top-2 left-2 text-foreground/20 cursor-grab active:cursor-grabbing z-10">
                <GripVertical class="size-4" />
              </div>

              <div class="flex flex-row items-start justify-between space-y-0 pb-2">
                <div class="flex-1">
                  <span class="font-medium text-foreground/50">
                    {{ getWidgetById(item.i)!.name }}
                  </span>
                </div>
                <div class="flex items-center gap-2">
                  <!-- Actions - hidden in drag mode -->
                  <div v-if="!isDragMode && (canEditWidget || canDeleteWidget)" class="flex items-center gap-1 opacity-0 group-hover:opacity-100">
                    <Button
                      v-if="canEditWidget"
                      variant="ghost"
                      size="icon"
                      class="size-6 text-foreground/20 hover:text-foreground hover:bg-accent"
                      @click.stop="openEditWidgetDialog(getWidgetById(item.i)!)"
                      :title="$t('dashboard.editWidgetTooltip')"
                    >
                      <Pencil class="size-3" />
                    </Button>
                    <Button
                      v-if="canDeleteWidget"
                      variant="ghost"
                      size="icon"
                      class="size-6 text-foreground/20 hover:text-destructive hover:bg-destructive/10"
                      @click.stop="openDeleteDialog(getWidgetById(item.i)!)"
                      :title="$t('dashboard.deleteWidgetTooltip')"
                    >
                      <Trash2 class="size-3" />
                    </Button>
                  </div>
                  <!-- Icon -->
                  <div :class="['size-10 rounded-lg flex items-center justify-center', getWidgetColor(getWidgetById(item.i)!.color).bg]">
                    <component :is="getWidgetIcon(getWidgetById(item.i)!.data_source)" :class="['size-5', getWidgetColor(getWidgetById(item.i)!.color).text]" />
                  </div>
                </div>
              </div>

              <div class="pt-2">
                <div class="text-3xl font-bold text-foreground">
                  <template v-if="isWidgetDataLoading">
                    <Skeleton class="h-8 w-20 bg-muted" />
                  </template>
                  <template v-else>
                    <span>{{ formatNumber(widgetData[item.i]?.value || 0) }}</span>
                  </template>
                </div>
                <div v-if="getWidgetById(item.i)!.show_change && widgetData[item.i]" class="flex items-center text-foreground/40 mt-1">
                  <component
                    :is="widgetData[item.i]?.change > 0 ? TrendingUp : widgetData[item.i]?.change < 0 ? TrendingDown : Minus"
                    :class="[
                      'size-3 mr-1',
                      widgetData[item.i]?.change > 0 ? 'text-emerald-400' : widgetData[item.i]?.change < 0 ? 'text-red-400' : 'text-foreground/30'
                    ]"
                  />
                  <span :class="widgetData[item.i]?.change > 0 ? 'text-emerald-400' : widgetData[item.i]?.change < 0 ? 'text-red-400' : 'text-foreground/30'">
                    {{ Math.abs(widgetData[item.i]?.change || 0).toFixed(1) }}%
                  </span>
                  <span class="ml-1">{{ comparisonPeriodLabel }}</span>
                </div>
              </div>
            </div>

            <!-- Chart widget card -->
            <div
              v-else-if="getWidgetById(item.i) && isChartWidget(getWidgetById(item.i)!)"
              class="group relative h-full flex flex-col card-depth rounded-xl border border-border bg-card p-6 hover:bg-accent transition-colors overflow-hidden"
            >
              <!-- Drag handle indicator -->
              <div v-if="isDragMode" class="widget-drag-handle absolute top-2 left-2 text-foreground/20 cursor-grab active:cursor-grabbing z-10">
                <GripVertical class="size-4" />
              </div>

              <div class="flex flex-row items-center justify-between pb-2">
                <div>
                  <span class="font-medium text-foreground/50">{{ getWidgetById(item.i)!.name }}</span>
                  <p v-if="getWidgetById(item.i)!.description" class="text-foreground/30 mt-0.5">{{ getWidgetById(item.i)!.description }}</p>
                </div>
                <div class="flex items-center gap-2">
                  <div v-if="!isDragMode && (canEditWidget || canDeleteWidget)" class="flex items-center gap-1 opacity-0 group-hover:opacity-100">
                    <Button
                      v-if="canEditWidget"
                      variant="ghost"
                      size="icon"
                      class="size-6 text-foreground/20 hover:text-foreground hover:bg-accent"
                      @click.stop="openEditWidgetDialog(getWidgetById(item.i)!)"
                      :title="$t('dashboard.editWidgetTooltip')"
                    >
                      <Pencil class="size-3" />
                    </Button>
                    <Button
                      v-if="canDeleteWidget"
                      variant="ghost"
                      size="icon"
                      class="size-6 text-foreground/20 hover:text-destructive hover:bg-destructive/10"
                      @click.stop="openDeleteDialog(getWidgetById(item.i)!)"
                      :title="$t('dashboard.deleteWidgetTooltip')"
                    >
                      <Trash2 class="size-3" />
                    </Button>
                  </div>
                  <div :class="['size-10 rounded-lg flex items-center justify-center', getWidgetColor(getWidgetById(item.i)!.color).bg]">
                    <component :is="getWidgetIcon(getWidgetById(item.i)!.data_source)" :class="['size-5', getWidgetColor(getWidgetById(item.i)!.color).text]" />
                  </div>
                </div>
              </div>
              <div class="flex-1 min-h-0 pt-2">
                <template v-if="isWidgetDataLoading">
                  <Skeleton class="size-full bg-muted" />
                </template>
                <template v-else-if="(widgetData[item.i]?.chart_data?.length || 0) > 0 || (widgetData[item.i]?.data_points?.length || 0) > 0 || (widgetData[item.i]?.grouped_series?.datasets?.length || 0) > 0">
                  <Line v-if="getWidgetById(item.i)!.chart_type === 'line'" :data="getChartComponentData(getWidgetById(item.i)!)" :options="lineBarChartOptions" />
                  <Bar v-else-if="getWidgetById(item.i)!.chart_type === 'bar'" :data="getChartComponentData(getWidgetById(item.i)!)" :options="lineBarChartOptions" />
                  <Pie v-else-if="getWidgetById(item.i)!.chart_type === 'pie'" :data="getChartComponentData(getWidgetById(item.i)!)" :options="pieChartOptions" />
                </template>
                <template v-else>
                  <div class="h-full flex items-center justify-center text-foreground/40">
                    {{ $t('common.noData') }}
                  </div>
                </template>
              </div>
            </div>

            <!-- Table widget card -->
            <div
              v-else-if="getWidgetById(item.i) && isTableWidget(getWidgetById(item.i)!)"
              class="group relative h-full flex flex-col card-depth rounded-xl border border-border bg-card hover:bg-accent transition-colors overflow-hidden"
            >
              <!-- Drag handle -->
              <div v-if="isDragMode" class="widget-drag-handle absolute top-2 left-2 text-foreground/20 cursor-grab active:cursor-grabbing z-10">
                <GripVertical class="size-4" />
              </div>

              <div class="p-6 pb-3 flex flex-row items-center justify-between">
                <div>
                  <span class="font-medium text-foreground/50">{{ getWidgetById(item.i)!.name }}</span>
                  <p v-if="getWidgetById(item.i)!.description" class="text-foreground/30 mt-0.5">{{ getWidgetById(item.i)!.description }}</p>
                </div>
                <div class="flex items-center gap-2">
                  <div v-if="!isDragMode && (canEditWidget || canDeleteWidget)" class="flex items-center gap-1 opacity-0 group-hover:opacity-100">
                    <Button v-if="canEditWidget" variant="ghost" size="icon" class="size-6 text-foreground/20 hover:text-foreground hover:bg-accent" @click.stop="openEditWidgetDialog(getWidgetById(item.i)!)" :title="$t('dashboard.editWidgetTooltip')">
                      <Pencil class="size-3" />
                    </Button>
                    <Button v-if="canDeleteWidget" variant="ghost" size="icon" class="size-6 text-foreground/20 hover:text-destructive hover:bg-destructive/10" @click.stop="openDeleteDialog(getWidgetById(item.i)!)" :title="$t('dashboard.deleteWidgetTooltip')">
                      <Trash2 class="size-3" />
                    </Button>
                  </div>
                </div>
              </div>

              <div class="flex-1 min-h-0 overflow-auto px-6 pb-6">
                <template v-if="isWidgetDataLoading">
                  <Skeleton class="size-full bg-muted" />
                </template>
                <!-- Grouped table (group_by set) -->
                <template v-else-if="getWidgetById(item.i)!.group_by_field && widgetData[item.i]?.data_points?.length">
                  <table class="w-full">
                    <thead>
                      <tr class="border-b border-border">
                        <th class="text-left py-2 font-medium text-foreground/40 uppercase">{{ getWidgetById(item.i)!.group_by_field }}</th>
                        <th class="text-right py-2 font-medium text-foreground/40 uppercase">{{ $t('dashboard.count') }}</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-for="dp in widgetData[item.i]?.data_points" :key="dp.label" class="border-b border-border">
                        <td class="py-2 text-foreground/70">{{ dp.label }}</td>
                        <td class="py-2 text-right text-foreground font-medium">{{ dp.value }}</td>
                      </tr>
                    </tbody>
                  </table>
                </template>
                <!-- Row list (no group_by) -->
                <template v-else-if="widgetData[item.i]?.table_rows?.length">
                  <div class="space-y-3">
                    <div
                      v-for="row in widgetData[item.i]?.table_rows"
                      :key="row.id"
                      class="flex items-start gap-3 p-3 rounded-lg hover:bg-accent transition-colors"
                    >
                      <div
                        :class="[
                          'size-10 rounded-lg flex items-center justify-center font-medium shrink-0',
                          row.direction === 'incoming' ? 'bg-linear-to-br from-emerald-500 to-green-600 text-white' : 'bg-linear-to-br from-blue-500 to-cyan-600 text-white'
                        ]"
                      >
                        {{ row.label.split('').map((n: string) => n[0]).join('').slice(0, 2).toUpperCase() }}
                      </div>
                      <div class="flex-1 min-w-0">
                        <div class="flex items-center justify-between">
                          <p class="font-medium truncate text-foreground">{{ row.label }}</p>
                          <span class="text-foreground/40 flex items-center gap-1 shrink-0">
                            <Clock class="size-3" />
                            {{ formatTime(row.created_at) }}
                          </span>
                        </div>
                        <p class="text-foreground/50 truncate">{{ row.sub_label }}</p>
                        <div class="flex items-center gap-2 mt-1">
                          <span
                            v-if="row.direction"
                            :class="[
                              'px-1.5 py-0.5 rounded-full font-medium',
                              row.direction === 'incoming' ? 'bg-success/20 text-success' : 'bg-info/20 text-info'
                            ]"
                          >
                            {{ row.direction }}
                          </span>
                          <span
                            v-if="row.status"
                            :class="[
                              'px-1.5 py-0.5 rounded-full font-medium',
                              row.status === 'delivered' ? 'bg-info/20 text-info' :
                              row.status === 'read' ? 'bg-success/20 text-success' :
                              row.status === 'failed' ? 'bg-destructive/20 text-destructive' :
                              'bg-muted text-foreground/50'
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
                    {{ $t('common.noData') }}
                  </div>
                </template>
              </div>
            </div>

            <!-- Shortcuts widget card -->
            <div
              v-else-if="getWidgetById(item.i) && isShortcutsWidget(getWidgetById(item.i)!)"
              class="group relative h-full flex flex-col card-depth rounded-xl border border-border bg-card hover:bg-accent transition-colors overflow-hidden"
            >
              <!-- Drag handle -->
              <div v-if="isDragMode" class="widget-drag-handle absolute top-2 left-2 text-foreground/20 cursor-grab active:cursor-grabbing z-10">
                <GripVertical class="size-4" />
              </div>

              <div class="p-6 pb-3 flex flex-row items-center justify-between">
                <div>
                  <span class="font-medium text-foreground/50">{{ getWidgetById(item.i)!.name }}</span>
                  <p v-if="getWidgetById(item.i)!.description" class="text-foreground/30 mt-0.5">{{ getWidgetById(item.i)!.description }}</p>
                </div>
                <div class="flex items-center gap-2">
                  <div v-if="!isDragMode && (canEditWidget || canDeleteWidget)" class="flex items-center gap-1 opacity-0 group-hover:opacity-100">
                    <Button v-if="canEditWidget" variant="ghost" size="icon" class="size-6 text-foreground/20 hover:text-foreground hover:bg-accent" @click.stop="openEditWidgetDialog(getWidgetById(item.i)!)" :title="$t('dashboard.editWidgetTooltip')">
                      <Pencil class="size-3" />
                    </Button>
                    <Button v-if="canDeleteWidget" variant="ghost" size="icon" class="size-6 text-foreground/20 hover:text-destructive hover:bg-destructive/10" @click.stop="openDeleteDialog(getWidgetById(item.i)!)" :title="$t('dashboard.deleteWidgetTooltip')">
                      <Trash2 class="size-3" />
                    </Button>
                  </div>
                </div>
              </div>

              <div class="flex-1 min-h-0 overflow-y-auto px-6 pb-6">
                <div :class="['grid gap-3 pt-1', item.w >= 8 ? 'grid-cols-3' : 'grid-cols-2']">
                  <template v-for="key in (getWidgetById(item.i)!.config?.shortcuts || [])" :key="key">
                    <RouterLink
                      v-if="SHORTCUT_REGISTRY[key as keyof typeof SHORTCUT_REGISTRY]"
                      :to="SHORTCUT_REGISTRY[key as keyof typeof SHORTCUT_REGISTRY].to"
                      class="card-interactive flex flex-col items-center justify-center p-4 rounded-xl border border-border bg-muted"
                    >
                      <div :class="['size-12 rounded-lg bg-linear-to-br flex items-center justify-center mb-2 shadow-lg', SHORTCUT_REGISTRY[key as keyof typeof SHORTCUT_REGISTRY].gradient, 'shadow-' + (key as string) + '-500/20']">
                        <component :is="SHORTCUT_REGISTRY[key as keyof typeof SHORTCUT_REGISTRY].icon" class="size-6 text-white" />
                      </div>
                      <span class="font-medium text-foreground">{{ SHORTCUT_REGISTRY[key as keyof typeof SHORTCUT_REGISTRY].label }}</span>
                    </RouterLink>
                  </template>
                </div>
              </div>
            </div>
          </GridItem>
        </GridLayout>

      </div>
    </ScrollArea>

    <!-- Widget Dialog -->
    <Dialog v-model:open="isWidgetDialogOpen">
      <DialogContent class="sm:max-w-[500px] bg-white border-border text-foreground dark:bg-[#141414]">
        <DialogHeader>
          <DialogTitle>{{ isEditMode ? $t('dashboard.editWidget') : $t('dashboard.createWidget') }}</DialogTitle>
          <DialogDescription class="text-foreground/50">
            {{ $t('dashboard.widgetDialogDesc') }}
          </DialogDescription>
        </DialogHeader>

        <div class="space-y-4 py-4">
          <!-- Name -->
          <div class="space-y-2">
            <Label class="text-foreground/70">{{ $t('dashboard.widgetName') }} *</Label>
            <Input
              v-model="widgetForm.name"
              :placeholder="$t('dashboard.widgetNamePlaceholder')"
              class="bg-card border-border text-foreground placeholder:text-muted-foreground"
            />
          </div>

          <!-- Description -->
          <div class="space-y-2">
            <Label class="text-foreground/70">{{ $t('dashboard.widgetDescription') }}</Label>
            <Textarea
              v-model="widgetForm.description"
              :placeholder="$t('dashboard.widgetDescriptionPlaceholder')"
              class="bg-card border-border text-foreground placeholder:text-muted-foreground"
              :rows="2"
            />
          </div>

          <!-- Data Source (hidden for shortcuts) -->
          <div v-if="widgetForm.display_type !== 'shortcuts'" class="space-y-2">
            <Label class="text-foreground/70">{{ $t('dashboard.dataSource') }} *</Label>
            <Select :model-value="widgetForm.data_source" @update:model-value="(val) => widgetForm.data_source = String(val)">
              <SelectTrigger class="bg-card border-border text-foreground">
                <SelectValue :placeholder="$t('dashboard.selectDataSource')" />
              </SelectTrigger>
              <SelectContent class="bg-white border-border dark:bg-[#1a1a1a]">
                <SelectItem
                  v-for="source in dataSources"
                  :key="source.name"
                  :value="source.name"
                  class="text-foreground/70 focus:bg-accent focus:text-foreground"
                >
                  {{ source.label }}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>

          <!-- Metric (hidden for shortcuts and table) -->
          <div v-if="widgetForm.display_type !== 'shortcuts' && widgetForm.display_type !== 'table'" class="space-y-2">
            <Label class="text-foreground/70">{{ $t('dashboard.metric') }}</Label>
            <Select :model-value="widgetForm.metric" @update:model-value="(val) => widgetForm.metric = String(val)">
              <SelectTrigger class="bg-card border-border text-foreground">
                <SelectValue :placeholder="$t('dashboard.selectMetric')" />
              </SelectTrigger>
              <SelectContent class="bg-white border-border dark:bg-[#1a1a1a]">
                <SelectItem value="count" class="text-foreground/70 focus:bg-accent focus:text-foreground">{{ $t('dashboard.metricCount') }}</SelectItem>
                <SelectItem value="sum" class="text-foreground/70 focus:bg-accent focus:text-foreground">{{ $t('dashboard.metricSum') }}</SelectItem>
                <SelectItem value="avg" class="text-foreground/70 focus:bg-accent focus:text-foreground">{{ $t('dashboard.metricAverage') }}</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <!-- Display Type -->
          <div class="space-y-2">
            <Label class="text-foreground/70">{{ $t('dashboard.displayType') }}</Label>
            <Select :model-value="widgetForm.display_type" @update:model-value="(val) => widgetForm.display_type = String(val)">
              <SelectTrigger class="bg-card border-border text-foreground">
                <SelectValue :placeholder="$t('dashboard.selectDisplayType')" />
              </SelectTrigger>
              <SelectContent class="bg-white border-border dark:bg-[#1a1a1a]">
                <SelectItem value="number" class="text-foreground/70 focus:bg-accent focus:text-foreground">{{ $t('dashboard.displayNumber') }}</SelectItem>
                <SelectItem value="chart" class="text-foreground/70 focus:bg-accent focus:text-foreground">{{ $t('dashboard.displayChart') }}</SelectItem>
                <SelectItem value="table" class="text-foreground/70 focus:bg-accent focus:text-foreground">{{ $t('dashboard.displayTable') }}</SelectItem>
                <SelectItem value="shortcuts" class="text-foreground/70 focus:bg-accent focus:text-foreground">{{ $t('dashboard.displayShortcuts') }}</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <!-- Chart Type (visible when display type is chart) -->
          <div v-if="widgetForm.display_type === 'chart'" class="space-y-2">
            <Label class="text-foreground/70">{{ $t('dashboard.chartType') }}</Label>
            <Select :model-value="widgetForm.chart_type" @update:model-value="(val) => widgetForm.chart_type = String(val)">
              <SelectTrigger class="bg-card border-border text-foreground">
                <SelectValue :placeholder="$t('dashboard.selectChartType')" />
              </SelectTrigger>
              <SelectContent class="bg-white border-border dark:bg-[#1a1a1a]">
                <SelectItem
                  v-for="ct in chartTypeOptions"
                  :key="ct.value"
                  :value="ct.value"
                  class="text-foreground/70 focus:bg-accent focus:text-foreground"
                >
                  {{ ct.label }}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>

          <!-- Group By (visible when display type is chart or table, and data source is selected) -->
          <div v-if="(widgetForm.display_type === 'chart' || widgetForm.display_type === 'table') && widgetForm.data_source" class="space-y-2">
            <Label class="text-foreground/70">{{ $t('dashboard.groupBy') }}</Label>
            <Select :model-value="widgetForm.group_by_field || 'none'" @update:model-value="(val) => widgetForm.group_by_field = val === 'none' ? '' : String(val)">
              <SelectTrigger class="bg-card border-border text-foreground">
                <SelectValue :placeholder="$t('dashboard.noneTimeSeries')" />
              </SelectTrigger>
              <SelectContent class="bg-white border-border dark:bg-[#1a1a1a]">
                <SelectItem
                  value="none"
                  class="text-foreground/70 focus:bg-accent focus:text-foreground"
                >
                  {{ $t('dashboard.noneTimeSeries') }}
                </SelectItem>
                <SelectItem
                  v-for="field in availableFields"
                  :key="field"
                  :value="field"
                  class="text-foreground/70 focus:bg-accent focus:text-foreground"
                >
                  {{ field }}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>

          <!-- Shortcuts selector (only for shortcuts display type) -->
          <div v-if="widgetForm.display_type === 'shortcuts'" class="space-y-2">
            <Label class="text-foreground/70">{{ $t('dashboard.selectShortcuts') }}</Label>
            <div class="space-y-2 max-h-64 overflow-y-auto pr-1">
              <label
                v-for="(shortcut, key) in SHORTCUT_REGISTRY"
                :key="key"
                class="flex items-center gap-3 p-2 rounded-lg hover:bg-accent cursor-pointer"
              >
                <input
                  type="checkbox"
                  :value="key"
                  v-model="selectedShortcuts"
                  class="rounded border-border bg-card text-emerald-500 focus:ring-emerald-500"
                />
                <div class="flex items-center gap-2">
                  <div :class="['size-8 rounded-lg bg-linear-to-br flex items-center justify-center', shortcut.gradient]">
                    <component :is="shortcut.icon" class="size-4 text-white" />
                  </div>
                  <span class="text-foreground/70">{{ shortcut.label }}</span>
                </div>
              </label>
            </div>
          </div>

          <!-- Filters (hidden for shortcuts) -->
          <div v-if="widgetForm.display_type !== 'shortcuts'" class="space-y-2">
            <div class="flex items-center justify-between">
              <Label class="text-foreground/70">{{ $t('dashboard.filters') }} ({{ widgetForm.filters.length }})</Label>
              <Button type="button" variant="outline" size="sm" @click.stop.prevent="addFilter" class="border-border text-foreground hover:bg-accent">
                <Plus class="size-4 mr-1" />
                {{ $t('dashboard.addFilter') }}
              </Button>
            </div>
            <p v-if="!widgetForm.data_source && widgetForm.filters.length === 0" class="text-foreground/40">
              {{ $t('dashboard.selectDataSourceFirst') }}
            </p>
            <div v-for="(filter, index) in widgetForm.filters" :key="index" class="flex items-center gap-2">
              <div class="flex-1">
                <Select :model-value="filter.field" @update:model-value="(val) => filter.field = String(val)">
                  <SelectTrigger class="w-full bg-card border-border text-foreground">
                    <SelectValue :placeholder="$t('dashboard.field')" />
                  </SelectTrigger>
                  <SelectContent class="bg-white border-border dark:bg-[#1a1a1a]">
                    <SelectItem
                      v-for="field in availableFields"
                      :key="field"
                      :value="field"
                      class="text-foreground/70 focus:bg-accent focus:text-foreground"
                    >
                      {{ field }}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div class="w-36">
                <Select :model-value="filter.operator" @update:model-value="(val) => filter.operator = String(val)">
                  <SelectTrigger class="w-full bg-card border-border text-foreground">
                    <SelectValue :placeholder="$t('dashboard.operator')" />
                  </SelectTrigger>
                  <SelectContent class="bg-white border-border dark:bg-[#1a1a1a]">
                    <SelectItem
                      v-for="op in operators"
                      :key="op.value"
                      :value="op.value"
                      class="text-foreground/70 focus:bg-accent focus:text-foreground"
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
              <Button variant="ghost" size="icon" @click="removeFilter(index)" class="text-foreground/50 hover:text-destructive shrink-0">
                <X class="size-4" />
              </Button>
            </div>
          </div>

          <!-- Color (hidden for shortcuts and table) -->
          <div v-if="widgetForm.display_type !== 'shortcuts' && widgetForm.display_type !== 'table'" class="space-y-2">
            <Label class="text-foreground/70">{{ $t('dashboard.color') }}</Label>
            <Select :model-value="widgetForm.color" @update:model-value="(val) => widgetForm.color = String(val)">
              <SelectTrigger class="bg-card border-border text-foreground">
                <SelectValue :placeholder="$t('dashboard.selectColor')" />
              </SelectTrigger>
              <SelectContent class="bg-white border-border dark:bg-[#1a1a1a]">
                <SelectItem
                  v-for="color in colorOptions"
                  :key="color.value"
                  :value="color.value"
                  class="text-foreground/70 focus:bg-accent focus:text-foreground"
                >
                  <span class="flex items-center gap-2">
                    <span :class="['inline-block size-3 rounded-full', color.bg]"></span>
                    {{ color.label }}
                  </span>
                </SelectItem>
              </SelectContent>
            </Select>
          </div>

          <!-- Options -->
          <div class="flex items-center justify-between">
            <div v-if="widgetForm.display_type === 'number' || widgetForm.display_type === 'percentage'" class="flex items-center gap-2">
              <Switch v-model:checked="widgetForm.show_change" />
              <Label class="text-foreground/70">{{ $t('dashboard.showPercentChange') }}</Label>
            </div>
            <div class="flex items-center gap-2">
              <Switch v-model:checked="widgetForm.is_shared" />
              <Label class="text-foreground/70">{{ $t('dashboard.shareWithTeam') }}</Label>
            </div>
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" @click="isWidgetDialogOpen = false" class="border-border text-foreground/70 hover:bg-accent">
            {{ $t('common.cancel') }}
          </Button>
          <Button @click="saveWidget" :disabled="isSavingWidget">
            {{ isSavingWidget ? $t('common.saving') + '...' : (isEditMode ? $t('common.update') : $t('common.create')) }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- Delete Confirmation Dialog -->
    <AlertDialog v-model:open="deleteDialogOpen">
      <AlertDialogContent class="bg-white border-border dark:bg-[#141414]">
        <AlertDialogHeader>
          <AlertDialogTitle class="text-foreground">{{ $t('dashboard.deleteWidgetTitle') }}</AlertDialogTitle>
          <AlertDialogDescription class="text-foreground/60">
            {{ $t('dashboard.deleteWidgetConfirm', { name: widgetToDelete?.name }) }}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel class="bg-transparent border-border text-foreground/70 hover:bg-accent">
            {{ $t('common.cancel') }}
          </AlertDialogCancel>
          <AlertDialogAction @click="confirmDeleteWidget" class="bg-red-600 text-white hover:bg-red-700">
            {{ $t('common.delete') }}
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
  border: 2px dashed rgba(16, 185, 129, 0.4);
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
