<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  MessageSquare,
  ChevronLeft,
  ChevronRight,
  Menu,
  X
} from '@lucide/vue'
import { wsService } from '@/services/websocket'
import { authService } from '@/services/api'
import OrganizationSwitcher from './OrganizationSwitcher.vue'
import UserMenu from './UserMenu.vue'
import ActiveCallPanel from '@/components/calling/ActiveCallPanel.vue'
import { ScrollToTop } from '@/components/shared'
import { navigationSections, type NavItem, type NavSection } from './navigation'

type FilteredNavItem = NavItem & { active: boolean; children?: NavItem[] }

useI18n() // Enable $t() in template

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const isCollapsed = ref(false)
const isMobileMenuOpen = ref(false)

// Refresh user data and connect WebSocket on mount
onMounted(() => {
  if (authStore.isAuthenticated) {
    // Fetch fresh permissions in background (non-destructive — interceptor handles 401)
    authStore.refreshUserData()

    wsService.connect(async () => {
      try {
        const resp = await authService.getWSToken()
        return resp.data.data.token
      } catch {
        return null
      }
    })
  }
})

function filterItems(items: NavSection['items']) {
  return items
    .filter(item => {
      if (item.childPermissions) {
        return item.childPermissions.some(p => authStore.hasPermission(p, 'read'))
      }
      return !item.permission || authStore.hasPermission(item.permission, 'read')
    })
    .map(item => {
      const filteredChildren = item.children?.filter(
        child => !child.permission || authStore.hasPermission(child.permission, 'read')
      )

      let effectivePath = item.path
      if (item.childPermissions && item.permission && !authStore.hasPermission(item.permission, 'read') && filteredChildren?.length) {
        effectivePath = filteredChildren[0].path
      }

      const originalPath = item.path
      const isActive = originalPath === '/'
        ? route.name === 'dashboard'
        : originalPath === '/chat'
          ? route.name === 'chat' || route.name === 'chat-conversation'
          : route.path.startsWith(originalPath)

      return {
        ...item,
        path: effectivePath,
        active: isActive,
        children: filteredChildren
      }
    })
}

// Filter navigation sections based on user permissions
const navSections = computed(() => {
  return navigationSections
    .map(section => ({
      ...section,
      items: filterItems(section.items)
    }))
    .filter(section => section.items.length > 0)
})

const mainSections = computed(() => navSections.value.filter(s => !s.pinBottom))
const bottomSections = computed(() => navSections.value.filter(s => s.pinBottom))

/**
 * The bottom-pinned item (Settings) whose sub-pages get their own column on
 * md+, the way Chat splits contacts from the conversation. Below md the
 * column is dropped and the same children render nested in the drawer.
 */
const activeSubNav = computed<FilteredNavItem | null>(() => {
  for (const section of bottomSections.value) {
    const item = section.items.find(i => i.active && i.children?.length)
    if (item) return item
  }
  return null
})

/** Consecutive children sharing a `group` collapse into one labelled block. */
const subNavGroups = computed(() => {
  const groups: { label: string; items: NavItem[] }[] = []
  for (const child of activeSubNav.value?.children ?? []) {
    const label = child.group ?? ''
    const last = groups[groups.length - 1]
    if (last && last.label === label) {
      last.items.push(child)
    } else {
      groups.push({ label, items: [child] })
    }
  }
  return groups
})

const toggleSidebar = () => {
  isCollapsed.value = !isCollapsed.value
}

const handleLogout = async () => {
  await authStore.logout()
  router.push('/login')
}
</script>

<template>
  <div class="flex h-screen bg-shell">
    <!-- Skip link for accessibility -->
    <a href="#main-content" class="skip-link">{{ $t('nav.skipToMain') }}</a>

    <!-- Mobile header -->
    <header class="fixed top-0 left-0 right-0 z-50 flex h-12 items-center justify-between border-b border-border bg-sidebar/95 backdrop-blur-sm px-3 md:hidden">
      <RouterLink to="/" class="flex items-center gap-2">
        <div class="size-7 rounded-lg bg-linear-to-br from-emerald-500 to-green-600 flex items-center justify-center shadow-lg shadow-emerald-500/20">
          <MessageSquare class="size-4 text-white" />
        </div>
        <span class="font-semibold text-foreground">Whatomate</span>
      </RouterLink>
      <Button
        variant="ghost"
        size="icon"
        class="size-8 text-foreground/70 hover:text-foreground hover:bg-accent"
        aria-label="Toggle menu"
        :aria-expanded="isMobileMenuOpen"
        @click="isMobileMenuOpen = !isMobileMenuOpen"
      >
        <X v-if="isMobileMenuOpen" class="size-5" />
        <Menu v-else class="size-5" />
      </Button>
    </header>

    <!-- Mobile menu overlay -->
    <div
      v-if="isMobileMenuOpen"
      class="fixed inset-0 z-40 bg-black/30 dark:bg-black/60 backdrop-blur-sm md:hidden"
      @click="isMobileMenuOpen = false"
    />

    <!-- Sidebar -->
    <aside
      :class="[ 'flex flex-col border-r border-border bg-sidebar transition-all duration-300', 'fixed inset-y-0 left-0 z-40 md:relative', 'transform md:transform-none', isMobileMenuOpen ? 'translate-x-0' : '-translate-x-full md:translate-x-0', isCollapsed ? 'w-64 md:w-16' : 'w-64' ]"
      role="navigation"
      aria-label="Main navigation"
    >
      <!-- Logo (hidden on mobile, shown in header instead) -->
      <div class="hidden md:flex h-12 items-center justify-between px-3 border-b border-border">
        <RouterLink to="/" class="flex items-center gap-2">
          <div class="size-7 rounded-lg bg-linear-to-br from-emerald-500 to-green-600 flex items-center justify-center shadow-lg shadow-emerald-500/20">
            <MessageSquare class="size-4 text-white" />
          </div>
          <span
            v-if="!isCollapsed"
            class="font-semibold text-foreground"
          >
            Whatomate
          </span>
        </RouterLink>
        <Button
          variant="ghost"
          size="icon"
          class="size-7 text-foreground/50 hover:text-foreground hover:bg-accent"
          :aria-label="isCollapsed ? $t('nav.expandSidebar') : $t('nav.collapseSidebar')"
          :aria-expanded="!isCollapsed"
          @click="toggleSidebar"
        >
          <ChevronLeft v-if="!isCollapsed" class="size-3.5" />
          <ChevronRight v-else class="size-3.5" />
        </Button>
      </div>
      <!-- Mobile logo spacer -->
      <div class="h-12 md:hidden" />

      <!-- Organization Switcher (Super Admin only) -->
      <OrganizationSwitcher :collapsed="isCollapsed" />

      <!-- Navigation -->
      <ScrollArea class="flex-1 py-2">
        <nav class="px-2" role="menubar">
          <template v-for="(section, sIdx) in mainSections" :key="section.label">
            <!-- Section header -->
            <div
              v-if="section.label && !isCollapsed"
              :class="['px-2.5 pt-4 pb-1 font-semibold uppercase tracking-wider text-foreground/45', sIdx === 0 && 'pt-1']"
            >
              {{ $t(section.label) }}
            </div>
            <div v-else-if="sIdx > 0" :class="['my-2 mx-2.5 border-t border-border', isCollapsed && 'mx-1']" />

            <!-- Section items -->
            <div class="space-y-0.5">
              <template v-for="item in section.items" :key="item.path">
                <RouterLink
                  :to="item.path"
                  :class="[ 'nav-active-indicator btn-press flex items-center gap-2.5 rounded-lg px-2.5 py-2 font-medium transition-all duration-200', item.active ? 'bg-muted text-foreground' : 'text-foreground/50 hover:text-foreground hover:bg-accent', isCollapsed && 'md:justify-center md:px-2' ]"
                  :data-active="item.active"
                  role="menuitem"
                  :aria-current="item.active ? 'page' : undefined"
                  @click="isMobileMenuOpen = false"
                >
                  <component :is="item.icon" class="size-4 shrink-0" aria-hidden="true" />
                  <span :class="isCollapsed && 'md:sr-only'">{{ $t(item.name) }}</span>
                </RouterLink>

                <!-- Submenu items -->
                <template v-if="item.children && item.active && !isCollapsed">
                  <RouterLink
                    v-for="child in item.children"
                    :key="child.path"
                    :to="child.path"
                    :class="[ 'flex items-center gap-2.5 rounded-lg px-2.5 py-1.5 font-medium transition-all duration-200 ml-4', route.path === child.path ? 'bg-muted text-foreground' : 'text-foreground/40 hover:text-foreground/70 hover:bg-accent' ]"
                    role="menuitem"
                    :aria-current="route.path === child.path ? 'page' : undefined"
                    @click="isMobileMenuOpen = false"
                  >
                    <component :is="child.icon" class="size-3.5 shrink-0" aria-hidden="true" />
                    <span>{{ $t(child.name) }}</span>
                  </RouterLink>
                </template>
              </template>
            </div>
          </template>
        </nav>
      </ScrollArea>

      <!-- Bottom-pinned navigation (Settings) -->
      <div v-if="bottomSections.length > 0" class="border-t border-border px-2 py-2">
        <template v-for="section in bottomSections" :key="section.label">
          <template v-for="item in section.items" :key="item.path">
            <RouterLink
              :to="item.path"
              :class="[ 'nav-active-indicator btn-press flex items-center gap-2.5 rounded-lg px-2.5 py-2 font-medium transition-all duration-200', item.active ? 'bg-muted text-foreground' : 'text-foreground/50 hover:text-foreground hover:bg-accent', isCollapsed && 'md:justify-center md:px-2' ]"
              :data-active="item.active"
              role="menuitem"
              :aria-current="item.active ? 'page' : undefined"
              @click="isMobileMenuOpen = false"
            >
              <component :is="item.icon" class="size-4 shrink-0" aria-hidden="true" />
              <span :class="isCollapsed && 'md:sr-only'">{{ $t(item.name) }}</span>
            </RouterLink>

            <!-- Children live in the sub-nav column on md+; nest them in the drawer below it -->
            <template v-if="item.children && item.active">
              <RouterLink
                v-for="child in item.children"
                :key="child.path"
                :to="child.path"
                :class="[ 'md:hidden flex items-center gap-2.5 rounded-lg px-2.5 py-1.5 font-medium transition-all duration-200 ml-4', route.path === child.path ? 'bg-muted text-foreground' : 'text-foreground/40 hover:text-foreground/70 hover:bg-accent' ]"
                role="menuitem"
                :aria-current="route.path === child.path ? 'page' : undefined"
                @click="isMobileMenuOpen = false"
              >
                <component :is="child.icon" class="size-3.5 shrink-0" aria-hidden="true" />
                <span>{{ $t(child.name) }}</span>
              </RouterLink>
            </template>
          </template>
        </template>
      </div>

      <!-- User Menu -->
      <UserMenu :collapsed="isCollapsed" @logout="handleLogout" />
    </aside>

    <!-- Sub-navigation column (Settings) -->
    <aside
      v-if="activeSubNav"
      class="hidden md:flex w-56 shrink-0 flex-col border-r border-border bg-subnav"
      role="navigation"
      :aria-label="$t(activeSubNav.name)"
    >
      <div class="flex h-12 shrink-0 items-center gap-2 px-3 border-b border-border">
        <component :is="activeSubNav.icon" class="size-4 shrink-0 text-foreground/60" aria-hidden="true" />
        <span class="font-semibold text-foreground truncate">{{ $t(activeSubNav.name) }}</span>
      </div>
      <ScrollArea class="flex-1 py-2">
        <nav class="px-2" role="menu">
          <template v-for="(group, gIdx) in subNavGroups" :key="group.label || gIdx">
            <div
              v-if="group.label"
              :class="['px-2.5 pt-4 pb-1 font-semibold uppercase tracking-wider text-foreground/45', gIdx === 0 && 'pt-1']"
            >
              {{ $t(group.label) }}
            </div>
            <div class="space-y-0.5">
              <RouterLink
                v-for="child in group.items"
                :key="child.path"
                :to="child.path"
                :class="[ 'nav-active-indicator btn-press flex items-center gap-2.5 rounded-lg px-2.5 py-1.5 font-medium transition-all duration-200', route.path === child.path ? 'bg-muted text-foreground' : 'text-foreground/50 hover:text-foreground hover:bg-accent' ]"
                :data-active="route.path === child.path"
                role="menuitem"
                :aria-current="route.path === child.path ? 'page' : undefined"
              >
                <component :is="child.icon" class="size-4 shrink-0" aria-hidden="true" />
                <span class="truncate">{{ $t(child.name) }}</span>
              </RouterLink>
            </div>
          </template>
        </nav>
      </ScrollArea>
    </aside>

    <!-- Main content -->
    <main id="main-content" class="flex-1 overflow-hidden pt-12 md:pt-0 bg-shell" role="main">
      <RouterView v-slot="{ Component, route: viewRoute }">
        <Transition name="page" mode="out-in">
          <component :is="Component" :key="viewRoute.meta.stableKey ? String(viewRoute.name) : viewRoute.path" />
        </Transition>
      </RouterView>
      <ActiveCallPanel />
      <ScrollToTop />
    </main>
  </div>
</template>
