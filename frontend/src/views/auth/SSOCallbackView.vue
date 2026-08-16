<script setup lang="ts">
import { Spinner } from "@/components/shared";
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { useAuthStore } from "@/stores/auth";
import { api } from "@/services/api";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { AlertCircle, CheckCircle } from "@lucide/vue";
import { toast } from "vue-sonner";

const { t } = useI18n();
const router = useRouter();
const authStore = useAuthStore();

const status = ref<"loading" | "success" | "error">("loading");
const errorMessage = ref("");

onMounted(async () => {
  try {
    // Cookies were set by the SSO redirect — call /me to get the user object
    const response = await api.get("/me");
    const user = response.data.data;

    // Set auth in store (only user — tokens are in httpOnly cookies)
    authStore.setAuth({ user });

    status.value = "success";
    toast.success(t("auth.ssoLoginSuccess"));

    // Redirect based on role
    setTimeout(() => {
      if (user.role?.name === "agent") {
        router.push("/analytics/agents");
      } else {
        router.push("/");
      }
    }, 1000);
  } catch (error: any) {
    status.value = "error";
    errorMessage.value =
      error.response?.data?.message || t("auth.ssoLoginFailed");
  }
});
</script>

<template>
  <div
    class="min-h-screen flex items-center justify-center bg-background p-4"
  >
    <Card class="w-full max-w-md">
      <CardHeader class="text-center">
        <div class="flex justify-center mb-4">
          <div
            v-if="status === 'loading'"
            class="size-12 rounded-xl bg-primary/10 flex items-center justify-center"
          >
            <Spinner class="size-7 text-primary" />
          </div>
          <div
            v-else-if="status === 'success'"
            class="size-12 rounded-xl bg-success/30 flex items-center justify-center"
          >
            <CheckCircle class="size-7 text-success" />
          </div>
          <div
            v-else
            class="size-12 rounded-xl bg-destructive/30 flex items-center justify-center"
          >
            <AlertCircle class="size-7 text-destructive" />
          </div>
        </div>
        <CardTitle class="text-xl">
          <template v-if="status === 'loading'">{{
            $t("auth.ssoLoading")
          }}</template>
          <template v-else-if="status === 'success'">{{
            $t("auth.ssoSuccess")
          }}</template>
          <template v-else>{{ $t("auth.ssoFailed") }}</template>
        </CardTitle>
        <CardDescription>
          <template v-if="status === 'loading'">{{
            $t("auth.ssoLoadingDesc")
          }}</template>
          <template v-else-if="status === 'success'">{{
            $t("auth.ssoSuccessDesc")
          }}</template>
          <template v-else>{{ errorMessage }}</template>
        </CardDescription>
      </CardHeader>
      <CardContent v-if="status === 'error'" class="text-center">
        <RouterLink to="/login" class="text-primary hover:underline">
          {{ $t("auth.returnToLogin") }}
        </RouterLink>
      </CardContent>
    </Card>
  </div>
</template>
