<script setup>
import BaseButton from "@/components/BaseButton.vue";
import BaseIcon from "@/components/BaseIcon.vue";
import CardBox from "@/components/CardBox.vue";
import CardBoxModal from "@/components/CardBoxModal.vue";
import FormControl from "@/components/FormControl.vue";
import FormField from "@/components/FormField.vue";

import ApiService from "@/services/ApiService";
import {
    mdiAlert,
    mdiCheck,
    mdiChevronLeft, mdiChevronRight,
    mdiClose,
    mdiCog,
    mdiDelete,
    mdiDomain,
    mdiEthernet,
    mdiFileCode,
    mdiFileDocument,
    mdiFolder,
    mdiPencil,
    mdiPlus,
    mdiRefresh,
    mdiServer,
    mdiWeb
} from "@mdi/js";
import { computed, onMounted, ref } from "vue";

// Reactive state
const virtualHosts = ref([])
const phpServices = ref([])
const loading = ref(false)
const isAddModalActive = ref(false)
const isEditModalActive = ref(false)
const isDeleteModalActive = ref(false)
const modalPath = ref('')
const virtualhostContent = ref('')

// Pagination
const currentPage = ref(1)
const itemsPerPage = ref(6)

// Form data
const createVirtualHost = ref({
  domain: '',
  service: 'nginx',
  configurationType: 'Default',
  proxyPass: '',
  folder: '',
  phpService: '',
})

// Options
const serviceOptions = ['nginx', 'httpd']
const configurationTypes = ['Default', 'Proxy Pass']

// Computed
const vhostStats = computed(() => {
  const total = virtualHosts.value.length
  const nginx = virtualHosts.value.filter(vhost => vhost[0]?.includes('nginx')).length
  const apache = total - nginx
  
  return { total, nginx, apache }
})

const paginatedVirtualHosts = computed(() => {
  const start = (currentPage.value - 1) * itemsPerPage.value
  const end = start + itemsPerPage.value
  return virtualHosts.value.slice(start, end)
})

const totalPages = computed(() => {
  return Math.ceil(virtualHosts.value.length / itemsPerPage.value)
})

const paginationInfo = computed(() => {
  const start = (currentPage.value - 1) * itemsPerPage.value + 1
  const end = Math.min(start + itemsPerPage.value - 1, virtualHosts.value.length)
  return `${start}-${end} of ${virtualHosts.value.length}`
})

// Methods
const getAllVHosts = async () => {
  loading.value = true
  try {
    const response = await ApiService.getAllVHosts()
    virtualHosts.value = response.data.data.map(path => [path])
  } catch (error) {
    console.error('Failed to load virtual hosts:', error)
    // Mock data for demo
    virtualHosts.value = [
      ['/etc/nginx/sites-available/app.example.com'],
      ['/etc/nginx/sites-available/api.example.com'],
      ['/etc/apache2/sites-available/blog.example.com.conf']
    ]
  } finally {
    loading.value = false
  }
}

const getPhpServices = async () => {
  try {
    const response = await ApiService.getPhpServices()
    phpServices.value = response.data.data || []
  } catch (error) {
    console.error('Failed to load PHP services:', error)
    phpServices.value = ['php8.2-fpm', 'php8.1-fpm', 'php7.4-fpm']
  }
}

const editVirtualHost = async (data) => {
  modalPath.value = data[0]
  isEditModalActive.value = true
  
  try {
    const response = await ApiService.getVHostContent(data[0])
    virtualhostContent.value = response.data.data
  } catch (error) {
    console.error('Failed to load vhost content:', error)
    virtualhostContent.value = `# Virtual Host Configuration
# ${data[0]}

server {
    listen 80;
    server_name example.com;
    root /var/www/html;
    index index.php index.html;
    
    location / {
        try_files $uri $uri/ =404;
    }
    
    location ~ \\.php$ {
        include snippets/fastcgi-php.conf;
        fastcgi_pass unix:/var/run/php/php8.2-fpm.sock;
    }
}`
  }
}

const deleteVirtualHost = (data) => {
  modalPath.value = data[0]
  isDeleteModalActive.value = true
}

const editSubmit = async () => {
  try {
    await ApiService.setVHostContent(modalPath.value, virtualhostContent.value)
    isEditModalActive.value = false
    await getAllVHosts()
  } catch (error) {
    console.error('Failed to update vhost:', error)
  }
}

const deleteSubmit = async () => {
  try {
    await ApiService.deleteVHost(modalPath.value)
    isDeleteModalActive.value = false
    await getAllVHosts()
  } catch (error) {
    console.error('Failed to delete vhost:', error)
  }
}

const addSubmit = async () => {
  try {
    await ApiService.addVHost(createVirtualHost.value)
    isAddModalActive.value = false
    resetForm()
    await getAllVHosts()
  } catch (error) {
    console.error('Failed to create vhost:', error)
  }
}

const resetForm = () => {
  createVirtualHost.value = {
    domain: '',
    service: 'nginx',
    configurationType: 'Default',
    proxyPass: '',
    folder: '',
    phpService: '',
  }
}

const getServiceIcon = (path) => {
  if (path.includes('nginx')) return mdiServer
  if (path.includes('apache')) return mdiWeb
  return mdiCog
}

const getServiceColor = (path) => {
  if (path.includes('nginx')) return 'text-green-600'
  if (path.includes('apache')) return 'text-blue-600'
  return 'text-gray-600'
}

const extractDomain = (path) => {
  const parts = path.split('/')
  const filename = parts[parts.length - 1]
  return filename.replace('.conf', '').replace('sites-available/', '')
}

const nextPage = () => {
  if (currentPage.value < totalPages.value) {
    currentPage.value++
  }
}

const prevPage = () => {
  if (currentPage.value > 1) {
    currentPage.value--
  }
}

const goToPage = (page) => {
  if (page >= 1 && page <= totalPages.value) {
    currentPage.value = page
  }
}

// Lifecycle
onMounted(() => {
  getAllVHosts()
  getPhpServices()
})
</script>

<template>
  <div class="space-y-8">
      <!-- Header -->
      <div class="bg-gradient-to-r from-green-600 via-teal-600 to-blue-600 rounded-2xl p-8 text-white shadow-lg">
        <div class="flex flex-col lg:flex-row lg:items-center lg:justify-between">
          <div>
            <h1 class="text-3xl lg:text-4xl font-bold mb-2 flex items-center">
              <BaseIcon :path="mdiWeb" size="40" class="mr-4" />
              Virtual Hosts Manager
            </h1>
            <p class="text-green-100 text-lg">Web server configuration and domain management</p>
          </div>
          <div class="mt-6 lg:mt-0 flex space-x-3">
            <BaseButton
              label="Refresh"
              :icon="mdiRefresh"
              color="white"
              outline
              @click="getAllVHosts"
              :disabled="loading"
              class="shadow-lg hover:shadow-xl transform hover:scale-105 transition-all duration-200"
            />
            <BaseButton
              label="Create VHost"
              :icon="mdiPlus"
              color="white"
              @click="isAddModalActive = true"
              class="shadow-lg hover:shadow-xl transform hover:scale-105 transition-all duration-200"
            />
          </div>
        </div>
      </div>

      <!-- Statistics -->
      <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
        <CardBox class="bg-gradient-to-br from-green-50 to-green-100 dark:from-green-900/20 dark:to-green-800/20 border-green-200 dark:border-green-700">
          <div class="flex items-center justify-between">
            <div>
              <div class="text-2xl font-bold text-green-600 dark:text-green-400">{{ vhostStats.total }}</div>
              <div class="text-sm text-green-600/70 dark:text-green-400/70">Total VHosts</div>
            </div>
            <BaseIcon :path="mdiWeb" size="48" class="text-green-500 opacity-20" />
          </div>
        </CardBox>

        <CardBox class="bg-gradient-to-br from-blue-50 to-blue-100 dark:from-blue-900/20 dark:to-blue-800/20 border-blue-200 dark:border-blue-700">
          <div class="flex items-center justify-between">
            <div>
              <div class="text-2xl font-bold text-blue-600 dark:text-blue-400">{{ vhostStats.nginx }}</div>
              <div class="text-sm text-blue-600/70 dark:text-blue-400/70">Nginx Sites</div>
            </div>
            <BaseIcon :path="mdiServer" size="48" class="text-blue-500 opacity-20" />
          </div>
        </CardBox>

        <CardBox class="bg-gradient-to-br from-purple-50 to-purple-100 dark:from-purple-900/20 dark:to-purple-800/20 border-purple-200 dark:border-purple-700">
          <div class="flex items-center justify-between">
            <div>
              <div class="text-2xl font-bold text-purple-600 dark:text-purple-400">{{ vhostStats.apache }}</div>
              <div class="text-sm text-purple-600/70 dark:text-purple-400/70">Apache Sites</div>
            </div>
            <BaseIcon :path="mdiWeb" size="48" class="text-purple-500 opacity-20" />
          </div>
        </CardBox>
      </div>

      <!-- Virtual Hosts List -->
      <CardBox>
        <div class="bg-gradient-to-r from-slate-50 to-slate-100 dark:from-slate-800 dark:to-slate-700 p-6 -m-6 mb-6">
          <div class="flex items-center justify-between">
            <div>
              <h2 class="text-xl font-bold flex items-center">
                <BaseIcon :path="mdiFileDocument" size="24" class="mr-3 text-green-600 dark:text-green-400" />
                Virtual Host Configurations
              </h2>
              <p class="text-slate-600 dark:text-slate-400 mt-1">Manage web server virtual hosts</p>
            </div>
          </div>
        </div>

        <div v-if="loading" class="text-center py-12">
          <div class="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-green-600"></div>
          <p class="text-slate-500 dark:text-slate-400 mt-4">Loading virtual hosts...</p>
        </div>

        <div v-else-if="virtualHosts.length === 0" class="text-center py-12">
          <BaseIcon :path="mdiWeb" size="64" class="mx-auto text-slate-300 dark:text-slate-600 mb-4" />
          <p class="text-slate-500 dark:text-slate-400 mb-4">No virtual hosts configured</p>
          <BaseButton
            label="Create Your First VHost"
            :icon="mdiPlus"
            color="info"
            @click="isAddModalActive = true"
          />
        </div>

        <div v-else class="space-y-4">
          <div 
            v-for="vhost in paginatedVirtualHosts" 
            :key="vhost[0]"
            class="flex items-center justify-between p-6 bg-slate-50 dark:bg-slate-800/50 rounded-xl hover:bg-slate-100 dark:hover:bg-slate-700/50 transition-colors"
          >
            <div class="flex items-center space-x-6">
              <div class="flex-shrink-0">
                <div class="w-12 h-12 bg-gradient-to-br from-green-500 to-teal-600 rounded-xl flex items-center justify-center">
                  <BaseIcon 
                    :path="getServiceIcon(vhost[0])" 
                    size="24" 
                    class="text-white" 
                  />
                </div>
              </div>
              
              <div class="flex-1">
                <h3 class="font-semibold text-lg flex items-center">
                  <BaseIcon :path="mdiDomain" size="20" class="mr-2 text-blue-500" />
                  {{ extractDomain(vhost[0]) }}
                </h3>
                <div class="flex items-center space-x-4 mt-1 text-sm text-slate-500 dark:text-slate-400">
                  <div class="flex items-center">
                    <BaseIcon :path="mdiFolder" size="16" class="mr-1" />
                    {{ vhost[0] }}
                  </div>
                  <div 
                    class="flex items-center" 
                    :class="getServiceColor(vhost[0])"
                  >
                    <BaseIcon :path="getServiceIcon(vhost[0])" size="16" class="mr-1" />
                    {{ vhost[0].includes('nginx') ? 'Nginx' : vhost[0].includes('apache') ? 'Apache' : 'Other' }}
                  </div>
                </div>
              </div>
            </div>
            
            <div class="flex items-center space-x-2 ml-6">
              <BaseButton 
                :icon="mdiPencil" 
                color="info"
                size="small"
                @click="editVirtualHost(vhost)"
                title="Edit Configuration"
              />
              
              <BaseButton 
                :icon="mdiDelete" 
                color="danger"
                size="small"
                @click="deleteVirtualHost(vhost)"
                title="Delete VHost"
              />
            </div>
          </div>
        </div>

        <!-- Pagination -->
        <div v-if="totalPages > 1" class="flex items-center justify-between mt-6 pt-6 border-t border-slate-200 dark:border-slate-700">
          <div class="text-sm text-slate-700 dark:text-slate-300">
            Showing {{ paginationInfo }}
          </div>
          
          <div class="flex items-center space-x-2">
            <BaseButton
              :icon="mdiChevronLeft"
              color="lightDark"
              size="small"
              @click="prevPage"
              :disabled="currentPage === 1"
            />
            
            <div class="flex space-x-1">
              <button
                v-for="page in Math.min(totalPages, 5)"
                :key="page"
                @click="goToPage(page)"
                :class="[
                  'px-3 py-2 text-sm font-medium rounded-lg transition-colors',
                  currentPage === page
                    ? 'bg-blue-600 text-white'
                    : 'text-slate-700 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800'
                ]"
              >
                {{ page }}
              </button>
            </div>
            
            <BaseButton
              :icon="mdiChevronRight"
              color="lightDark"
              size="small"
              @click="nextPage"
              :disabled="currentPage === totalPages"
            />
          </div>
        </div>
      </CardBox>

      <!-- Add VHost Modal -->
      <CardBoxModal 
        v-model="isAddModalActive" 
        title="Create Virtual Host" 
        button="success" 
        buttonLabel="Create Virtual Host"
        has-cancel
        @confirm="addSubmit"
      >
        <form class="space-y-6">
          <FormField label="Web Server Service">
            <div class="relative">
              <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                <BaseIcon :path="mdiServer" size="20" class="text-slate-400" />
              </div>
              <FormControl
                v-model="createVirtualHost.service"
                type="select"
                :options="serviceOptions"
                required
                class="pl-10"
              />
            </div>
          </FormField>

          <FormField label="Domain Name" help="The domain name for this virtual host">
            <div class="relative">
              <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                <BaseIcon :path="mdiDomain" size="20" class="text-slate-400" />
              </div>
              <FormControl
                v-model="createVirtualHost.domain"
                placeholder="example.com"
                required
                class="pl-10"
              />
            </div>
          </FormField>

          <FormField label="Configuration Type">
            <div class="relative">
              <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                <BaseIcon :path="mdiCog" size="20" class="text-slate-400" />
              </div>
              <FormControl
                v-model="createVirtualHost.configurationType"
                type="select"
                :options="configurationTypes"
                required
                class="pl-10"
              />
            </div>
          </FormField>

          <!-- Default Configuration Fields -->
          <div v-if="createVirtualHost.configurationType === 'Default'" class="space-y-4">
            <FormField label="Document Root Folder" help="Path to the website files">
              <div class="relative">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                  <BaseIcon :path="mdiFolder" size="20" class="text-slate-400" />
                </div>
                <FormControl
                  v-model="createVirtualHost.folder"
                  placeholder="/var/www/html"
                  class="pl-10"
                />
              </div>
            </FormField>

            <FormField label="PHP Service" help="PHP-FPM service to use">
              <div class="relative">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                  <BaseIcon :path="mdiFileCode" size="20" class="text-slate-400" />
                </div>
                <FormControl
                  v-model="createVirtualHost.phpService"
                  type="select"
                  :options="phpServices"
                  class="pl-10"
                />
              </div>
            </FormField>
          </div>

          <!-- Proxy Pass Configuration -->
          <div v-if="createVirtualHost.configurationType === 'Proxy Pass'">
            <FormField label="Proxy Port" help="Port to proxy requests to">
              <div class="relative">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                  <BaseIcon :path="mdiEthernet" size="20" class="text-slate-400" />
                </div>
                <FormControl
                  v-model="createVirtualHost.proxyPass"
                  type="number"
                  placeholder="3000"
                  class="pl-10"
                />
              </div>
            </FormField>
          </div>
        </form>
      </CardBoxModal>

      <!-- Edit VHost Modal -->
      <CardBoxModal 
        v-model="isEditModalActive" 
        title="Edit Virtual Host" 
        button="success" 
        buttonLabel="Update Configuration"
        has-cancel
        @confirm="editSubmit"
      >
        <form class="space-y-6">
          <div class="bg-blue-50 dark:bg-blue-900/20 p-4 rounded-lg mb-6">
            <h4 class="font-semibold text-blue-800 dark:text-blue-200 mb-2 flex items-center">
              <BaseIcon :path="mdiFileDocument" size="20" class="mr-2" />
              {{ modalPath }}
            </h4>
            <p class="text-sm text-blue-600 dark:text-blue-300">
              Edit the virtual host configuration file directly.
            </p>
          </div>

          <FormField label="Configuration Content">
            <FormControl
              v-model="virtualhostContent"
              type="textarea"
              rows="15"
              placeholder="Virtual host configuration..."
              class="font-mono text-sm"
            />
          </FormField>
        </form>

        <template #footer>
          <div class="flex justify-end space-x-3">
            <BaseButton
              :icon="mdiClose"
              label="Cancel"
              color="lightDark"
              @click="isEditModalActive = false"
            />
            <BaseButton
              :icon="mdiCheck"
              label="Save Configuration"
              color="success"
              @click="editSubmit"
            />
          </div>
        </template>
      </CardBoxModal>

      <!-- Delete Confirmation Modal -->
      <CardBoxModal 
        v-model="isDeleteModalActive" 
        title="Delete Virtual Host" 
        button="danger"
        buttonLabel="Delete Virtual Host"
        has-cancel
        @confirm="deleteSubmit"
      >
        <div class="text-center">
          <BaseIcon :path="mdiAlert" size="48" class="mx-auto text-red-500 mb-4" />
          <h3 class="text-lg font-semibold mb-2">Are you sure?</h3>
          <p class="text-slate-600 dark:text-slate-400 mb-6">
            This will permanently delete the virtual host configuration:
          </p>
          <div class="bg-red-50 dark:bg-red-900/20 p-4 rounded-lg border border-red-200 dark:border-red-800">
            <code class="text-red-600 dark:text-red-400 break-all">{{ modalPath }}</code>
          </div>
        </div>

        <template #footer>
          <div class="flex justify-end space-x-3">
            <BaseButton
              :icon="mdiClose"
              label="Cancel"
              color="lightDark"
              @click="isDeleteModalActive = false"
            />
            <BaseButton
              :icon="mdiDelete"
              label="Delete VHost"
              color="danger"
              @click="deleteSubmit"
            />
          </div>
        </template>
      </CardBoxModal>
    </div>
</template>

<style scoped>
/* Loading spinner */
.animate-spin {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

/* Code textarea styling */
textarea.font-mono {
  font-family: 'SF Mono', 'Monaco', 'Inconsolata', 'Fira Code', 'Droid Sans Mono', 'Source Code Pro', monospace;
}
</style>