<script setup>
import BaseIcon from "@/components/BaseIcon.vue";
import FormControl from "@/components/FormControl.vue";
import FormField from "@/components/FormField.vue";
import ApiService from "@/services/ApiService";
import { mdiAccountPlus, mdiLogin, mdiTunnel } from "@mdi/js";
import { computed, onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";

const route = useRoute();
const router = useRouter();

const mode = ref("login"); // "login" | "register"
const loading = ref(false);
const errorMsg = ref("");
const credentials = ref({ email: "", password: "" });

const state = computed(() => {
  const s = route.query.state;
  return typeof s === "string" ? s.trim() : "";
});
const serverId = computed(() => route.query.server_id);
const baseUrl = computed(() => {
  const u = route.query.base_url;
  return typeof u === "string" ? (u || "").trim() : "";
});
const serverName = computed(() => {
  const n = route.query.server_name;
  return typeof n === "string" ? n : "Tunnel server";
});

// Origin where the Redock app runs; callback must point there (tunnel-auth may run on tunnel server).
const redirectOrigin = computed(() => {
  const o = route.query.redirect_origin;
  return typeof o === "string" ? o.trim() : "";
});

// Callback URL must target the Redock app backend, not the tunnel server (use redirect_origin from client).
const callbackUrl = computed(() => {
  if (!state.value || !baseUrl.value) return "";
  const origin = redirectOrigin.value || window.location.origin;
  return (
    origin.replace(/\/$/, "") +
    "/api/v1/tunnel/auth/callback?state=" +
    encodeURIComponent(state.value)
  );
});

const isValid = computed(() => {
  if (!state.value || !baseUrl.value) return false;
  return !!(credentials.value.email?.trim() && credentials.value.password);
});

const goBack = () => {
  router.push("/tunnel-proxy-client");
};

const submit = async () => {
  if (!isValid.value || !baseUrl.value) return;
  errorMsg.value = "";
  loading.value = true;
  try {
    let token;
    if (mode.value === "register") {
      const res = await ApiService.tunnelRegisterExternal(
        baseUrl.value,
        credentials.value.email,
        credentials.value.password
      );
      token = res?.data?.data?.token ?? res?.data?.token;
    } else {
      const res = await ApiService.tunnelLoginExternal(
        baseUrl.value,
        credentials.value.email,
        credentials.value.password
      );
      token = res?.data?.data?.token ?? res?.data?.token;
    }
    if (!token) {
      errorMsg.value = "Login or registration failed; no token received.";
      return;
    }
    
    const callback = callbackUrl.value;
    if (!callback) {
      errorMsg.value = "Invalid state or base_url.";
      return;
    }
    const tokenParams =
      "tunnel_token=" +
      encodeURIComponent(token) +
      "&tunnel_base_url=" +
      encodeURIComponent(baseUrl.value) +
      (serverId.value ? "&server=" + encodeURIComponent(serverId.value) : "");
    const sep = callback.includes("?") ? "&" : "?";
    window.location.href = callback + sep + tokenParams;
  } catch (e) {
    const msg = e.response?.data?.msg || e.message || "Login or registration failed.";
    errorMsg.value = msg;
  } finally {
    loading.value = false;
  }
};

onMounted(() => {
  if (!state.value) {
    errorMsg.value = "state is required. Use Connect from the Tunnel Proxy Client page.";
  }
  if (state.value && !baseUrl.value) {
    errorMsg.value = "base_url is required.";
  }
});
</script>

<template>
  <div class="min-h-screen flex items-center justify-center bg-gradient-to-br from-slate-100 to-slate-200 dark:from-slate-900 dark:to-slate-800 p-4">
    <div class="w-full max-w-md">
      <div class="bg-white dark:bg-slate-800 rounded-2xl shadow-xl p-8">
        <div class="flex items-center justify-center gap-3 mb-6">
          <BaseIcon :path="mdiTunnel" size="40" class="text-purple-600 dark:text-purple-400" />
          <h1 class="text-2xl font-bold text-slate-800 dark:text-slate-100">
            Sign in to tunnel server
          </h1>
        </div>
        <p class="text-slate-600 dark:text-slate-400 text-sm mb-6">
          Sign in or register for {{ serverName }} ({{ baseUrl || "—" }}).
        </p>

        <div v-if="!state || !baseUrl" class="rounded-lg bg-red-50 dark:bg-red-900/20 p-4 mb-6">
          <p class="text-sm text-red-700 dark:text-red-300">
            {{ !state ? "state is missing." : "base_url is missing." }} Please open the Tunnel Proxy Client page and use "Connect" to get here.
          </p>
          <button
            type="button"
            class="mt-3 text-sm font-medium text-purple-600 dark:text-purple-400 hover:underline"
            @click="goBack"
          >
            Back to Tunnel Proxy Client
          </button>
        </div>

        <template v-else-if="state && baseUrl">
          <div class="flex gap-2 mb-6">
            <button
              type="button"
              :class="
                mode === 'login'
                  ? 'bg-purple-600 text-white'
                  : 'bg-slate-200 dark:bg-slate-700 text-slate-600 dark:text-slate-400'
              "
              class="flex-1 px-4 py-2 rounded-lg text-sm font-medium flex items-center justify-center gap-2"
              @click="mode = 'login'"
            >
              <BaseIcon :path="mdiLogin" size="18" />
              Sign in
            </button>
            <button
              type="button"
              :class="
                mode === 'register'
                  ? 'bg-purple-600 text-white'
                  : 'bg-slate-200 dark:bg-slate-700 text-slate-600 dark:text-slate-400'
              "
              class="flex-1 px-4 py-2 rounded-lg text-sm font-medium flex items-center justify-center gap-2"
              @click="mode = 'register'"
            >
              <BaseIcon :path="mdiAccountPlus" size="18" />
              Register
            </button>
          </div>

          <form class="space-y-4" @submit.prevent="submit">
            <FormField label="E-posta">
              <FormControl
                v-model="credentials.email"
                type="email"
                placeholder="email@example.com"
              />
            </FormField>
            <FormField label="Şifre">
              <FormControl
                v-model="credentials.password"
                type="password"
                placeholder="••••••••"
              />
            </FormField>

            <div v-if="errorMsg" class="rounded-lg bg-red-50 dark:bg-red-900/20 p-3">
              <p class="text-sm text-red-700 dark:text-red-300">{{ errorMsg }}</p>
            </div>

            <div class="flex gap-3 pt-2">
              <button
                type="button"
                class="flex-1 px-4 py-2 rounded-lg border border-slate-300 dark:border-slate-600 text-slate-700 dark:text-slate-300 text-sm font-medium"
                @click="goBack"
              >
                Cancel
              </button>
              <button
                type="submit"
                :disabled="!isValid || loading"
                class="flex-1 px-4 py-2 rounded-lg bg-purple-600 text-white text-sm font-medium disabled:opacity-50 flex items-center justify-center gap-2"
              >
                <span v-if="loading" class="animate-spin rounded-full h-4 w-4 border-2 border-white border-t-transparent" />
                <span v-else>{{ mode === "register" ? "Register" : "Sign in" }}</span>
              </button>
            </div>
          </form>
        </template>
      </div>
    </div>
  </div>
</template>
