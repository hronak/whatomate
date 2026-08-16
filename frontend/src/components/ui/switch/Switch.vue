<script setup lang="ts">
import type { HTMLAttributes } from "vue";
import { SwitchRoot, SwitchThumb } from "reka-ui";
import { cn } from "@/lib/utils";

const props = defineProps<{
  checked?: boolean;
  defaultChecked?: boolean;
  disabled?: boolean;
  required?: boolean;
  name?: string;
  value?: string;
  class?: HTMLAttributes["class"];
}>();

const emits = defineEmits<{
  "update:checked": [value: boolean];
}>();

function handleChange(value: boolean) {
  emits("update:checked", value);
}
</script>

<template>
  <SwitchRoot
    :model-value="props.checked"
    :default-value="props.defaultChecked"
    :disabled="props.disabled"
    :required="props.required"
    :name="props.name"
    :value="props.value"
    @update:model-value="handleChange"
    :class="
      cn(
        'peer data-[state=checked]:bg-primary data-[state=unchecked]:bg-input focus-visible:border-ring focus-visible:ring-ring/50 dark:data-[state=unchecked]:bg-input/80 inline-flex h-[1.15rem] w-8 shrink-0 cursor-pointer items-center rounded-full border border-transparent shadow-xs transition-all outline-none focus-visible:ring-3 disabled:cursor-not-allowed disabled:opacity-50',
        props.class,
      )
    "
  >
    <SwitchThumb
      :class="
        cn(
          'bg-background dark:data-[state=unchecked]:bg-foreground dark:data-[state=checked]:bg-primary-foreground pointer-events-none block size-4 rounded-full ring-0 transition-transform data-[state=checked]:translate-x-[calc(100%-2px)] data-[state=unchecked]:translate-x-0',
        )
      "
    >
      <slot name="thumb" />
    </SwitchThumb>
  </SwitchRoot>
</template>
