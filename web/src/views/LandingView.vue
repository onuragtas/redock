<script setup>
import { mdiGithub, mdiMonitor, mdiCloud, mdiCog, mdiDatabase, mdiCodeBraces } from '@mdi/js'
import SectionFullScreen from '@/components/SectionFullScreen.vue'
import CardBox from '@/components/CardBox.vue'
import BaseButton from '@/components/BaseButton.vue'
import BaseButtons from '@/components/BaseButtons.vue'
import LayoutGuest from '@/layouts/LayoutGuest.vue'
import IconRounded from '@/components/IconRounded.vue'
</script>

<template>
  <LayoutGuest>
    <div class="min-h-screen bg-gradient-to-br from-purple-400 via-pink-500 to-red-500">
      <!-- Hero Section -->
      <div class="container mx-auto px-6 py-24">
        <div class="text-center">
          <h1 class="text-5xl font-bold text-white mb-6">Redock</h1>
          <p class="text-xl text-white mb-12">A powerful Docker environment management tool</p>
          <BaseButtons>
            <BaseButton to="/login" color="info" label="Get Started" />
            <BaseButton 
              href="https://github.com/onuragtas/redock" 
              target="_blank"
              :icon="mdiGithub"
              label="GitHub"
              color="contrast"
              outline
            />
          </BaseButtons>
        </div>
      </div>

      <!-- Features Section -->
      <div class="bg-white py-24">
        <div class="container mx-auto px-6">
          <h2 class="text-3xl font-bold text-center mb-16">Key Features</h2>
          
          <div class="grid md:grid-cols-2 lg:grid-cols-3 gap-12">
            <!-- Feature 1 -->
            <div class="text-center">
              <IconRounded :icon="mdiMonitor" color="info" class="mx-auto mb-4" size="48" />
              <h3 class="text-xl font-semibold mb-4">Environment Management</h3>
              <p class="text-gray-600">Easily manage your Docker environments with an intuitive interface</p>
            </div>

            <!-- Feature 2 -->
            <div class="text-center">
              <IconRounded :icon="mdiCloud" color="info" class="mx-auto mb-4" size="48" />
              <h3 class="text-xl font-semibold mb-4">Tunnel Proxy</h3>
              <p class="text-gray-600">Secure tunnel proxy for remote access to your services</p>
            </div>

            <!-- Feature 3 -->
            <div class="text-center">
              <IconRounded :icon="mdiCog" color="info" class="mx-auto mb-4" size="48" />
              <h3 class="text-xl font-semibold mb-4">Virtual Hosts</h3>
              <p class="text-gray-600">Configure and manage virtual hosts with ease</p>
            </div>

            <!-- Feature 4 -->
            <div class="text-center">
              <IconRounded :icon="mdiDatabase" color="info" class="mx-auto mb-4" size="48" />
              <h3 class="text-xl font-semibold mb-4">Local Proxy</h3>
              <p class="text-gray-600">Manage local proxy configurations efficiently</p>
            </div>

            <!-- Feature 5 -->
            <div class="text-center">
              <IconRounded :icon="mdiCodeBraces" color="info" class="mx-auto mb-4" size="48" />
              <h3 class="text-xl font-semibold mb-4">PHP XDebug Adapter</h3>
              <p class="text-gray-600">Built-in support for PHP debugging with XDebug</p>
            </div>

            <!-- Feature 6 -->
            <div class="text-center">
              <IconRounded :icon="mdiMonitor" color="info" class="mx-auto mb-4" size="48" />
              <h3 class="text-xl font-semibold mb-4">SSH Client</h3>
              <p class="text-gray-600">Integrated SSH client for container access</p>
            </div>
          </div>
        </div>
      </div>

      <!-- CTA Section -->
      <div class="container mx-auto px-6 py-24">
        <div class="text-center">
          <h2 class="text-3xl font-bold text-white mb-8">Ready to get started?</h2>
          <BaseButton to="/login" color="info" label="Try Redock Now" />
        </div>
      </div>
    </div>
  </LayoutGuest>
</template>