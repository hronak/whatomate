<script setup lang="ts">
import {
  DropdownMenuPortal,
  DropdownMenuContent as DropdownMenuContentPrimitive,
} from "reka-ui";
import { cn } from "@/lib/utils";

const props = defineProps<{
  class?: string;
  side?: "top" | "right" | "bottom" | "left";
  sideOffset?: number;
  align?: "start" | "center" | "end";
}>();
</script>

<template>
  <DropdownMenuPortal>
    <DropdownMenuContentPrimitive
      :side="side"
      :side-offset="sideOffset ?? 4"
      :align="align ?? 'center'"
      :class="
        cn(
          'z-50 min-w-32 overflow-hidden rounded-md border bg-popover p-1 text-popover-foreground shadow-md',
          props.class,
        )
      "
    >
      <slot />
    </DropdownMenuContentPrimitive>
  </DropdownMenuPortal>
</template>
