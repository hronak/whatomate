<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Sparkles,
  TrendingUp,
  Users,
  MessageSquare,
} from "@lucide/vue";
import type { Stats } from "./types";

const { t } = useI18n();

const props = defineProps<{
  isLoading: boolean;
  stats: Stats;
}>();

const statCards = computed(() => [
  {
    title: t("chatbot.totalSessions"),
    key: "total_sessions",
    icon: Users,
    color: "text-blue-500",
  },
  {
    title: t("chatbot.activeSessions"),
    key: "active_sessions",
    icon: MessageSquare,
    color: "text-success",
  },
  {
    title: t("chatbot.messagesHandled"),
    key: "messages_handled",
    icon: TrendingUp,
    color: "text-purple-500",
  },
  {
    title: t("chatbot.aiResponses"),
    key: "ai_responses",
    icon: Sparkles,
    color: "text-orange-500",
  },
]);
</script>

<template>
  <div class="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
    <!-- Skeleton Loading State -->
    <template v-if="isLoading">
      <div
        v-for="i in 4"
        :key="i"
        class="rounded-xl border border-border bg-card p-6"
      >
        <div class="flex flex-row items-center justify-between gap-y-0 pb-2">
          <Skeleton class="h-4 w-24 bg-muted" />
          <Skeleton class="size-10 rounded-lg bg-muted" />
        </div>
        <div class="pt-2">
          <Skeleton class="h-8 w-16 bg-muted" />
        </div>
      </div>
    </template>
    <!-- Actual Stats -->
    <template v-else>
      <div
        v-for="card in statCards"
        :key="card.key"
        data-testid="stat-card"
        class="card-depth rounded-xl border border-border bg-card p-6"
      >
        <div class="flex flex-row items-center justify-between gap-y-0 pb-2">
          <span class="font-medium text-foreground/50">{{ card.title }}</span>
          <div
            :class="[
              'size-10 rounded-lg flex items-center justify-center',
              card.key === 'total_sessions' ? 'bg-blue-500/20' : '',
              card.key === 'active_sessions' ? 'bg-success/10' : '',
              card.key === 'messages_handled' ? 'bg-purple-500/20' : '',
              card.key === 'ai_responses' ? 'bg-orange-500/20' : '',
            ]"
          >
            <component
              :is="card.icon"
              :class="[
                'size-5',
                card.key === 'total_sessions' ? 'text-blue-400' : '',
                card.key === 'active_sessions' ? 'text-success' : '',
                card.key === 'messages_handled' ? 'text-purple-400' : '',
                card.key === 'ai_responses' ? 'text-orange-400' : '',
              ]"
            />
          </div>
        </div>
        <div class="pt-2">
          <div class="text-3xl font-bold text-foreground">
            {{ stats[card.key as keyof Stats].toLocaleString() }}
          </div>
        </div>
      </div>
    </template>
  </div>
</template>
