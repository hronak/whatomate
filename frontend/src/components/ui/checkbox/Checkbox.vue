<script setup lang="ts">
import type { HTMLAttributes } from "vue";
import { Check } from "@lucide/vue";
import { CheckboxIndicator, CheckboxRoot } from "reka-ui";
import { cn } from "@/lib/utils";

type CheckedState = boolean | "indeterminate";

const props = defineProps<{
  checked?: CheckedState;
  defaultChecked?: CheckedState;
  disabled?: boolean;
  required?: boolean;
  name?: string;
  value?: string;
  id?: string;
  class?: HTMLAttributes["class"];
}>();

const emits = defineEmits<{
  "update:checked": [value: CheckedState];
}>();

function handleChange(value: CheckedState) {
  emits("update:checked", value);
}
</script>

<template>
  <CheckboxRoot
    :model-value="props.checked"
    :default-value="props.defaultChecked"
    :disabled="props.disabled"
    :required="props.required"
    :name="props.name"
    :value="props.value"
    :id="props.id"
    @update:model-value="handleChange"
    :class="
      cn(
        'grid place-content-center peer border-input data-[state=checked]:bg-primary data-[state=checked]:text-primary-foreground data-[state=checked]:border-primary focus-visible:border-ring focus-visible:ring-ring/50 aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 aria-invalid:border-destructive size-4 shrink-0 rounded-[4px] border shadow-xs transition-shadow outline-none focus-visible:ring-3 disabled:cursor-not-allowed disabled:opacity-50',
        props.class,
      )
    "
  >
    <CheckboxIndicator
      class="grid place-content-center text-current transition-none"
    >
      <slot>
        <Check class="size-3.5" />
      </slot>
    </CheckboxIndicator>
  </CheckboxRoot>
</template>
