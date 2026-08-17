<script setup lang="ts">
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Key,
  Workflow,
  Sparkles,
  Settings,
  Clock,
} from "@lucide/vue";
import type { Stats, ChatbotSettings } from "./types";

const props = defineProps<{
  stats: Stats;
  settings: ChatbotSettings;
}>();
</script>

<template>
  <div class="flex flex-col gap-y-6">
    <!-- Quick Actions -->
    <div class="grid gap-4 md:grid-cols-3">
      <RouterLink
        to="/chatbot/keywords"
        class="card-interactive rounded-xl border border-border bg-card h-full"
      >
        <div class="p-6">
          <div class="flex items-center gap-3">
            <div
              class="size-10 rounded-lg bg-linear-to-br from-blue-500 to-cyan-600 flex items-center justify-center shadow-lg shadow-blue-500/20"
            >
              <Key class="size-5 text-white" />
            </div>
            <div>
              <h3 class="text-lg font-semibold text-foreground">
                {{ $t("chatbot.keywordRules") }}
              </h3>
              <p class="text-foreground/40">
                {{
                  $t("chatbot.rulesConfigured", {
                    count: stats.keywords_count,
                  })
                }}
              </p>
            </div>
          </div>
        </div>
        <div class="px-6 pb-6">
          <p class="text-foreground/50">
            {{ $t("chatbot.keywordRulesDesc") }}
          </p>
        </div>
      </RouterLink>

      <RouterLink
        to="/chatbot/flows"
        class="card-interactive rounded-xl border border-border bg-card h-full"
      >
        <div class="p-6">
          <div class="flex items-center gap-3">
            <div
              class="size-10 rounded-lg bg-linear-to-br from-purple-500 to-pink-600 flex items-center justify-center shadow-lg shadow-purple-500/20"
            >
              <Workflow class="size-5 text-white" />
            </div>
            <div>
              <h3 class="text-lg font-semibold text-foreground">
                {{ $t("chatbot.conversationFlows") }}
              </h3>
              <p class="text-foreground/40">
                {{
                  $t("chatbot.flowsCreated", { count: stats.flows_count })
                }}
              </p>
            </div>
          </div>
        </div>
        <div class="px-6 pb-6">
          <p class="text-foreground/50">
            {{ $t("chatbot.flowsDesc") }}
          </p>
        </div>
      </RouterLink>

      <RouterLink
        to="/chatbot/ai"
        class="card-interactive rounded-xl border border-border bg-card h-full"
      >
        <div class="p-6">
          <div class="flex items-center gap-3">
            <div
              class="size-10 rounded-lg bg-linear-to-br from-orange-500 to-amber-600 flex items-center justify-center shadow-lg shadow-orange-500/20"
            >
              <Sparkles class="size-5 text-white" />
            </div>
            <div>
              <h3 class="text-lg font-semibold text-foreground">
                {{ $t("chatbot.aiContexts") }}
              </h3>
              <p class="text-foreground/40">
                {{
                  $t("chatbot.contextsActive", {
                    count: stats.ai_contexts_count,
                  })
                }}
              </p>
            </div>
          </div>
        </div>
        <div class="px-6 pb-6">
          <p class="text-foreground/50">
            {{ $t("chatbot.aiContextsDesc") }}
          </p>
        </div>
      </RouterLink>
    </div>

    <!-- Current Settings -->
    <div class="rounded-xl border border-border bg-card">
      <div class="p-6">
        <div class="flex items-center justify-between">
          <div>
            <h3 class="text-lg font-semibold text-foreground">
              {{ $t("chatbot.currentConfiguration") }}
            </h3>
            <p class="text-foreground/40">
              {{ $t("chatbot.configOverview") }}
            </p>
          </div>
          <RouterLink to="/settings/chatbot">
            <Button variant="outline" size="sm">
              <Settings class="size-4 mr-2" />
              {{ $t("chatbot.editSettings") }}
            </Button>
          </RouterLink>
        </div>
      </div>
      <div class="px-6 pb-6">
        <div class="grid gap-4 md:grid-cols-2">
          <div class="gap-y-2">
            <h4 class="font-medium text-foreground/70">
              {{ $t("chatbot.greetingMessage") }}
            </h4>
            <p class="text-foreground/50 bg-muted p-3 rounded-lg">
              {{ settings.greeting_message || $t("chatbot.notConfigured") }}
            </p>
          </div>
          <div class="gap-y-2">
            <h4 class="font-medium text-foreground/70">
              {{ $t("chatbot.fallbackMessage") }}
            </h4>
            <p class="text-foreground/50 bg-muted p-3 rounded-lg">
              {{ settings.fallback_message || $t("chatbot.notConfigured") }}
            </p>
          </div>
          <div class="gap-y-2">
            <h4 class="font-medium text-foreground/70">
              {{ $t("chatbot.sessionTimeout") }}
            </h4>
            <div class="flex items-center gap-2 text-foreground/50">
              <Clock class="size-4" />
              {{
                $t("chatbot.minutes", {
                  count: settings.session_timeout_minutes,
                })
              }}
            </div>
          </div>
          <div class="gap-y-2">
            <h4 class="font-medium text-foreground/70">
              {{ $t("chatbot.aiProvider") }}
            </h4>
            <div class="flex items-center gap-2">
              <Badge
                v-if="settings.ai_enabled"
                class="bg-success/20 text-success"
              >
                {{ settings.ai_provider || $t("chatbot.notConfigured") }}
              </Badge>
              <Badge v-else class="bg-muted text-foreground/50">{{
                $t("chatbot.disabled")
              }}</Badge>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
