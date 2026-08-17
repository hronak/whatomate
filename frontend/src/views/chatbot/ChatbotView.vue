<script setup lang="ts">
import { ref, onMounted } from "vue";
import { useI18n } from "vue-i18n";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { ScrollArea } from "@/components/ui/scroll-area";
import { chatbotService } from "@/services/api";
import { toast } from "vue-sonner";
import { PageHeader, ConfirmDialog, ErrorState } from "@/components/shared";
import { getErrorMessage } from "@/lib/api-utils";
import { Bot, Power } from "@lucide/vue";
import type { ChatbotSettings, Stats } from "./types";
import ChatbotStats from "./ChatbotStats.vue";
import ChatbotConfigList from "./ChatbotConfigList.vue";

const { t } = useI18n();

const settings = ref<ChatbotSettings>({
  enabled: false,
  greeting_message: "",
  fallback_message: "",
  session_timeout_minutes: 30,
  ai_enabled: false,
  ai_provider: "",
});

const stats = ref<Stats>({
  total_sessions: 0,
  active_sessions: 0,
  messages_handled: 0,
  ai_responses: 0,
  agent_transfers: 0,
  keywords_count: 0,
  flows_count: 0,
  ai_contexts_count: 0,
});

const isLoading = ref(true);
const isToggling = ref(false);
const error = ref(false);
const showToggleConfirm = ref(false);

onMounted(async () => {
  try {
    const response = await chatbotService.getSettings();
    // API response is wrapped in { status: "success", data: { settings: {...}, stats: {...} } }
    const data = response.data || response.data;
    settings.value = data.settings || settings.value;
    stats.value = data.stats || stats.value;
  } catch (err) {
    console.error("Failed to load chatbot settings:", err);
    error.value = true;
  } finally {
    isLoading.value = false;
  }
});

async function toggleChatbot() {
  isToggling.value = true;
  try {
    const newState = !settings.value.enabled;
    await chatbotService.updateSettings({ enabled: newState });
    settings.value.enabled = newState;
    toast.success(
      newState
        ? t("common.enabledSuccess", { resource: t("resources.Chatbot") })
        : t("common.disabledSuccess", { resource: t("resources.Chatbot") }),
    );
  } catch (err: any) {
    toast.error(
      getErrorMessage(
        err,
        t("common.failedToggle", { resource: t("resources.chatbot") }),
      ),
    );
  } finally {
    isToggling.value = false;
    showToggleConfirm.value = false;
  }
}

async function retryFetch() {
  isLoading.value = true;
  error.value = false;
  try {
    const response = await chatbotService.getSettings();
    const data = response.data || response.data;
    settings.value = data.settings || settings.value;
    stats.value = data.stats || stats.value;
  } catch (err) {
    console.error("Failed to load chatbot settings:", err);
    error.value = true;
  } finally {
    isLoading.value = false;
  }
}
</script>

<template>
  <div class="flex flex-col h-full bg-background">
    <PageHeader
      :title="$t('chatbot.title')"
      :description="$t('chatbot.subtitle')"
      :icon="Bot"
    >
      <template #actions>
        <div class="flex items-center gap-3">
          <Badge
            :class="
              settings.enabled
                ? 'bg-success/20 text-success'
                : 'bg-muted text-foreground/50'
            "
          >
            {{
              settings.enabled ? $t("chatbot.active") : $t("chatbot.inactive")
            }}
          </Badge>
          <Button
            variant="outline"
            size="sm"
            @click="showToggleConfirm = true"
            :disabled="isToggling"
            :class="
              settings.enabled
                ? 'border-red-500/50 text-red-400 hover:bg-red-500/10'
                : 'border-primary/50 text-primary hover:bg-primary/10'
            "
          >
            <Power class="size-4 mr-2" />
            {{
              settings.enabled ? $t("chatbot.disable") : $t("chatbot.enable")
            }}
          </Button>
        </div>
      </template>
    </PageHeader>

    <!-- Confirm Toggle Dialog -->
    <ConfirmDialog
      v-model:open="showToggleConfirm"
      :title="
        settings.enabled
          ? $t('chatbot.confirmDisableTitle')
          : $t('chatbot.confirmEnableTitle')
      "
      :description="
        settings.enabled
          ? $t('chatbot.confirmDisableDescription')
          : $t('chatbot.confirmEnableDescription')
      "
      :confirm-label="
        settings.enabled ? $t('chatbot.disable') : $t('chatbot.enable')
      "
      :variant="settings.enabled ? 'destructive' : 'default'"
      :is-submitting="isToggling"
      @confirm="toggleChatbot"
    />

    <!-- Error State -->
    <ErrorState
      v-if="error && !isLoading"
      :title="$t('chatbot.fetchErrorTitle')"
      :description="$t('chatbot.fetchErrorDescription')"
      :retry-label="$t('common.retry')"
      class="flex-1"
      @retry="retryFetch"
    />

    <!-- Content -->
    <ScrollArea v-else class="flex-1">
      <div class="p-6 gap-y-6">
        <!-- Stats -->
        <ChatbotStats :is-loading="isLoading" :stats="stats" />

        <!-- Config List -->
        <ChatbotConfigList :stats="stats" :settings="settings" />
      </div>
    </ScrollArea>
  </div>
</template>
