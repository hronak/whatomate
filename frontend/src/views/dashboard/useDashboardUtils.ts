import { computed, type Ref } from "vue";
import { useI18n } from "vue-i18n";
import {
  MessageSquare,
  Users,
  Bot,
  Send,
  BarChart3,
  Megaphone,
  FileText,
  Contact,
  Workflow,
  UserX,
  LineChart,
  Settings,
  MessageSquareText,
  Tags,
  Shield,
  Key,
  Webhook,
  Zap,
  ShieldCheck,
} from "@lucide/vue";
import type { DashboardWidget, WidgetData } from "@/services/api";

export function useDashboardUtils(selectedRange?: Ref<string>) {
  const { t } = useI18n();

  const SHORTCUT_REGISTRY = computed(() => ({
    chat: { label: t("dashboard.startChat"), to: "/chat", icon: MessageSquare },
    campaigns: { label: t("nav.campaigns"), to: "/campaigns", icon: Megaphone },
    templates: { label: t("nav.templates"), to: "/templates", icon: FileText },
    chatbot: { label: t("nav.chatbot"), to: "/chatbot", icon: Bot },
    contacts: { label: t("nav.contacts"), to: "/settings/contacts", icon: Contact },
    flows: { label: t("nav.flows"), to: "/flows", icon: Workflow },
    transfers: { label: t("nav.transfers"), to: "/chatbot/transfers", icon: UserX },
    agentAnalytics: { label: t("nav.agentAnalytics"), to: "/analytics/agents", icon: BarChart3 },
    metaInsights: { label: t("nav.metaInsights"), to: "/analytics/meta-insights", icon: LineChart },
    settings: { label: t("nav.settings"), to: "/settings", icon: Settings },
    accounts: { label: t("nav.accounts"), to: "/settings/accounts", icon: Users },
    cannedResponses: { label: t("nav.cannedResponses"), to: "/settings/canned-responses", icon: MessageSquareText },
    tags: { label: t("nav.tags"), to: "/settings/tags", icon: Tags },
    teams: { label: t("nav.teams"), to: "/settings/teams", icon: Users },
    users: { label: t("nav.users"), to: "/settings/users", icon: Users },
    roles: { label: t("nav.roles"), to: "/settings/roles", icon: Shield },
    apiKeys: { label: t("nav.apiKeys"), to: "/settings/api-keys", icon: Key },
    webhooks: { label: t("nav.webhooks"), to: "/settings/webhooks", icon: Webhook },
    customActions: { label: t("nav.customActions"), to: "/settings/custom-actions", icon: Zap },
    sso: { label: t("nav.sso"), to: "/settings/sso", icon: ShieldCheck },
  }));

  const colorOptions = computed(() => [
    { value: "blue", label: t("dashboard.colorBlue"), bg: "bg-blue-500/20", text: "text-blue-400" },
    { value: "green", label: t("dashboard.colorGreen"), bg: "bg-emerald-500/20", text: "text-emerald-400" },
    { value: "purple", label: t("dashboard.colorPurple"), bg: "bg-purple-500/20", text: "text-purple-400" },
    { value: "orange", label: t("dashboard.colorOrange"), bg: "bg-orange-500/20", text: "text-orange-400" },
    { value: "red", label: t("dashboard.colorRed"), bg: "bg-red-500/20", text: "text-red-400" },
    { value: "cyan", label: t("dashboard.colorCyan"), bg: "bg-cyan-500/20", text: "text-cyan-400" },
  ]);

  const chartTypeOptions = computed(() => [
    { value: "line", label: t("dashboard.chartLine") },
    { value: "bar", label: t("dashboard.chartBar") },
    { value: "pie", label: t("dashboard.chartPie") },
  ]);

  const chartColors = [
    "var(--color-chart-1)",
    "var(--color-chart-2)",
    "var(--color-chart-3)",
    "var(--color-chart-4)",
    "var(--color-chart-5)",
    "var(--color-chart-1)",
    "var(--color-chart-2)",
    "var(--color-chart-3)",
  ];

  const getChartComponentData = (widget: DashboardWidget, widgetData: Record<string, WidgetData>) => {
    const data = widgetData[widget.id];
    if (!data) return { labels: [], datasets: [] };

    const chartData = data.chart_data || [];
    const dataPoints = data.data_points || [];
    const groupedSeries = data.grouped_series;

    if (widget.chart_type === "line" && groupedSeries && groupedSeries.datasets.length > 0) {
      return {
        labels: groupedSeries.labels,
        datasets: groupedSeries.datasets.map((ds, i) => ({
          label: ds.label,
          data: ds.data,
          borderColor: chartColors[i % chartColors.length],
          backgroundColor: chartColors[i % chartColors.length].replace("0.8)", "0.1)"),
          fill: false,
          tension: 0.3,
        })),
      };
    }

    if (widget.chart_type === "pie") {
      const source = dataPoints.length > 0 ? dataPoints : chartData;
      return {
        labels: source.map((d: { label: string }) => d.label),
        datasets: [
          {
            data: source.map((d: { value: number }) => d.value),
            backgroundColor: chartColors.slice(0, source.length),
            borderWidth: 0,
          },
        ],
      };
    }

    if (widget.chart_type === "bar" && dataPoints.length > 0) {
      return {
        labels: dataPoints.map((d: { label: string }) => d.label),
        datasets: [
          {
            label: widget.name,
            data: dataPoints.map((d: { value: number }) => d.value),
            backgroundColor: dataPoints.map((_, i) => chartColors[i % chartColors.length]),
            borderWidth: 0,
          },
        ],
      };
    }

    const colorMap: Record<string, string> = {
      blue: "rgb(59, 130, 246)",
      green: "rgb(16, 185, 129)",
      purple: "rgb(139, 92, 246)",
      orange: "rgb(245, 158, 11)",
      red: "rgb(239, 68, 68)",
      cyan: "rgb(6, 182, 212)",
    };
    const borderColor = colorMap[widget.color] || colorMap.blue;

    return {
      labels: chartData.map((d: { label: string }) => d.label),
      datasets: [
        {
          label: widget.name,
          data: chartData.map((d: { value: number }) => d.value),
          borderColor,
          backgroundColor:
            widget.chart_type === "bar"
              ? borderColor.replace("rgb", "rgba").replace(")", ", 0.8)")
              : borderColor.replace("rgb", "rgba").replace(")", ", 0.1)"),
          fill: widget.chart_type === "line",
          tension: 0.3,
        },
      ],
    };
  };

  const lineBarChartOptions = {
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: { display: true, position: "top" as const },
    },
    scales: {
      y: { beginAtZero: true },
    },
  };

  const pieChartOptions = {
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: { position: "bottom" as const },
    },
  };

  const comparisonPeriodLabel = computed(() => {
    if (!selectedRange) return t("dashboard.fromPreviousPeriod");
    switch (selectedRange.value) {
      case "today": return t("dashboard.fromYesterday");
      case "7days": return t("dashboard.fromPrevious7Days");
      case "30days": return t("dashboard.fromPrevious30Days");
      case "this_month": return t("dashboard.fromLastMonth");
      case "custom": return t("dashboard.fromPreviousPeriod");
      default: return t("dashboard.fromPreviousPeriod");
    }
  });

  const formatNumber = (num: number): string => {
    if (num >= 1000000) return (num / 1000000).toFixed(1) + "M";
    if (num >= 1000) return (num / 1000).toFixed(1) + "K";
    return Math.round(num).toString();
  };

  const formatTime = (dateStr: string): string => {
    const date = new Date(dateStr);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);
    const diffDays = Math.floor(diffMs / 86400000);

    if (diffMins < 1) return t("dashboard.justNow");
    if (diffMins < 60) return t("dashboard.minutesAgo", { count: diffMins });
    if (diffHours < 24) return t("dashboard.hoursAgo", { count: diffHours });
    return t("dashboard.daysAgo", { count: diffDays });
  };

  const getWidgetColor = (color: string) => {
    const gradientMap: Record<string, string> = {
      blue: "bg-linear-to-r from-blue-500/60 to-blue-500/0",
      green: "bg-linear-to-r from-emerald-500/60 to-emerald-500/0",
      purple: "bg-linear-to-r from-violet-500/60 to-violet-500/0",
      orange: "bg-linear-to-r from-amber-500/60 to-amber-500/0",
      red: "bg-linear-to-r from-rose-500/60 to-rose-500/0",
      cyan: "bg-linear-to-r from-cyan-500/60 to-cyan-500/0",
    };
    const colorConfig = colorOptions.value.find((c) => c.value === color) || colorOptions.value[0];
    return {
      ...colorConfig,
      gradient: gradientMap[colorConfig.value] || gradientMap.blue,
    };
  };

  const getWidgetIcon = (dataSource: string) => {
    switch (dataSource) {
      case "messages": return MessageSquare;
      case "contacts": return Users;
      case "sessions": return Bot;
      case "campaigns": return Send;
      case "transfers": return Users;
      default: return BarChart3;
    }
  };

  return {
    SHORTCUT_REGISTRY,
    colorOptions,
    chartTypeOptions,
    getChartComponentData,
    lineBarChartOptions,
    pieChartOptions,
    comparisonPeriodLabel,
    formatNumber,
    formatTime,
    getWidgetColor,
    getWidgetIcon,
  };
}
