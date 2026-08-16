<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { RouterLink, RouterView, useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { useAuthStore } from "@/stores/auth";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { MessageSquare, ChevronLeft, ChevronRight, Menu, X, Search, ChevronDown } from "@lucide/vue";
import { wsService } from "@/services/websocket";
import { authService } from "@/services/api";
import OrganizationSwitcher from "./OrganizationSwitcher.vue";
import UserMenu from "./UserMenu.vue";
import ActiveCallPanel from "@/components/calling/ActiveCallPanel.vue";
import { ScrollToTop } from "@/components/shared";
import {
  navigationSections,
  type NavItem,
  type NavSection,
} from "./navigation";

type FilteredNavItem = NavItem & { active: boolean; children?: NavItem[] };

useI18n();

const route = useRoute();
const router = useRouter();
const authStore = useAuthStore();
const isCollapsed = ref(false);
const isMobileMenuOpen = ref(false);

onMounted(() => {
  if (authStore.isAuthenticated) {
    authStore.refreshUserData();

    wsService.connect(async () => {
      try {
        const resp = await authService.getWSToken();
        return resp.data.data.token;
      } catch {
        return null;
      }
    });
  }
});

function filterItems(items: NavSection["items"]) {
  return items
    .filter((item) => {
      if (item.childPermissions) {
        return item.childPermissions.some((p) =>
          authStore.hasPermission(p, "read"),
        );
      }
      return (
        !item.permission || authStore.hasPermission(item.permission, "read")
      );
    })
    .map((item) => {
      const filteredChildren = item.children?.filter(
        (child) =>
          !child.permission ||
          authStore.hasPermission(child.permission, "read"),
      );

      let effectivePath = item.path;
      if (
        item.childPermissions &&
        item.permission &&
        !authStore.hasPermission(item.permission, "read") &&
        filteredChildren?.length
      ) {
        effectivePath = filteredChildren[0].path;
      }

      const originalPath = item.path;
      const isActive =
        originalPath === "/"
          ? route.name === "dashboard"
          : originalPath === "/chat"
            ? route.name === "chat" || route.name === "chat-conversation"
            : route.path.startsWith(originalPath);

      return {
        ...item,
        path: effectivePath,
        active: isActive,
        children: filteredChildren,
      };
    });
}

const navSections = computed(() => {
  return navigationSections
    .map((section) => ({
      ...section,
      items: filterItems(section.items),
    }))
    .filter((section) => section.items.length > 0);
});

const mainSections = computed(() =>
  navSections.value.filter((s) => !s.pinBottom),
);
const bottomSections = computed(() =>
  navSections.value.filter((s) => s.pinBottom),
);

const getSubNavGroups = (item: FilteredNavItem) => {
  const groups: { label: string; items: NavItem[] }[] = [];
  for (const child of item.children ?? []) {
    const label = child.group ?? "";
    const last = groups[groups.length - 1];
    if (last && last.label === label) {
      last.items.push(child);
    } else {
      groups.push({ label, items: [child] });
    }
  }
  return groups;
};

const toggleSidebar = () => {
  isCollapsed.value = !isCollapsed.value;
};

const handleLogout = async () => {
  await authStore.logout();
  router.push("/login");
};

const manuallyToggled = ref<Record<string, boolean>>({});

const isGroupExpanded = (item: FilteredNavItem) => {
  if (manuallyToggled.value[item.path] !== undefined) {
    return manuallyToggled.value[item.path];
  }
  return item.active;
};

const toggleGroup = (item: FilteredNavItem) => {
  manuallyToggled.value[item.path] = !isGroupExpanded(item);
};
</script>

<template>
  <div class="flex h-screen bg-muted/30">
    <a href="#main-content" class="skip-link">{{ $t("nav.skipToMain") }}</a>

    <!-- Mobile header -->
    <header
      class="fixed top-0 left-0 right-0 z-50 flex h-14 items-center justify-between border-b border-border/60 bg-background/95 backdrop-blur-md px-3 md:hidden"
    >
      <RouterLink to="/" class="flex items-center gap-2">
        <div class="size-8 rounded-xl bg-linear-to-br from-emerald-500 to-emerald-600 flex items-center justify-center shadow-md">
          <MessageSquare class="size-4 text-white" />
        </div>
        <span class="font-semibold text-foreground">Whatomate</span>
      </RouterLink>
      <div class="flex items-center gap-1">
        <div class="w-10 overflow-hidden">
          <UserMenu :collapsed="true" @logout="handleLogout" />
        </div>
        <Button
          variant="ghost"
          size="icon"
          class="size-8"
          @click="isMobileMenuOpen = !isMobileMenuOpen"
        >
          <X v-if="isMobileMenuOpen" class="size-5" />
          <Menu v-else class="size-5" />
        </Button>
      </div>
    </header>

    <div
      v-if="isMobileMenuOpen"
      class="fixed inset-0 z-40 bg-black/40 backdrop-blur-sm md:hidden"
      @click="isMobileMenuOpen = false"
    />

    <!-- Sidebar Navigation -->
    <aside
      :class="[
        'flex flex-col border-r border-border/60 bg-background',
        'fixed inset-y-0 left-0 z-40 md:relative',
        'transform md:transform-none transition-all duration-300 ease-[cubic-bezier(0.2,0.8,0.2,1)]',
        isMobileMenuOpen ? 'translate-x-0' : '-translate-x-full md:translate-x-0',
        isCollapsed ? 'w-64 md:w-[72px]' : 'w-64',
      ]"
    >
      <!-- App Logo -->
      <div class="hidden md:flex h-[60px] items-center px-4 border-b border-border/60 shrink-0">
        <RouterLink to="/" class="flex items-center gap-3 overflow-hidden whitespace-nowrap outline-none">
          <div class="size-[34px] shrink-0 rounded-xl bg-linear-to-br from-emerald-500 to-emerald-600 flex items-center justify-center shadow-md shadow-emerald-500/10">
            <MessageSquare class="size-4 text-white" />
          </div>
          <span
            class="font-semibold text-lg tracking-tight transition-opacity duration-200"
            :class="isCollapsed ? 'opacity-0 w-0' : 'opacity-100'"
          >
            Whatomate
          </span>
        </RouterLink>
      </div>

      <ScrollArea class="flex-1">
        <div class="flex flex-col min-h-full py-4">
          <nav class="px-3" role="menubar">
            <template v-for="section in mainSections" :key="section.label">
              <div
                v-if="section.label && !isCollapsed"
                class="px-3 pt-4 pb-2 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/60"
              >
                {{ $t(section.label) }}
              </div>
              
              <div class="space-y-1 mt-1">
                <template v-for="item in section.items" :key="item.path">
                  <button
                    v-if="item.children"
                    type="button"
                    :class="[
                      'w-full text-left group flex items-center justify-between gap-3 rounded-xl px-3 py-2.5 font-medium transition-all duration-200 relative outline-none',
                      isGroupExpanded(item) || item.active
                        ? 'bg-primary/10 text-primary'
                        : 'text-muted-foreground hover:bg-muted/80 hover:text-foreground',
                      isCollapsed && 'md:justify-center'
                    ]"
                    @click="toggleGroup(item)"
                  >
                    <div class="flex items-center gap-3 overflow-hidden">
                      <component
                        :is="item.icon"
                        class="size-[18px] shrink-0 transition-transform duration-200 group-hover:scale-110"
                      />
                      <span class="truncate" :class="isCollapsed && 'md:hidden'">{{ $t(item.name) }}</span>
                    </div>
                    <ChevronDown v-if="!isCollapsed" class="size-4 shrink-0 transition-transform duration-200" :class="isGroupExpanded(item) ? 'rotate-180' : ''" />
                  </button>

                  <RouterLink
                    v-else
                    :to="item.path"
                    :class="[
                      'group flex items-center gap-3 rounded-xl px-3 py-2.5 font-medium transition-all duration-200 relative outline-none',
                      item.active
                        ? 'bg-primary/10 text-primary'
                        : 'text-muted-foreground hover:bg-muted/80 hover:text-foreground',
                      isCollapsed && 'md:justify-center'
                    ]"
                    @click="isMobileMenuOpen = false"
                  >
                    <component
                      :is="item.icon"
                      class="size-[18px] shrink-0 transition-transform duration-200 group-hover:scale-110"
                    />
                    <span :class="isCollapsed && 'md:hidden'">{{ $t(item.name) }}</span>
                  </RouterLink>

                  <div v-if="item.children && isGroupExpanded(item) && !isCollapsed" class="pl-11 pr-2 py-1 space-y-0.5">
                    <RouterLink
                      v-for="child in item.children"
                      :key="child.path"
                      :to="child.path"
                      :class="[
                        'flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors outline-none',
                        route.path === child.path
                          ? 'text-primary bg-primary/5'
                          : 'text-muted-foreground hover:text-foreground hover:bg-muted/60'
                      ]"
                      @click="isMobileMenuOpen = false"
                    >
                      <span>{{ $t(child.name) }}</span>
                    </RouterLink>
                  </div>
                </template>
              </div>
            </template>
          </nav>

          <div v-if="bottomSections.length > 0" class="mt-auto px-3 pt-6 pb-2">
            <template v-for="section in bottomSections" :key="section.label">
              <div class="space-y-1">
                <template v-for="item in section.items" :key="item.path">
                  <button
                    v-if="item.children"
                    type="button"
                    :class="[
                      'w-full text-left group flex items-center justify-between gap-3 rounded-xl px-3 py-2.5 font-medium transition-all duration-200 relative outline-none',
                      isGroupExpanded(item) || item.active
                        ? 'bg-primary/10 text-primary'
                        : 'text-muted-foreground hover:bg-muted/80 hover:text-foreground',
                      isCollapsed && 'md:justify-center'
                    ]"
                    @click="toggleGroup(item)"
                  >
                    <div class="flex items-center gap-3 overflow-hidden">
                      <component
                        :is="item.icon"
                        class="size-[18px] shrink-0 transition-transform duration-200 group-hover:scale-110"
                      />
                      <span class="truncate" :class="isCollapsed && 'md:hidden'">{{ $t(item.name) }}</span>
                    </div>
                    <ChevronDown v-if="!isCollapsed" class="size-4 shrink-0 transition-transform duration-200" :class="isGroupExpanded(item) ? 'rotate-180' : ''" />
                  </button>

                  <RouterLink
                    v-else
                    :to="item.path"
                    :class="[
                      'group flex items-center gap-3 rounded-xl px-3 py-2.5 font-medium transition-all duration-200 relative outline-none',
                      item.active
                        ? 'bg-primary/10 text-primary'
                        : 'text-muted-foreground hover:bg-muted/80 hover:text-foreground',
                      isCollapsed && 'md:justify-center'
                    ]"
                    @click="isMobileMenuOpen = false"
                  >
                    <component
                      :is="item.icon"
                      class="size-[18px] shrink-0 transition-transform duration-200 group-hover:scale-110"
                    />
                    <span :class="isCollapsed && 'md:hidden'">{{ $t(item.name) }}</span>
                  </RouterLink>

                  <div v-if="item.children && isGroupExpanded(item) && !isCollapsed" class="pl-2 pr-2 py-2 space-y-4">
                    <template v-for="(group, gIdx) in getSubNavGroups(item)" :key="group.label || gIdx">
                      <div class="space-y-0.5">
                        <div
                          v-if="group.label"
                          class="px-3 pt-1 pb-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground/50"
                        >
                          {{ $t(group.label) }}
                        </div>
                        <RouterLink
                          v-for="child in group.items"
                          :key="child.path"
                          :to="child.path"
                          :class="[
                            'flex items-center gap-3 rounded-lg px-3 py-2 text-[13px] font-medium transition-colors outline-none',
                            route.path === child.path
                              ? 'text-primary bg-primary/5'
                              : 'text-muted-foreground hover:text-foreground hover:bg-muted/60'
                          ]"
                          @click="isMobileMenuOpen = false"
                        >
                          <component :is="child.icon" class="size-4 shrink-0 text-muted-foreground/70" />
                          <span>{{ $t(child.name) }}</span>
                        </RouterLink>
                      </div>
                    </template>
                  </div>
                </template>
              </div>
            </template>
          </div>
        </div>
      </ScrollArea>

      <div class="p-3 border-t border-border/60 hidden md:flex justify-end shrink-0">
        <Button
          variant="ghost"
          size="icon"
          class="size-8 rounded-lg text-muted-foreground hover:text-foreground hover:bg-muted"
          @click="toggleSidebar"
        >
          <ChevronRight v-if="isCollapsed" class="size-4" />
          <ChevronLeft v-else class="size-4" />
        </Button>
      </div>
    </aside>

    <main class="flex-1 flex flex-col min-w-0 h-screen pt-14 md:pt-0" id="main-content">
      <!-- Desktop Global Header Rail -->
      <header class="hidden md:flex h-[60px] shrink-0 items-center justify-between px-6 border-b border-border/40 bg-background/40 backdrop-blur-xl z-10">
        <!-- Context Switcher -->
        <div class="flex items-center gap-4">
          <div class="w-56">
            <OrganizationSwitcher :collapsed="false" />
          </div>
        </div>
        
        <!-- Global Search -->
        <div class="flex-1 flex justify-center max-w-md mx-4 hidden lg:flex">
          <div class="relative w-full">
            <Search class="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <Input 
              type="text" 
              placeholder="Search..." 
              class="w-full pl-9 rounded-full bg-muted/40 border-border/60 focus-visible:bg-background transition-colors h-9"
            />
          </div>
        </div>
        
        <!-- User Actions -->
        <div class="flex items-center gap-3 shrink-0">
          <UserMenu :collapsed="false" @logout="handleLogout" />
        </div>
      </header>

      <!-- Main Canvas with Floating Surface -->
      <div class="flex-1 p-2 md:p-5 lg:p-6 overflow-hidden flex flex-col relative z-0">
        <div class="flex-1 rounded-2xl border border-border/40 bg-card shadow-sm overflow-hidden flex flex-col relative">
          <RouterView v-slot="{ Component, route: viewRoute }">
            <component
              :is="Component"
              :key="viewRoute.meta.stableKey ? String(viewRoute.name) : viewRoute.path"
            />
          </RouterView>
        </div>
      </div>
      
      <ActiveCallPanel />
      <ScrollToTop />
    </main>
  </div>
</template>
