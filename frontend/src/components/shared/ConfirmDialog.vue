<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";

const { t } = useI18n();

const open = defineModel<boolean>("open", { default: false });

const props = withDefaults(
  defineProps<{
    title: string;
    description?: string;
    /** Convenience for the destructive/delete flavor: fills the default description with the item's name. */
    itemName?: string;
    confirmLabel?: string;
    cancelLabel?: string;
    variant?: "default" | "destructive";
    isSubmitting?: boolean;
  }>(),
  {
    variant: "default",
    isSubmitting: false,
  },
);

const emit = defineEmits<{
  confirm: [];
  cancel: [];
}>();

const resolvedDescription = computed(() => {
  if (props.description) return props.description;
  if (props.variant !== "destructive") return "";
  return props.itemName
    ? t("common.deleteItemWarning", { item: props.itemName })
    : t("common.deleteItemWarningGeneric");
});

const resolvedConfirmLabel = computed(
  () =>
    props.confirmLabel ??
    (props.variant === "destructive"
      ? t("common.delete")
      : t("common.confirm")),
);

const resolvedCancelLabel = computed(
  () => props.cancelLabel ?? t("common.cancel"),
);

function handleConfirm() {
  emit("confirm");
}

function handleCancel() {
  open.value = false;
  emit("cancel");
}
</script>

<template>
  <AlertDialog v-model:open="open">
    <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogTitle>{{ title }}</AlertDialogTitle>
        <AlertDialogDescription>
          <slot name="description">
            {{ resolvedDescription }}
          </slot>
        </AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancel :disabled="isSubmitting" @click="handleCancel">
          {{ resolvedCancelLabel }}
        </AlertDialogCancel>
        <Button
          :variant="variant === 'destructive' ? 'destructive' : 'default'"
          :loading="isSubmitting"
          @click="handleConfirm"
        >
          {{ resolvedConfirmLabel }}
        </Button>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>
</template>
