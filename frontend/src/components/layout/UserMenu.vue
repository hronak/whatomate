<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted } from "vue";
import { RouterLink } from "vue-router";
import { useI18n } from "vue-i18n";
import { useAuthStore } from "@/stores/auth";
import { useContactsStore } from "@/stores/contacts";
import { usersService, chatbotService } from "@/services/api";
import { Button } from "@/components/ui/button";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Separator } from "@/components/ui/separator";
import { Switch } from "@/components/ui/switch";
import { Badge } from "@/components/ui/badge";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { LogOut, User } from "@lucide/vue";
import { toast } from "vue-sonner";
import { getInitials } from "@/lib/utils";
import ThemeSwitcher from "./ThemeSwitcher.vue";
import LanguageSwitcher from "@/components/LanguageSwitcher.vue";

const { t } = useI18n();

defineProps<{
  collapsed?: boolean;
}>();

const emit = defineEmits<{
  logout: [];
}>();

const authStore = useAuthStore();
const contactsStore = useContactsStore();
const isUserMenuOpen = ref(false);
const isUpdatingAvailability = ref(false);
const isCheckingTransfers = ref(false);
const showAwayWarning = ref(false);
const awayWarningTransferCount = ref(0);

const handleAvailabilityChange = async (checked: boolean) => {
  if (!checked) {
    isCheckingTransfers.value = true;
    try {
      const response = await chatbotService.listTransfers({ status: "active" });
      const data = response.data.data || response.data;
      const transfers = data.transfers || [];
      const userId = authStore.user?.id;
      const myActiveTransfers = transfers.filter(
        (t: any) => t.agent_id === userId,
      );

      if (myActiveTransfers.length > 0) {
        awayWarningTransferCount.value = myActiveTransfers.length;
        showAwayWarning.value = true;
        return;
      }
    } catch (error) {
      console.error("Failed to check transfers:", error);
    } finally {
      isCheckingTransfers.value = false;
    }
  }

  await setAvailability(checked);
};

const confirmGoAway = async () => {
  showAwayWarning.value = false;
  await setAvailability(false);
};

const setAvailability = async (checked: boolean) => {
  isUpdatingAvailability.value = true;
  try {
    const response = await usersService.updateAvailability(checked);
    const data = response.data.data;
    authStore.setAvailability(checked, data.break_started_at);

    if (checked) {
      toast.success(t("userMenu.available"), {
        description: t("userMenu.availableDesc"),
      });
    } else {
      const transfersReturned = data.transfers_to_queue || 0;
      toast.success(t("userMenu.away"), {
        description:
          transfersReturned > 0
            ? t("userMenu.transfersReturned", { count: transfersReturned })
            : t("userMenu.awayDesc"),
      });

      if (transfersReturned > 0) {
        contactsStore.fetchContacts();
      }
    }
  } catch (error) {
    toast.error(t("common.error"), {
      description: t("userMenu.failedUpdateAvailability"),
    });
  } finally {
    isUpdatingAvailability.value = false;
  }
};

// Break duration tracking
const breakDuration = ref("");
let breakTimerInterval: ReturnType<typeof setInterval> | null = null;

const updateBreakDuration = () => {
  if (!authStore.breakStartedAt) {
    breakDuration.value = "";
    return;
  }
  const start = new Date(authStore.breakStartedAt);
  const now = new Date();
  const diffMs = now.getTime() - start.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const hours = Math.floor(diffMins / 60);
  const mins = diffMins % 60;

  if (hours > 0) {
    breakDuration.value = `${hours}h ${mins}m`;
  } else {
    breakDuration.value = `${mins}m`;
  }
};

watch(
  () => authStore.isAvailable,
  (available) => {
    if (!available && authStore.breakStartedAt) {
      updateBreakDuration();
      breakTimerInterval = setInterval(updateBreakDuration, 60000);
    } else if (breakTimerInterval) {
      clearInterval(breakTimerInterval);
      breakTimerInterval = null;
      breakDuration.value = "";
    }
  },
  { immediate: true },
);

onMounted(() => {
  authStore.restoreBreakTime();
  if (!authStore.isAvailable && authStore.breakStartedAt) {
    updateBreakDuration();
    breakTimerInterval = setInterval(updateBreakDuration, 60000);
  }
});

onUnmounted(() => {
  if (breakTimerInterval) {
    clearInterval(breakTimerInterval);
  }
});

const handleLogout = () => {
  emit("logout");
};
</script>

<template>
  <div class="p-1">
    <Popover v-model:open="isUserMenuOpen">
      <PopoverTrigger as-child>
        <Button
          variant="ghost"
          :class="[
            'flex items-center justify-start w-full h-auto px-2 py-1.5 gap-2 hover:bg-accent rounded-lg',
            collapsed && 'md:justify-center',
          ]"
          aria-label="User menu"
        >
          <Avatar class="size-7 ring-2 ring-border">
            <AvatarImage :src="undefined" />
            <AvatarFallback
              class="bg-muted text-foreground"
            >
              {{ getInitials(authStore.user?.full_name || "U") }}
            </AvatarFallback>
          </Avatar>
          <div v-if="!collapsed" class="flex flex-col items-start text-left">
            <span class="font-medium truncate max-w-[140px] text-foreground">
              {{ authStore.user?.full_name }}
            </span>
            <span class="text-foreground/40 truncate max-w-[140px]">
              {{ authStore.user?.email }}
            </span>
          </div>
        </Button>
      </PopoverTrigger>
      <PopoverContent side="top" align="start" class="w-52 p-1.5">
        <div class="font-medium px-2 py-1 text-foreground/40">
          {{ $t("userMenu.myAccount") }}
        </div>
        <Separator class="my-1 bg-muted" />
        <!-- Availability Toggle -->
        <div class="flex items-center justify-between px-2 py-1.5">
          <div class="flex items-center gap-2">
            <span class="text-foreground/70">{{ $t("userMenu.status") }}</span>
            <Badge
              :class="
                'px-1.5 py-0 ' +
                (authStore.isAvailable
                  ? 'bg-success/20 text-success'
                  : 'bg-muted text-foreground/50')
              "
            >
              {{
                authStore.isAvailable
                  ? $t("userMenu.available")
                  : $t("userMenu.away")
              }}
            </Badge>
            <span
              v-if="!authStore.isAvailable && breakDuration"
              class="text-foreground/40"
            >
              {{ breakDuration }}
            </span>
          </div>
          <Switch
            :checked="authStore.isAvailable"
            :disabled="isUpdatingAvailability || isCheckingTransfers"
            aria-label="Toggle availability status"
            @update:checked="handleAvailabilityChange"
          />
        </div>
        <Separator class="my-1 bg-muted" />
        <RouterLink to="/profile">
          <Button
            variant="ghost"
            class="w-full justify-start px-2 py-1 h-auto font-normal text-foreground/70 hover:text-foreground hover:bg-accent"
            @click="isUserMenuOpen = false"
          >
            <User class="mr-2 size-3.5" aria-hidden="true" />
            <span>{{ $t("userMenu.profile") }}</span>
          </Button>
        </RouterLink>
        <Separator class="my-1 bg-muted" />
        <div class="font-medium px-2 py-1 text-foreground/40">
          {{ $t("userMenu.theme") }}
        </div>
        <ThemeSwitcher />
        <Separator class="my-1 bg-muted" />
        <div class="font-medium px-2 py-1 text-foreground/40">
          {{ $t("userMenu.language") }}
        </div>
        <div class="px-1.5 py-1">
          <LanguageSwitcher />
        </div>
        <Separator class="my-1 bg-muted" />
        <Button
          variant="ghost"
          class="w-full justify-start px-2 py-1 h-auto font-normal text-foreground/70 hover:text-foreground hover:bg-accent"
          @click="handleLogout"
        >
          <LogOut class="mr-2 size-3.5" aria-hidden="true" />
          <span>{{ $t("userMenu.logOut") }}</span>
        </Button>
      </PopoverContent>
    </Popover>
  </div>

  <!-- Away Warning Dialog -->
  <AlertDialog :open="showAwayWarning">
    <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogTitle>{{
          $t("userMenu.awayWarningTitle")
        }}</AlertDialogTitle>
        <AlertDialogDescription>
          {{
            $t("userMenu.awayWarningDesc", { count: awayWarningTransferCount })
          }}
        </AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <Button variant="outline" @click="showAwayWarning = false">{{
          $t("common.cancel")
        }}</Button>
        <Button @click="confirmGoAway" :disabled="isUpdatingAvailability">{{
          $t("userMenu.goAway")
        }}</Button>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>
</template>
