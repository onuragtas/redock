<script>
import BaseButton from "@/components/BaseButton.vue";
import BaseButtons from "@/components/BaseButtons.vue";
import CardBox from "@/components/CardBox.vue";
import CardBoxModal from "@/components/CardBoxModal.vue";
import FormControl from "@/components/FormControl.vue";
import FormField from "@/components/FormField.vue";
import SectionMain from "@/components/SectionMain.vue";
import SectionTitleLineWithButton from "@/components/SectionTitleLineWithButton.vue";
import LayoutAuthenticated from "@/layouts/LayoutAuthenticated.vue";
import ApiService from "@/services/ApiService";
import { useMainStore } from "@/stores/main";
import { mdiAccountMultiple, mdiMinus, mdiPencil } from "@mdi/js";
import { DataTable } from "datatables.net-vue3";
import { ref } from "vue";

export default {
  components: {
    SectionTitleLineWithButton,
    SectionMain,
    LayoutAuthenticated,
    BaseButton,
    BaseButtons,
    FormControl,
    FormField,
    CardBox,
    DataTable,
    CardBoxModal
  },
  data() {
    return {
      cardClass: '',
      buttonSettingsModel: ref([]),
      mainStore: useMainStore(),
      login: false,
      email: '',
      username: '',
      password: '',
      isAddModalActive: false,
      isStartModalActive: false,
      isRegisterModalActive: false,
      proxies: [],
      datatableOptions: {
        columns: [
          { title: "ID", data: "id" },
          { title: "Domain", data: "domain" },
          { title: "Port", data: "port" },
          { title: "Keep Alive", data: "keep_alive" },
          { title: "CreatedAt", data: "CreatedAt" },
          { title: "UpdatedAt",   data: "UpdatedAt" },
          { title: "Action" }
        ],
        lengthMenu: [15, 50, 100],
        pageLength: 15,
        ordering: true
      },
      create: {
        domain: '',
        port: 80,
        keep_alive: 0
      },
      startDomain: {},
      start: {
        localIp: '127.0.0.1',
        destinationIp: '127.0.0.1',
        localPort: 80,
      }
    }
  },
  computed: {
    clientBarItems: function () {
      return this.mainStore.clients.slice(0, 4)
    },
    transactionBarItems: function () {
      return this.mainStore.history
    }
  },
  mounted() {
    this.checkLogin()
  },
  methods: {
    mdiAccountMultiple() {
      return mdiAccountMultiple
    },
    mdiEdit() {
      return mdiPencil
    },
    mdiDelete() {
      return mdiMinus
    },
    checkLogin() {
      ApiService.checkLogin().then(value => {
        this.login = value.data.data.login
        if (this.login) {
          this.tunnelList()
        }
      })
    },
    loginSubmit() {
      ApiService.tunnelLogin(this.username, this.password).then(value => {
        this.login = value.data.data.login
        if (this.login) {
          this.tunnelList()
        }
      })
    },

    registerSubmit() {
      ApiService.tunnelRegister(this.email, this.username, this.password).then(value => {
        this.login = value.data.data.login
      })
    },

    logoutSubmit() {
      ApiService.tunnelLogout().then(value => {
        this.login = null
        this.username = ''
        this.password = ''
        this.proxies = []
        this.isAddModalActive = false
        this.isStartModalActive = false
        this.isDeleteModalActive = false
        this.startDomain = {}
        this.start = {
          localIp: '',
          destinationIp: '',
          localPort: ''
        }
        this.create = {
          domain: '',
          port: 80,
          keep_alive: 0
        }
        this.tunnelList()
      })
    },
    tunnelList()  {
      ApiService.tunnelList().then(value => {
        this.proxies = value.data.data
      })
    },
    deleteModal(data) {
      ApiService.tunnelDelete(data).then(() => {
        this.isDeleteModalActive = false
        this.tunnelList()
      })
    },
    addSubmit() {
      ApiService.tunnelCreate(this.create).then(() => {
        this.isAddModalActive = false
        this.tunnelList()
      })
    },
    startModal(data) {
      this.startDomain = data
      this.isStartModalActive = true
    },
    stopModal(item) {
      let data = {
        DomainId: item.id,
        Domain: item.domain,
      }

      ApiService.tunnelStop(data).then(() => {
        setTimeout(() => {
          this.tunnelList()
        }, 2000)
      })
    },
    startSubmit() {
      let data = {
        DomainId: this.startDomain.id,
        Domain: this.startDomain.domain,
        LocalIp: this.start.localIp,
        DestinationIp: this.start.destinationIp,
        LocalPort: parseInt(this.start.localPort)
      }

      ApiService.tunnelStart(data).then(() => {
        this.isStartModalActive = false
        this.tunnelList()
      })
    }
  }
}

</script>

<template>
  <LayoutAuthenticated>
    <SectionMain v-if="login">
      <SectionTitleLineWithButton :icon="mdiAccountMultiple()" title="Tunnel Proxy">
        <BaseButtons>
          <BaseButton v-if="login" type="submit" label="Logout" color="info" @click="logoutSubmit" />
          <BaseButton v-if="login" type="submit" label="Refresh List" color="info" @click="tunnelList" />
          <BaseButton type="submit" label="Create" color="info" @click="isAddModalActive = true" />
          <!-- <BaseButton type="submit" label="Reload" :disabled="regenerateBtnActive != true" color="info" @click="regenerate()" /> -->
        </BaseButtons>
      </SectionTitleLineWithButton>
      <CardBox>

        <CardBoxModal v-model="isAddModalActive" title="Tunnel Domain" hide-buttons>
          <CardBox :class="cardClass" is-form @submit.prevent="addSubmit">
            <FormField label="Domain" help="">
              <FormControl v-model="create.domain" type="input" placeholder="Domain" />
            </FormField>
            <template #footer>
              <BaseButtons>
                <BaseButton type="submit" color="info" label="Save" />
                <BaseButton color="danger" label="Cancel" @click="isAddModalActive = false" />
              </BaseButtons>
            </template>
          </CardBox>
        </CardBoxModal>


        <CardBoxModal v-model="isStartModalActive" title="Start Tunnel" hide-buttons>
          <CardBox :class="cardClass" is-form @submit.prevent="startSubmit">
            <FormField label="Local IP" help="">
              <FormControl v-model="start.localIp" type="input" placeholder="Local IP" />
            </FormField>
            <FormField label="Destionation IP" help="">
              <FormControl v-model="start.destinationIp" type="input" placeholder="Destination IP" />
            </FormField>
            <FormField label="Local Port" help="">
              <FormControl v-model="start.localPort" type="input" placeholder="Local Port" />
            </FormField>
            <template #footer>
              <BaseButtons>
                <BaseButton type="submit" color="info" label="Start Tunnel" />
                <BaseButton color="danger" label="Cancel" @click="isAddModalActive = false" />
              </BaseButtons>
            </template>
          </CardBox>
        </CardBoxModal>

        <DataTable :options="datatableOptions" :data="proxies">
          <thead>
            <tr>
              <th>ID</th>
              <th>Domain</th>
              <th>Port</th>
              <th>KeepAlive</th>
              <th>CreatedAt</th>
              <th>UpdatedAt</th>
            </tr>
          </thead>
          <template #column-6="props">
            <BaseButton :label="props.rowData.started ? 'Stop': 'Start'" :icon="mdiDelete()" :color="props.rowData.started ? 'danger': 'success'" rounded-full @click="props.rowData.started ? stopModal(props.rowData) : startModal(props.rowData)" />
            <BaseButton label="Delete" :icon="mdiDelete()" color="whiteDark" rounded-full @click="deleteModal(props.rowData)" />
          </template>
        </DataTable>
      </CardBox>
    </SectionMain>

    <SectionMain v-if="!login && !isRegisterModalActive">
      <CardBox :class="cardClass" is-form @submit.prevent="loginSubmit">
        <FormField label="Login" help="Please enter your login">
          <FormControl v-model="username" name="login" />
        </FormField>

        <FormField label="Password" help="Please enter your password">
          <FormControl v-model="password" type="password" name="password" autocomplete="current-password" />
        </FormField>

        <template #footer>
          <BaseButtons>
            <BaseButton type="submit" color="info" label="Login" />
            <BaseButton color="info" outline label="Register" @click="isRegisterModalActive = true" />
          </BaseButtons>
        </template>
      </CardBox>
    </SectionMain>
    
    
    <SectionMain v-if="!login && isRegisterModalActive">
      <CardBox :class="cardClass" is-form @submit.prevent="registerSubmit">
        <FormField label="Register" help="Please enter your username">
          <FormControl v-model="username" name="login"  />
        </FormField>
        
        <FormField label="Email" help="Please enter your email">
          <FormControl v-model="email" name="email" />
        </FormField>

        <FormField label="Password" help="Please enter your password">
          <FormControl v-model="password" type="password" name="password" autocomplete="current-password" />
        </FormField>

        <template #footer>
          <BaseButtons>
            <BaseButton type="submit"  color="info" label="Register" />
            <BaseButton outline color="info" label="Login" @click="isRegisterModalActive = false" />
          </BaseButtons>
        </template>
      </CardBox>
    </SectionMain>
  </LayoutAuthenticated>
</template>
