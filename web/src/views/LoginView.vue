<script setup>
import BaseIcon from '@/components/BaseIcon.vue'
import ApiService from '@/services/ApiService'
import {
    mdiAccount,
    mdiAccountPlus,
    mdiDocker,
    mdiEmail,
    mdiEye,
    mdiEyeOff,
    mdiLock,
    mdiLogin
} from '@mdi/js'
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useToast } from 'vue-toastification'

const router = useRouter()
const toast = useToast()

// Form state
const isRegisterMode = ref(false)
const showPassword = ref(false)
const loading = ref(false)

// Login form
const loginForm = ref({
  username: '',
  password: ''
})

// Register form
const registerForm = ref({
  email: '',
  username: '',
  password: '',
  confirmPassword: ''
})

// Methods
const togglePasswordVisibility = () => {
  showPassword.value = !showPassword.value
}

const toggleMode = () => {
  isRegisterMode.value = !isRegisterMode.value
  // Clear forms when switching
  loginForm.value = { username: '', password: '' }
  registerForm.value = { email: '', username: '', password: '', confirmPassword: '' }
}

const handleLogin = async () => {
  if (!loginForm.value.username.trim() || !loginForm.value.password.trim()) {
    toast.error('Please fill in all fields')
    return
  }

  loading.value = true
  
  try {
    const response = await ApiService.tunnelLogin(
      loginForm.value.username, 
      loginForm.value.password
    )
    
    if (response.data.data.message) {
      toast.error(response.data.data.message)
    }
    
    if (response.data.data.token) {
      toast.success('Login successful!')
      router.push('/')
    }
  } catch (error) {
    toast.error('Login failed. Please try again.')
  } finally {
    loading.value = false
  }
}

const handleRegister = async () => {
  const { email, username, password, confirmPassword } = registerForm.value
  
  if (!email.trim() || !username.trim() || !password.trim() || !confirmPassword.trim()) {
    toast.error('Please fill in all fields')
    return
  }

  if (password !== confirmPassword) {
    toast.error('Passwords do not match')
    return
  }

  if (password.length < 6) {
    toast.error('Password must be at least 6 characters long')
    return
  }

  loading.value = true
  
  try {
    const response = await ApiService.tunnelRegister(email, username, password)
    
    if (response.data.data.message) {
      toast.error(response.data.data.message)
    }
    
    if (response.data.data.token) {
      toast.success('Registration successful!')
      router.push('/dashboard')
    }
  } catch (error) {
    toast.error('Registration failed. Please try again.')
  } finally {
    loading.value = false
  }
}

// Check if already logged in
onMounted(async () => {
  try {
    const response = await ApiService.userInfo()
    if (response.data.data.id > 0) {
      router.push('/dashboard')
    }
  } catch (error) {
    // User not logged in, stay on login page
  }
})
</script>

<template>
  <div class="min-h-screen bg-gray-900 flex items-center justify-center p-4 relative overflow-hidden">
    <!-- Animated background -->
    <div class="absolute inset-0">
      <!-- Gradient background -->
      <div class="absolute inset-0 bg-gradient-to-br from-gray-900 via-blue-900 to-purple-900"></div>
      
      <!-- Floating particles -->
      <div class="absolute inset-0">
        <div class="absolute top-1/4 left-1/4 w-2 h-2 bg-blue-400 rounded-full animate-pulse"></div>
        <div class="absolute top-1/3 right-1/3 w-1 h-1 bg-purple-400 rounded-full animate-pulse delay-1000"></div>
        <div class="absolute bottom-1/4 left-1/3 w-3 h-3 bg-indigo-400 rounded-full animate-pulse delay-2000"></div>
        <div class="absolute top-1/2 right-1/4 w-2 h-2 bg-blue-300 rounded-full animate-pulse delay-3000"></div>
        <div class="absolute bottom-1/3 right-1/2 w-1 h-1 bg-purple-300 rounded-full animate-pulse delay-4000"></div>
      </div>
    </div>

    <!-- Main container -->
    <div class="relative z-10 w-full max-w-md">
      <!-- Logo section -->
      <div class="text-center mb-8">
        <div class="inline-flex items-center justify-center w-16 h-16 bg-blue-600 rounded-2xl mb-4 shadow-2xl shadow-blue-600/25">
          <BaseIcon :path="mdiDocker" size="32" class="text-white" />
        </div>
        <h1 class="text-3xl font-bold text-white mb-2">Redock</h1>
        <p class="text-gray-300">DevStation - Local Development Hub</p>
      </div>

      <!-- Auth card -->
      <div class="bg-gray-800/80 backdrop-blur-xl rounded-2xl shadow-2xl border border-gray-700 overflow-hidden">
        <!-- Header -->
        <div class="p-6 bg-gradient-to-r from-blue-600/20 to-purple-600/20 border-b border-gray-700">
          <h2 class="text-2xl font-bold text-white text-center">
            {{ isRegisterMode ? 'Create Account' : 'Welcome Back' }}
          </h2>
          <p class="text-gray-300 text-center mt-1">
            {{ isRegisterMode ? 'Join the Redock DevStation' : 'Sign in to your DevStation' }}
          </p>
        </div>

        <div class="p-6">
          <!-- Login Form -->
          <form v-if="!isRegisterMode" @submit.prevent="handleLogin" class="space-y-4">
            <!-- Username -->
            <div>
              <label class="block text-sm font-medium text-gray-300 mb-2">Username</label>
              <div class="relative">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
                  <BaseIcon :path="mdiAccount" size="20" class="text-gray-400" />
                </div>
                <input
                  v-model="loginForm.username"
                  type="text"
                  required
                  class="w-full pl-10 pr-4 py-3 bg-gray-700 border border-gray-600 rounded-lg text-white placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                  placeholder="Enter your username"
                />
              </div>
            </div>

            <!-- Password -->
            <div>
              <label class="block text-sm font-medium text-gray-300 mb-2">Password</label>
              <div class="relative">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
                  <BaseIcon :path="mdiLock" size="20" class="text-gray-400" />
                </div>
                <input
                  v-model="loginForm.password"
                  :type="showPassword ? 'text' : 'password'"
                  required
                  class="w-full pl-10 pr-12 py-3 bg-gray-700 border border-gray-600 rounded-lg text-white placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                  placeholder="Enter your password"
                />
                <button
                  type="button"
                  @click="togglePasswordVisibility"
                  class="absolute inset-y-0 right-0 pr-3 flex items-center"
                >
                  <BaseIcon 
                    :path="showPassword ? mdiEyeOff : mdiEye" 
                    size="20" 
                    class="text-gray-400 hover:text-gray-300" 
                  />
                </button>
              </div>
            </div>

            <!-- Login Button -->
            <button
              type="submit"
              :disabled="loading"
              class="w-full py-3 px-4 bg-blue-600 hover:bg-blue-700 disabled:bg-gray-600 disabled:cursor-not-allowed text-white font-medium rounded-lg transition-all duration-200 flex items-center justify-center space-x-2"
            >
              <BaseIcon v-if="!loading" :path="mdiLogin" size="20" />
              <div v-if="loading" class="w-5 h-5 border-2 border-white border-t-transparent rounded-full animate-spin"></div>
              <span>{{ loading ? 'Signing in...' : 'Sign In' }}</span>
            </button>
          </form>

          <!-- Register Form -->
          <form v-else @submit.prevent="handleRegister" class="space-y-4">
            <!-- Email -->
            <div>
              <label class="block text-sm font-medium text-gray-300 mb-2">Email</label>
              <div class="relative">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
                  <BaseIcon :path="mdiEmail" size="20" class="text-gray-400" />
                </div>
                <input
                  v-model="registerForm.email"
                  type="email"
                  required
                  class="w-full pl-10 pr-4 py-3 bg-gray-700 border border-gray-600 rounded-lg text-white placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                  placeholder="Enter your email"
                />
              </div>
            </div>

            <!-- Username -->
            <div>
              <label class="block text-sm font-medium text-gray-300 mb-2">Username</label>
              <div class="relative">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
                  <BaseIcon :path="mdiAccount" size="20" class="text-gray-400" />
                </div>
                <input
                  v-model="registerForm.username"
                  type="text"
                  required
                  class="w-full pl-10 pr-4 py-3 bg-gray-700 border border-gray-600 rounded-lg text-white placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                  placeholder="Choose a username"
                />
              </div>
            </div>

            <!-- Password -->
            <div>
              <label class="block text-sm font-medium text-gray-300 mb-2">Password</label>
              <div class="relative">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
                  <BaseIcon :path="mdiLock" size="20" class="text-gray-400" />
                </div>
                <input
                  v-model="registerForm.password"
                  :type="showPassword ? 'text' : 'password'"
                  required
                  class="w-full pl-10 pr-12 py-3 bg-gray-700 border border-gray-600 rounded-lg text-white placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                  placeholder="Create a password"
                />
                <button
                  type="button"
                  @click="togglePasswordVisibility"
                  class="absolute inset-y-0 right-0 pr-3 flex items-center"
                >
                  <BaseIcon 
                    :path="showPassword ? mdiEyeOff : mdiEye" 
                    size="20" 
                    class="text-gray-400 hover:text-gray-300" 
                  />
                </button>
              </div>
            </div>

            <!-- Confirm Password -->
            <div>
              <label class="block text-sm font-medium text-gray-300 mb-2">Confirm Password</label>
              <div class="relative">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
                  <BaseIcon :path="mdiLock" size="20" class="text-gray-400" />
                </div>
                <input
                  v-model="registerForm.confirmPassword"
                  type="password"
                  required
                  class="w-full pl-10 pr-4 py-3 bg-gray-700 border border-gray-600 rounded-lg text-white placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                  placeholder="Confirm your password"
                />
              </div>
            </div>

            <!-- Register Button -->
            <button
              type="submit"
              :disabled="loading"
              class="w-full py-3 px-4 bg-green-600 hover:bg-green-700 disabled:bg-gray-600 disabled:cursor-not-allowed text-white font-medium rounded-lg transition-all duration-200 flex items-center justify-center space-x-2"
            >
              <BaseIcon v-if="!loading" :path="mdiAccountPlus" size="20" />
              <div v-if="loading" class="w-5 h-5 border-2 border-white border-t-transparent rounded-full animate-spin"></div>
              <span>{{ loading ? 'Creating account...' : 'Create Account' }}</span>
            </button>
          </form>

          <!-- Toggle Mode -->
          <div class="mt-6 pt-6 border-t border-gray-700 text-center">
            <p class="text-gray-400 mb-3">
              {{ isRegisterMode ? 'Already have an account?' : "Don't have an account?" }}
            </p>
            <button
              @click="toggleMode"
              class="text-blue-400 hover:text-blue-300 font-medium transition-colors"
            >
              {{ isRegisterMode ? 'Sign in instead' : 'Create an account' }}
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* Custom animations */
@keyframes float {
  0%, 100% { transform: translateY(0px) rotate(0deg); }
  33% { transform: translateY(-10px) rotate(1deg); }
  66% { transform: translateY(5px) rotate(-1deg); }
}

.animate-float {
  animation: float 6s ease-in-out infinite;
}

/* Smooth transitions */
input:focus {
  transform: translateY(-1px);
}

button:hover:not(:disabled) {
  transform: translateY(-1px);
}

/* Loading spinner */
@keyframes spin {
  to { transform: rotate(360deg); }
}

.animate-spin {
  animation: spin 1s linear infinite;
}
</style>