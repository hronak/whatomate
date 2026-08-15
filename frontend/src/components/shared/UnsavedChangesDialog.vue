<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import ConfirmDialog from './ConfirmDialog.vue'

const { t } = useI18n()

const props = defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  stay: []
  leave: []
}>()

const isOpen = computed({
  get: () => props.open,
  set: (value: boolean) => {
    if (!value) emit('stay')
  },
})
</script>

<template>
  <ConfirmDialog
    v-model:open="isOpen"
    :title="t('common.unsavedChangesTitle')"
    :description="t('common.unsavedChangesDesc')"
    :confirm-label="t('common.leave')"
    :cancel-label="t('common.stay')"
    variant="destructive"
    @confirm="emit('leave')"
  />
</template>
