<script setup>
import { mdiTranslate, mdiCheck } from '@mdi/js'
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseIcon from '@/components/BaseIcon.vue'
import { setLocale, SUPPORTED_LOCALES } from '@/i18n'

const { locale, t } = useI18n()
const open = ref(false)

const choose = (lang) => {
  setLocale(lang)
  open.value = false
}
</script>

<template>
  <div class="relative">
    <button
      class="flex items-center space-x-1 p-2 text-gray-400 hover:text-white hover:bg-gray-700/50 rounded-lg transition-colors"
      :title="t('language.label')"
      @click.stop="open = !open"
    >
      <BaseIcon :path="mdiTranslate" size="20" />
      <span class="hidden md:block text-xs font-semibold uppercase">{{ locale }}</span>
    </button>

    <!-- Click-away backdrop -->
    <div v-if="open" class="fixed inset-0 z-40" @click="open = false"></div>

    <div
      v-show="open"
      class="absolute right-0 mt-2 w-40 bg-gray-800 border border-gray-700 rounded-lg shadow-xl py-1 z-50"
    >
      <button
        v-for="lang in SUPPORTED_LOCALES"
        :key="lang"
        class="w-full flex items-center justify-between px-3 py-2 text-sm text-gray-200 hover:bg-gray-700/60 transition-colors"
        @click="choose(lang)"
      >
        <span>{{ t('language.' + lang) }}</span>
        <BaseIcon v-if="locale === lang" :path="mdiCheck" size="16" class="text-green-400" />
      </button>
    </div>
  </div>
</template>
