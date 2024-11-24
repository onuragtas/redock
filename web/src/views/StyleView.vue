<script setup>
import { gradientBgPurplePink } from '@/colors.js'
import CardBox from '@/components/CardBox.vue'
import SectionMain from '@/components/SectionMain.vue'
import LayoutGuest from '@/layouts/LayoutGuest.vue'
import { useDarkModeStore } from '@/stores/darkMode.js'
import { useRouter } from 'vue-router'

const styles = ['white', 'basic']

const darkModeStore = useDarkModeStore()

darkModeStore.set(true)

const router = useRouter()

document.documentElement.classList.forEach((token) => {
      document.documentElement.classList.replace(token, `style-white`)
  })

  router.push('/dashboard')
</script>

<template>
  <LayoutGuest>
    <div :class="gradientBgPurplePink" class="flex min-h-screen items-center justify-center">
      <SectionMain>
        <h1 class="text-4xl md:text-5xl text-center text-white font-bold mt-12 mb-3 lg:mt-0">
          Pick a style&hellip;
        </h1>
        <h2 class="text-xl md:text-xl text-center text-white mb-12">
          Style switching with a single
          <code class="px-1.5 py-0.5 rounded bg-white bg-opacity-20">modifier</code>
        </h2>
        <div class="grid gap-6 grid-cols-1 lg:grid-cols-2 px-6 max-w-6xl mx-auto">
          <CardBox
            v-for="style in styles"
            :key="style"
            class="cursor-pointer bg-gray-50"
            is-hoverable
            @click="handleStyleChange(style)"
          >
            <div class="mb-3 md:mb-6">
              <img
                :src="`https://static.justboil.me/templates/one/small/${style}-v3.png`"
                width="1280"
                height="720"
              />
            </div>

            <h1 class="text-xl md:text-2xl font-black capitalize">
              {{ style }}
            </h1>
            <h2 class="text-lg md:text-xl">& Dark mode</h2>
          </CardBox>
        </div>
      </SectionMain>
    </div>
  </LayoutGuest>
</template>
