<script setup lang="ts">
import { computed } from "vue";
import type { SimulationState, FlowStep } from "@/types/flow-preview";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import ExecutionTimeline from "./ExecutionTimeline.vue";
import {
  Play,
  Pause,
  RotateCcw,
  SkipForward,
  Undo2,
  ChevronDown,
  ChevronRight,
  Braces,
  ListTree,
  CircleDot,
} from "@lucide/vue";
import { ref } from "vue";

const props = defineProps<{
  state: SimulationState;
  steps: FlowStep[];
  canUndo: boolean;
}>();

const emit = defineEmits<{
  start: [];
  pause: [];
  resume: [];
  reset: [];
  stepForward: [];
  undo: [];
  goToStep: [stepName: string];
}>();

const variablesExpanded = ref(true);
const timelineExpanded = ref(true);
const stepsExpanded = ref(false);

const statusLabel = computed(() => {
  switch (props.state.status) {
    case "idle":
      return "Ready";
    case "running":
      return "Running";
    case "paused":
      return "Paused";
    case "waiting_input":
      return "Waiting for input";
    case "completed":
      return "Completed";
    case "error":
      return "Error";
    default:
      return props.state.status;
  }
});

const statusColor = computed(() => {
  switch (props.state.status) {
    case "idle":
      return "bg-muted-foreground";
    case "running":
      return "bg-success";
    case "paused":
      return "bg-warning";
    case "waiting_input":
      return "bg-info";
    case "completed":
      return "bg-success";
    case "error":
      return "bg-destructive";
    default:
      return "bg-muted-foreground";
  }
});

const variableEntries = computed(() => {
  return Object.entries(props.state.variables);
});

function handlePlayPause() {
  // 'completed' restarts the flow (startSimulation wipes state).
  if (props.state.status === "idle" || props.state.status === "completed") {
    emit("start");
  } else if (props.state.status === "paused") {
    emit("resume");
  } else if (
    props.state.status === "running" ||
    props.state.status === "waiting_input"
  ) {
    emit("pause");
  }
}
</script>

<template>
  <div
    class="h-full flex flex-col bg-muted/30 border-l border-border"
  >
    <!-- Header -->
    <div
      class="px-3 py-2 border-b border-border bg-card"
    >
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-2">
          <div class="size-2 rounded-full animate-pulse" :class="statusColor" />
          <span class="font-medium text-foreground">
            {{ statusLabel }}
          </span>
        </div>

        <!-- Control Buttons -->
        <div class="flex items-center gap-1">
          <Button
            variant="ghost"
            size="icon"
            class="size-7"
            :disabled="state.status === 'error'"
            :title="
              state.status === 'completed'
                ? 'Restart'
                : state.status === 'running' || state.status === 'waiting_input'
                  ? 'Pause'
                  : 'Start'
            "
            @click="handlePlayPause"
          >
            <Pause
              v-if="
                state.status === 'running' || state.status === 'waiting_input'
              "
              class="size-4"
            />
            <Play v-else class="size-4" />
          </Button>

          <Button
            variant="ghost"
            size="icon"
            class="size-7"
            :disabled="
              state.status === 'idle' ||
              state.status === 'completed' ||
              state.status === 'error' ||
              state.status === 'waiting_input'
            "
            @click="emit('stepForward')"
          >
            <SkipForward class="size-4" />
          </Button>

          <Button
            variant="ghost"
            size="icon"
            class="size-7"
            title="Step back"
            :disabled="!canUndo"
            @click="emit('undo')"
          >
            <Undo2 class="size-4" />
          </Button>

          <Button
            variant="ghost"
            size="icon"
            class="size-7"
            title="Restart"
            @click="emit('reset')"
          >
            <RotateCcw class="size-4" />
          </Button>
        </div>
      </div>

      <!-- Current Step -->
      <div
        v-if="state.currentStepName"
        class="mt-1 text-muted-foreground"
      >
        Step {{ (state.currentStepIndex ?? 0) + 1 }}:
        <span class="font-mono">{{ state.currentStepName }}</span>
      </div>
    </div>

    <ScrollArea class="flex-1">
      <div class="p-2 space-y-2">
        <!-- Variables Section -->
        <Collapsible v-model:open="variablesExpanded">
          <CollapsibleTrigger
            class="flex items-center gap-2 w-full px-2 py-1.5 hover:bg-accent rounded font-medium text-foreground"
          >
            <ChevronDown v-if="variablesExpanded" class="size-4" />
            <ChevronRight v-else class="size-4" />
            <Braces class="size-4" />
            Variables
            <span class="ml-auto text-muted-foreground">{{
              variableEntries.length
            }}</span>
          </CollapsibleTrigger>
          <CollapsibleContent>
            <div
              class="mt-1 px-2 py-1 bg-card rounded border border-border"
            >
              <div
                v-if="variableEntries.length === 0"
                class="text-muted-foreground py-2 text-center"
              >
                No variables set
              </div>
              <div v-else class="space-y-1">
                <div
                  v-for="[key, value] in variableEntries"
                  :key="key"
                  class="flex items-start gap-2 py-1"
                >
                  <span
                    class="font-mono text-primary shrink-0"
                    >{{ key }}:</span
                  >
                  <span class="text-foreground break-all">
                    {{
                      typeof value === "object" ? JSON.stringify(value) : value
                    }}
                  </span>
                </div>
              </div>
            </div>
          </CollapsibleContent>
        </Collapsible>

        <!-- Steps Section -->
        <Collapsible v-model:open="stepsExpanded">
          <CollapsibleTrigger
            class="flex items-center gap-2 w-full px-2 py-1.5 hover:bg-accent rounded font-medium text-foreground"
          >
            <ChevronDown v-if="stepsExpanded" class="size-4" />
            <ChevronRight v-else class="size-4" />
            <ListTree class="size-4" />
            Steps
            <span class="ml-auto text-muted-foreground">{{ steps.length }}</span>
          </CollapsibleTrigger>
          <CollapsibleContent>
            <div
              class="mt-1 px-2 py-1 bg-card rounded border border-border max-h-40 overflow-y-auto"
            >
              <div
                v-for="(step, idx) in steps"
                :key="step.step_name"
                class="flex items-center gap-2 py-1.5 px-1 rounded cursor-pointer transition-colors"
                :class="{
                  'bg-accent':
                    state.currentStepName === step.step_name,
                  'hover:bg-accent':
                    state.currentStepName !== step.step_name,
                }"
                @click="emit('goToStep', step.step_name)"
              >
                <CircleDot
                  class="size-3"
                  :class="{
                    'text-success': state.currentStepName === step.step_name,
                    'text-muted-foreground/40':
                      state.currentStepName !== step.step_name,
                  }"
                />
                <span class="text-muted-foreground">{{ idx + 1 }}.</span>
                <span class="font-mono text-foreground">{{
                  step.step_name
                }}</span>
              </div>
            </div>
          </CollapsibleContent>
        </Collapsible>

        <!-- Timeline Section -->
        <Collapsible v-model:open="timelineExpanded">
          <CollapsibleTrigger
            class="flex items-center gap-2 w-full px-2 py-1.5 hover:bg-accent rounded font-medium text-foreground"
          >
            <ChevronDown v-if="timelineExpanded" class="size-4" />
            <ChevronRight v-else class="size-4" />
            Timeline
            <span class="ml-auto text-muted-foreground">{{
              state.executionLog.length
            }}</span>
          </CollapsibleTrigger>
          <CollapsibleContent>
            <div
              class="mt-1 bg-card rounded border border-border max-h-60 overflow-y-auto"
            >
              <ExecutionTimeline
                :entries="state.executionLog"
                :current-step-name="state.currentStepName"
              />
            </div>
          </CollapsibleContent>
        </Collapsible>
      </div>
    </ScrollArea>
  </div>
</template>
