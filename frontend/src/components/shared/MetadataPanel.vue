<script setup lang="ts">
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { formatDateTime } from "@/lib/utils";
import { Clock, UserCircle } from "@lucide/vue";

defineProps<{
  createdAt?: string;
  updatedAt?: string;
  createdByName?: string;
  updatedByName?: string;
}>();
</script>

<template>
  <Card class="overflow-hidden">
    <CardHeader class="pb-3">
      <CardTitle class="font-medium">{{
        $t("common.metadata", "Metadata")
      }}</CardTitle>
    </CardHeader>
    <CardContent class="space-y-2.5">
      <div v-if="createdByName" class="flex items-start gap-2">
        <UserCircle class="size-3.5 text-muted-foreground shrink-0 mt-0.5" />
        <div class="min-w-0">
          <span class="text-muted-foreground">{{
            $t("common.createdBy", "Created by")
          }}</span>
          <p class="font-medium truncate">{{ createdByName }}</p>
        </div>
      </div>
      <div v-if="createdAt" class="flex items-start gap-2">
        <Clock class="size-3.5 text-muted-foreground shrink-0 mt-0.5" />
        <div class="min-w-0">
          <span class="text-muted-foreground">{{
            $t("common.createdAt", "Created")
          }}</span>
          <p class="truncate">{{ formatDateTime(createdAt) }}</p>
        </div>
      </div>

      <Separator
        v-if="(createdByName || createdAt) && (updatedByName || updatedAt)"
      />

      <div v-if="updatedByName" class="flex items-start gap-2">
        <UserCircle class="size-3.5 text-muted-foreground shrink-0 mt-0.5" />
        <div class="min-w-0">
          <span class="text-muted-foreground">{{
            $t("common.updatedBy", "Modified by")
          }}</span>
          <p class="font-medium truncate">{{ updatedByName }}</p>
        </div>
      </div>
      <div v-if="updatedAt" class="flex items-start gap-2">
        <Clock class="size-3.5 text-muted-foreground shrink-0 mt-0.5" />
        <div class="min-w-0">
          <span class="text-muted-foreground">{{
            $t("common.lastUpdated", "Last updated")
          }}</span>
          <p class="truncate">{{ formatDateTime(updatedAt) }}</p>
        </div>
      </div>
    </CardContent>
  </Card>
</template>
