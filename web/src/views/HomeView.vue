<script>
import BaseButton from "@/components/BaseButton.vue";
import BaseLevel from "@/components/BaseLevel.vue";
import CardBox from "@/components/CardBox.vue";
import CardBoxModal from "@/components/CardBoxModal.vue";
import SectionMain from "@/components/SectionMain.vue";
import SectionTitleLineWithButton from "@/components/SectionTitleLineWithButton.vue";
import LayoutAuthenticated from "@/layouts/LayoutAuthenticated.vue";
import ApiService from "@/services/ApiService";
import { useMainStore } from "@/stores/main";
import {
  mdiAccountMultiple, mdiAutoDownload,
  mdiCartOutline,
  mdiChartPie,
  mdiChartTimelineVariant, mdiDownloadNetwork, mdiEye, mdiMinus,
  mdiMonitorCellphone, mdiPlus,
  mdiReload, mdiTrashCan, mdiUpdate
} from "@mdi/js";
import { ref } from "vue";

export default {
  components: {
    BaseLevel,
    CardBox,
    CardBoxModal,
    SectionTitleLineWithButton,
    SectionMain,
    LayoutAuthenticated,
    BaseButton,
  },
  data() {
    return {
      buttonSettingsModel: ref([]),
      mainStore: useMainStore(),
      chartData: ref(null),
      regenerateBtnActive: true,
      localIp: '',
      allServices: [],
      isModalActive: false,
      isModalDangerActive: false,
      btnState: {
        addXDebug: true,
        removeXDebug: true,
        restartNginx: true,
        selfUpdate: true,
        install: true,
        updateDocker: true,
        updateDockerImages: true
      }
    }
  },
  computed: {
    buttonsSmall: function () {
      this.buttonSettingsModel.value.indexOf('small') > -1
    },
    buttonsDisabled: function () {
      this.buttonSettingsModel.value.indexOf('disabled') > -1
    },
    buttonsRounded: function () {
      this.buttonSettingsModel.value.indexOf('rounded') > -1
    },
    buttonsOutline: function () {
      this.buttonSettingsModel.value.indexOf('outline') > -1
    },
    clientBarItems: function () {
      return this.mainStore.clients.slice(0, 4)
    },
    transactionBarItems: function () {
      return this.mainStore.history
    }
  },
  mounted() {
    this.getLocalIp()
    this.getAllServices()
    setInterval(this.getLocalIp, 10000)
  },
  methods: {
    mdiAutoDownload() {
      return mdiAutoDownload
    },
    mdiDownloadNetwork() {
      return mdiDownloadNetwork
    },
    mdiUpdate() {
      return mdiUpdate
    },
    mdiMinus() {
      return mdiMinus
    },
    mdiPlus() {
      return mdiPlus
    },
    mdiMonitorCellphone() {
      return mdiMonitorCellphone
    },
    mdiChartPie() {
      return mdiChartPie
    },
    mdiCartOutline() {
      return mdiCartOutline
    },
    mdiAccountMultiple() {
      return mdiAccountMultiple
    },
    mdiReload() {
      return mdiReload
    },
    mdiChartTimelineVariant() {
      return mdiChartTimelineVariant
    },
    mdiEye() {
      return mdiEye
    },
    mdiTrashCan() {
      return mdiTrashCan
    },
    regenerateXDebugConfiguration() {
      this.regenerateBtnActive = false
      ApiService.regenerateXDebugConfiguration().then(() => {
        this.regenerateBtnActive = true
      })
    },
    addXDebugConfiguration() {
      this.btnState.addXDebug = false
      ApiService.addXDebugConfiguration().then(() => {
        this.btnState.addXDebug = true
      })
    },
    removeXDebugConfiguration() {
      this.btnState.removeXDebug = false
      ApiService.removeXDebugConfiguration().then(() => {
        this.btnState.removeXDebug = true
      })
    },
    restartNginxHttpd() {
      this.btnState.restartNginx = false
      ApiService.restartNginxHttpd().then(() => {
        this.btnState.restartNginx = true
      })
    },
    selfUpdate() {
      this.btnState.selfUpdate = false
      ApiService.selfUpdate().then(() => {
        this.btnState.selfUpdate = true
      })
    },
    install() {
      this.btnState.install = false
      ApiService.install().then(() => {
        this.btnState.install = true
      })
    },
    updateDocker() {
      this.btnState.updateDocker = false
      ApiService.updateDocker().then(() => {
        this.btnState.updateDocker = true
      })
    },
    updateDockerImages() {
      this.btnState.updateDockerImages = false
      ApiService.updateDockerImages().then(() => {
        this.btnState.updateDockerImages = true
      })
    },
    getLocalIp() {
      ApiService.getLocalIp().then(value => {
        this.localIp = value.data.data.ip
      })
    },
    getAllServices() {
      ApiService.getAllServices().then(value => {
        this.allServices = value.data.data.all_services
        this.activeServices = value.data.data.active_services
      })
    },
    isChecked(isChecked, service) {
      service.disabled = true
      if(isChecked) {
        ApiService.addService(service.container_name).then(() => {
          this.getAllServices()
        })
      } else {
        ApiService.removeService(service.container_name).then(() => {
          this.getAllServices()
        })
      }
    }
  }
}

</script>

<template>
  <LayoutAuthenticated>
    <SectionMain>
      <SectionTitleLineWithButton :icon="mdiChartTimelineVariant()" title="Overview" main></SectionTitleLineWithButton>

      <div class="grid grid-cols-3 gap-6 lg:grid-cols-3 mb-6">
        <CardBox>
          <BaseLevel>
            <BaseLevel type="justify-start">
              <div class="text-center space-y-1 md:text-left md:mr-6">
                <h4 class="text-xl">XDebug Configuration</h4>
                <p class="text-gray-500 dark:text-slate-400">{{ localIp }}</p>
              </div>
            </BaseLevel>
            <div class="text-center md:text-right space-y-2">
              <p class="text-sm text-gray-500"></p>
              <div>
                <BaseButton label="Reload" :icon="mdiReload()" color="whiteDark"
                  rounded-full :disabled="regenerateBtnActive == false" @click="regenerateXDebugConfiguration" />
              </div>
            </div>
          </BaseLevel>
        </CardBox>

        <CardBox>
          <BaseLevel>
            <BaseLevel type="justify-start">
              <div class="text-center space-y-1 md:text-left md:mr-6">
                <h4 class="text-xl">XDebug Configuration</h4>
                <p class="text-gray-500 dark:text-slate-400"></p>
              </div>
            </BaseLevel>
            <div class="text-center md:text-right space-y-2">
              <p class="text-sm text-gray-500">
              </p>
              <div>
                <BaseButton label="Add" class="mb-1" :icon="mdiPlus()" rounded-full color="whiteDark" :disabled="btnState.addXDebug == false" @click="addXDebugConfiguration" />
                <BaseButton label="Remove" :icon="mdiMinus()" rounded-full color="whiteDark" :disabled="btnState.removeXDebug == false" @click="removeXDebugConfiguration" />
              </div>
            </div>
          </BaseLevel>
        </CardBox>

        <CardBox>
          <BaseLevel>
            <BaseLevel type="justify-start">
              <div class="text-center space-y-1 md:text-left md:mr-6">
                <h4 class="text-xl">Nginx/Apache2</h4>
                <p class="text-gray-500 dark:text-slate-400"></p>
              </div>
            </BaseLevel>
            <div class="text-center md:text-right space-y-2">
              <p class="text-sm text-gray-500">
              </p>
              <div>
                <BaseButton label="Restart" class="mb-1" :icon="mdiReload()" rounded-full color="whiteDark" :disabled="btnState.restartNginx == false" @click="restartNginxHttpd" />
              </div>
            </div>
          </BaseLevel>
        </CardBox>
      </div>

      <div class="grid grid-cols-1 gap-6 lg:grid-cols-1">
        <CardBox>
          <BaseLevel>
            <BaseLevel type="justify-start">
              <div class="text-center space-y-1 md:text-left md:mr-6">
                <h4 class="text-xl">Update</h4>
                <p class="text-gray-500 dark:text-slate-400"></p>
              </div>
            </BaseLevel>
            <div class="text-center md:text-right space-y-2">
              <p class="text-sm text-gray-500">
              </p>
              <div>
                <BaseButton label="Install" :icon="mdiMonitorCellphone()" color="info" class="mr-1" @click="install" />
                <BaseButton label="Attach SSH" :icon="mdiMonitorCellphone()" color="info" to="/exec" />
                <BaseButton label="Self Update" :icon="mdiUpdate()" color="whiteDark" :disabled="btnState.selfUpdate == false" rounded-full @click="selfUpdate" />
                <BaseButton label="Update Docker" :icon="mdiDownloadNetwork()" color="whiteDark" :disabled="btnState.updateDocker == false" rounded-full @click="updateDocker" />
                <BaseButton label="Update Docker Images" :icon="mdiAutoDownload()" color="whiteDark" :disabled="btnState.updateDockerImages == false" rounded-full @click="updateDockerImages" />
              </div>
            </div>
          </BaseLevel>
        </CardBox>
      </div>

      <SectionTitleLineWithButton :icon="mdiAccountMultiple()" title="Services" />
      <CardBox has-table>
        <CardBoxModal v-model="isModalActive" title="Sample modal">
          <p>Lorem ipsum dolor sit amet <b>adipiscing elit</b></p>
          <p>This is sample modal</p>
        </CardBoxModal>

        <CardBoxModal v-model="isModalDangerActive" title="Please confirm" button="danger" has-cancel>
          <p>Lorem ipsum dolor sit amet <b>adipiscing elit</b></p>
          <p>This is sample modal</p>
        </CardBoxModal>

        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="service in allServices" :key="service.container_name">
              <td data-label="Name">
                {{ service.container_name }}
              </td>
              <td>
                <BaseButton v-if="!service.active" :disabled="service.disabled" class="mr-1" label="Add" :icon="mdiMonitorCellphone()" color="success" @click="isChecked(true, service)" />
                <BaseButton v-if="service.active" :disabled="service.disabled" class="mr-1" label="Remove" :icon="mdiMonitorCellphone()" color="danger" @click="isChecked(false, service)" />
                <BaseButton v-if="service.active" class="mr-1" label="Attach" :icon="mdiMonitorCellphone()" color="info" :to="`/exec/${service.container_name}`" />
              </td>
            </tr>
          </tbody>
        </table>
      </CardBox>

    </SectionMain>
  </LayoutAuthenticated>
</template>
