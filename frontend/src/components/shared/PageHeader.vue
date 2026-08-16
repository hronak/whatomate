<script setup lang="ts">
import { Button } from "@/components/ui/button";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";
import { ArrowLeft } from "@lucide/vue";
import type { Component } from "vue";

defineProps<{
  title: string;
  description?: string;
  icon?: Component;
  backLink?: string;
  breadcrumbs?: Array<{ label: string; href?: string }>;
}>();
</script>

<template>
  <header class="border-b border-border bg-background/95 backdrop-blur-sm">
    <div class="flex h-16 items-center px-6">
      <RouterLink v-if="backLink" :to="backLink">
        <Button variant="ghost" size="icon" class="mr-3">
          <ArrowLeft class="size-5" />
        </Button>
      </RouterLink>
      <div
        v-if="icon"
        class="size-8 rounded-lg border bg-muted text-foreground flex items-center justify-center mr-3"
      >
        <component :is="icon" class="size-4" />
      </div>
      <div class="flex-1">
        <h1 class="text-xl font-semibold text-foreground">{{ title }}</h1>
        <template v-if="breadcrumbs?.length">
          <Breadcrumb>
            <BreadcrumbList>
              <template v-for="(crumb, index) in breadcrumbs" :key="index">
                <BreadcrumbItem>
                  <BreadcrumbLink v-if="crumb.href" :href="crumb.href">
                    {{ crumb.label }}
                  </BreadcrumbLink>
                  <BreadcrumbPage v-else>{{ crumb.label }}</BreadcrumbPage>
                </BreadcrumbItem>
                <BreadcrumbSeparator v-if="index < breadcrumbs.length - 1" />
              </template>
            </BreadcrumbList>
          </Breadcrumb>
        </template>
        <p v-else-if="description" class="text-foreground/50">
          {{ description }}
        </p>
      </div>
      <slot name="actions" />
    </div>
  </header>
</template>
