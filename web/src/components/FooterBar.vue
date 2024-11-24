<script setup>
import { containerMaxW } from '@/config.js'
import BaseLevel from '@/components/BaseLevel.vue'
import JustboilLogo from '@/components/JustboilLogo.vue'

const year = new Date().getFullYear()
</script>

<template>
  <footer class="py-2 px-6" :class="containerMaxW">
    <BaseLevel>
      <div class="text-center md:text-left">
        <b>&copy;2022, <a href="https://onur.resoft.org/" target="_blank">Redock</a>.</b>
        <slot />
      </div>
    </BaseLevel>
  </footer>
</template>
