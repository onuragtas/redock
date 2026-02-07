<script setup>
import BaseButton from '@/components/BaseButton.vue'
import BaseIcon from '@/components/BaseIcon.vue'
import CardBox from '@/components/CardBox.vue'
import CardBoxModal from '@/components/CardBoxModal.vue'
import FormControl from '@/components/FormControl.vue'
import FormField from '@/components/FormField.vue'
import ApiService from '@/services/ApiService'
import {
  mdiAccountPlus,
  mdiDelete,
  mdiPencil,
  mdiRefresh,
  mdiShieldAccount,
  mdiAccount
} from '@mdi/js'
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useToast } from 'vue-toastification'

const toast = useToast()
const router = useRouter()
const users = ref([])
const menuOptions = ref([])
const loading = ref(false)
const isAddModalActive = ref(false)
const isEditModalActive = ref(false)
const isDeleteModalActive = ref(false)
const selectedUser = ref(null)

const formCreate = ref({
  email: '',
  password: '',
  user_role: 'user',
  allowed_menus: []
})

const formEdit = ref({
  id: null,
  user_role: 'user',
  user_status: 1,
  allowed_menus: []
})

const fetchUsers = async () => {
  loading.value = true
  try {
    const res = await ApiService.getUsers()
    users.value = res.data?.data || []
  } catch (e) {
    if (e.response?.status === 403) {
      router.push('/')
      return
    }
    toast.error(e.response?.data?.msg || 'Kullanıcılar yüklenemedi')
    users.value = []
  } finally {
    loading.value = false
  }
}

const fetchMenuOptions = async () => {
  try {
    const res = await ApiService.getMenuOptions()
    menuOptions.value = res.data?.data || []
  } catch {
    menuOptions.value = []
  }
}

const openAddModal = () => {
  formCreate.value = { email: '', password: '', user_role: 'user', allowed_menus: [] }
  isAddModalActive.value = true
}

const openEditModal = (user) => {
  selectedUser.value = user
  formEdit.value = {
    id: user.id,
    user_role: user.user_role,
    user_status: user.user_status,
    allowed_menus: user.allowed_menus ? [...user.allowed_menus] : []
  }
  isEditModalActive.value = true
}

const openDeleteModal = (user) => {
  selectedUser.value = user
  isDeleteModalActive.value = true
}

const createSubmit = async () => {
  if (!formCreate.value.email?.trim() || !formCreate.value.password?.trim()) {
    toast.error('E-posta ve şifre gerekli')
    return
  }
  if (formCreate.value.password.length < 6) {
    toast.error('Şifre en az 6 karakter olmalı')
    return
  }
  try {
    await ApiService.createUser({
      email: formCreate.value.email.trim(),
      password: formCreate.value.password,
      user_role: formCreate.value.user_role,
      allowed_menus: formCreate.value.allowed_menus
    })
    toast.success('Kullanıcı eklendi')
    isAddModalActive.value = false
    await fetchUsers()
  } catch (e) {
    toast.error(e.response?.data?.msg || 'Kullanıcı eklenemedi')
  }
}

const editSubmit = async () => {
  try {
    await ApiService.updateUser(formEdit.value.id, {
      user_role: formEdit.value.user_role,
      user_status: formEdit.value.user_status,
      allowed_menus: formEdit.value.allowed_menus
    })
    toast.success('Kullanıcı güncellendi')
    isEditModalActive.value = false
    await fetchUsers()
  } catch (e) {
    toast.error(e.response?.data?.msg || 'Güncellenemedi')
  }
}

const deleteSubmit = async () => {
  try {
    await ApiService.deleteUser(selectedUser.value.id)
    toast.success('Kullanıcı silindi')
    isDeleteModalActive.value = false
    await fetchUsers()
  } catch (e) {
    toast.error(e.response?.data?.msg || 'Silinemedi')
  }
}

const toggleMenuCreate = (path) => {
  const idx = formCreate.value.allowed_menus.indexOf(path)
  if (idx === -1) formCreate.value.allowed_menus.push(path)
  else formCreate.value.allowed_menus.splice(idx, 1)
}

const toggleMenuEdit = (path) => {
  const idx = formEdit.value.allowed_menus.indexOf(path)
  if (idx === -1) formEdit.value.allowed_menus.push(path)
  else formEdit.value.allowed_menus.splice(idx, 1)
}

const isMenuCheckedCreate = (path) => formCreate.value.allowed_menus.includes(path)
const isMenuCheckedEdit = (path) => formEdit.value.allowed_menus.includes(path)

const roleLabel = (role) => (role === 'admin' ? 'Admin' : 'Kullanıcı')
const statusLabel = (status) => (status === 1 ? 'Aktif' : 'Pasif')

onMounted(() => {
  fetchUsers()
  fetchMenuOptions()
})
</script>

<template>
  <div class="space-y-6">
    <div class="flex flex-wrap items-center justify-between gap-4">
      <h1 class="text-2xl font-bold text-white">Kullanıcılar</h1>
      <BaseButton
        label="Yeni Kullanıcı"
        :icon="mdiAccountPlus"
        color="info"
        @click="openAddModal"
      />
    </div>

    <CardBox>
      <div v-if="loading" class="p-8 text-center text-gray-400">Yükleniyor...</div>
      <div v-else-if="users.length === 0" class="p-8 text-center text-gray-400">
        Henüz kullanıcı yok. İlk giriş yapan hesap otomatik admin olur.
      </div>
      <div v-else class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr class="border-b border-gray-700">
              <th class="text-left py-3 px-4 text-gray-300">E-posta</th>
              <th class="text-left py-3 px-4 text-gray-300">Rol</th>
              <th class="text-left py-3 px-4 text-gray-300">Durum</th>
              <th class="text-left py-3 px-4 text-gray-300">Menüler</th>
              <th class="w-24 text-right py-3 px-4 text-gray-300">İşlem</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="u in users"
              :key="u.id"
              class="border-b border-gray-700/50 hover:bg-gray-800/50"
            >
              <td class="py-3 px-4 text-white">{{ u.email }}</td>
              <td class="py-3 px-4">
                <span
                  :class="[
                    'inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium',
                    u.user_role === 'admin'
                      ? 'bg-purple-500/20 text-purple-300'
                      : 'bg-gray-500/20 text-gray-300'
                  ]"
                >
                  <BaseIcon
                    :path="u.user_role === 'admin' ? mdiShieldAccount : mdiAccount"
                    size="14"
                  />
                  {{ roleLabel(u.user_role) }}
                </span>
              </td>
              <td class="py-3 px-4 text-gray-300">{{ statusLabel(u.user_status) }}</td>
              <td class="py-3 px-4 text-gray-400">
                {{ u.user_role === 'admin' ? 'Tümü' : (u.allowed_menus || []).length + ' menü' }}
              </td>
              <td class="py-3 px-4 text-right">
                <BaseButton
                  :icon="mdiPencil"
                  small
                  color="info"
                  title="Düzenle"
                  @click="openEditModal(u)"
                />
                <BaseButton
                  :icon="mdiDelete"
                  small
                  color="danger"
                  title="Sil"
                  @click="openDeleteModal(u)"
                />
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </CardBox>

    <!-- Add user modal -->
    <CardBoxModal
      v-model="isAddModalActive"
      title="Yeni Kullanıcı"
      button-label="Ekle"
      @confirm="createSubmit"
    >
      <FormField label="E-posta">
        <FormControl v-model="formCreate.email" type="email" placeholder="user@example.com" />
      </FormField>
      <FormField label="Şifre">
        <FormControl v-model="formCreate.password" type="password" placeholder="••••••••" />
      </FormField>
      <FormField label="Rol">
        <select
          v-model="formCreate.user_role"
          class="w-full rounded-lg border border-gray-600 bg-gray-700 text-white px-3 py-2"
        >
          <option value="user">Kullanıcı</option>
          <option value="admin">Admin</option>
        </select>
      </FormField>
      <FormField
        v-if="formCreate.user_role === 'user'"
        label="Görünecek menüler"
        help="Bu kullanıcının göreceği menü öğelerini seçin."
      >
        <div class="max-h-48 overflow-y-auto space-y-2 border border-gray-600 rounded-lg p-3 bg-gray-800/50">
          <label
            v-for="option in menuOptions"
            :key="option.path"
            class="flex items-center gap-2 cursor-pointer text-gray-300 hover:text-white"
          >
            <input
              type="checkbox"
              :checked="isMenuCheckedCreate(option.path)"
              @change="toggleMenuCreate(option.path)"
            />
            <span>{{ option.name }}</span>
            <span class="text-xs text-gray-500">({{ option.path }})</span>
          </label>
        </div>
      </FormField>
    </CardBoxModal>

    <!-- Edit user modal -->
    <CardBoxModal
      v-model="isEditModalActive"
      title="Kullanıcıyı Düzenle"
      button-label="Kaydet"
      @confirm="editSubmit"
    >
      <p v-if="selectedUser" class="text-sm text-gray-400 mb-4">
        {{ selectedUser.email }}
      </p>
      <FormField label="Rol">
        <select
          v-model="formEdit.user_role"
          class="w-full rounded-lg border border-gray-600 bg-gray-700 text-white px-3 py-2"
        >
          <option value="user">Kullanıcı</option>
          <option value="admin">Admin</option>
        </select>
      </FormField>
      <FormField label="Durum">
        <select
          v-model="formEdit.user_status"
          class="w-full rounded-lg border border-gray-600 bg-gray-700 text-white px-3 py-2"
        >
          <option :value="1">Aktif</option>
          <option :value="0">Pasif</option>
        </select>
      </FormField>
      <FormField
        v-if="formEdit.user_role === 'user'"
        label="Görünecek menüler"
      >
        <div class="max-h-48 overflow-y-auto space-y-2 border border-gray-600 rounded-lg p-3 bg-gray-800/50">
          <label
            v-for="option in menuOptions"
            :key="option.path"
            class="flex items-center gap-2 cursor-pointer text-gray-300 hover:text-white"
          >
            <input
              type="checkbox"
              :checked="isMenuCheckedEdit(option.path)"
              @change="toggleMenuEdit(option.path)"
            />
            <span>{{ option.name }}</span>
          </label>
        </div>
      </FormField>
    </CardBoxModal>

    <!-- Delete confirm -->
    <CardBoxModal
      v-model="isDeleteModalActive"
      title="Kullanıcıyı Sil"
      button-label="Sil"
      :has-cancel="true"
      @confirm="deleteSubmit"
    >
      <p class="text-gray-300">
        <strong>{{ selectedUser?.email }}</strong> kullanıcısını silmek istediğinize emin misiniz?
      </p>
    </CardBoxModal>
  </div>
</template>
