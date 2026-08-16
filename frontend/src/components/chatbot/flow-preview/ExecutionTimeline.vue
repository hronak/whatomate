<script setup lang="ts">
import { computed } from "vue";
import type { ExecutionLogEntry } from "@/types/flow-preview";
import {
  Play,
  LogIn,
  LogOut,
  Tag,
  GitBranch,
  Globe,
  CheckCircle,
  XCircle,
  Flag,
  AlertCircle,
} from "@lucide/vue";

const props = defineProps<{
  entries: ExecutionLogEntry[];
  currentStepName?: string | null;
}>();

const formattedEntries = computed(() => {
  return props.entries
    .map((entry) => {
      const time = entry.timestamp.toLocaleTimeString("en-US", {
        hour: "numeric",
        minute: "2-digit",
        second: "2-digit",
        hour12: false,
      });

      let icon = Play;
      let color = "text-muted-foreground";
      let label = "";
      let details = "";

      switch (entry.type) {
        case "flow_start":
          icon = Play;
          color = "text-success";
          label = "Flow started";
          details = `${entry.details.stepsCount} steps`;
          break;
        case "step_enter":
          icon = LogIn;
          color = "text-info";
          label = `Enter: ${entry.stepName}`;
          details =
            (entry.details.type as string) ||
            `${entry.details.messageType || ""}/${entry.details.inputType || ""}`.replace(
              /^\/$/,
              "",
            );
          break;
        case "step_exit":
          icon = LogOut;
          color = "text-muted-foreground/60";
          label = `Exit: ${entry.stepName}`;
          details =
            (entry.details.outcome as string) ||
            (entry.details.next as string) ||
            "";
          break;
        case "variable_set":
          icon = Tag;
          color = "text-primary";
          label = `Set: ${entry.details.key}`;
          details = String(entry.details.value).substring(0, 30);
          break;
        case "condition_eval":
          icon = GitBranch;
          color = entry.details.result ? "text-success" : "text-warning";
          label = `Condition: ${entry.details.type}`;
          details = entry.details.result ? "true" : "false";
          break;
        case "api_call":
          icon = Globe;
          color = "text-cyan-500";
          label = "API call";
          details = `${entry.details.method} ${entry.details.url}`;
          break;
        case "validation_pass":
          icon = CheckCircle;
          color = "text-success";
          label = "Validation passed";
          break;
        case "validation_fail":
          icon = XCircle;
          color = "text-destructive";
          label = "Validation failed";
          details = `Retry ${entry.details.retryCount}/${entry.details.maxRetries}`;
          break;
        case "branch":
          icon = GitBranch;
          color = "text-amber-500";
          label = "Branch";
          details = `→ ${entry.details.nextStep || "end"}`;
          break;
        case "flow_complete":
          icon = Flag;
          color = "text-success";
          label = "Flow completed";
          details = entry.details.reason;
          break;
        case "flow_error":
          icon = AlertCircle;
          color = "text-destructive";
          label = "Error";
          details = entry.details.error;
          break;
      }

      return {
        ...entry,
        time,
        icon,
        color,
        label,
        details,
        isCurrent: entry.stepName === props.currentStepName,
      };
    })
    .reverse(); // Most recent first
});
</script>

<template>
  <div class="space-y-1">
    <div
      v-for="entry in formattedEntries"
      :key="entry.id"
      class="flex items-start gap-2 py-1 px-2 rounded transition-colors"
      :class="{ 'bg-accent': entry.isCurrent }"
    >
      <component
        :is="entry.icon"
        class="size-3.5 shrink-0 mt-0.5"
        :class="entry.color"
      />
      <div class="flex-1 min-w-0">
        <div class="flex items-center gap-2">
          <span class="font-medium text-foreground">{{
            entry.label
          }}</span>
          <span class="text-muted-foreground/60">{{ entry.time }}</span>
        </div>
        <p
          v-if="entry.details"
          class="text-muted-foreground truncate"
        >
          {{ entry.details }}
        </p>
      </div>
    </div>

    <div
      v-if="formattedEntries.length === 0"
      class="text-center text-muted-foreground py-4"
    >
      No execution logs yet
    </div>
  </div>
</template>
