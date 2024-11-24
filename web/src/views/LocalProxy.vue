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
      isAddModalActive: false,
      isStartModalActive: false,
      list: [],
      datatableOptions: {
        columns: [
          { title: "Name", data: "name" },
          { title: "Local Port", data: "local_port" },
          { title: "Host", data: "host" },
          { title: "Remote Port", data: "remote_port" },
          { title: "Timeout", data: "timeout" },
          { title: "Action" }
        ],
        lengthMenu: [15, 50, 100],
        pageLength: 15,
        ordering: true
      },
      create: {
        name: '',
        local_port: '',
        host: '',
        remote_port: '',
        timeout: 30,
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
    this.getList()
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
    getList()  {
      ApiService.localProxyList().then(value => {
        this.list = value.data.data
      })
    },
    deleteModal(data) {
      ApiService.localProxyDelete(data).then(() => {
        this.getList()
      })
    },
    addSubmit() {
      let data = {
        name: this.create.name,
        local_port: parseInt(this.create.local_port),
        host: this.create.host,
        remote_port: parseInt(this.create.remote_port),
        timeout: parseInt(this.create.timeout)
      }
      ApiService.localProxyCreate(data).then(() => {
        this.isAddModalActive = false
        this.getList()
      })
    },
    startModal(item) {
      ApiService.localProxyStart(item).then(() => {
        this.getList()
      })
    },
    stopModal(item) {
      ApiService.localProxyStop(item).then(() => {
        this.getList()
      })
    },
    startAll() {
      ApiService.localProxyStartAll().then(() => {
        this.getList()
      })
    }
  }
}

</script>

<template>
  <LayoutAuthenticated>
    <SectionMain>
      <SectionTitleLineWithButton :icon="mdiAccountMultiple()" title="Local Proxy">
        <BaseButtons>
          <BaseButton type="submit" label="Create" color="info" @click="isAddModalActive = true" />
          <BaseButton type="submit" label="Start All" color="info" @click="startAll()" />
        </BaseButtons>
      </SectionTitleLineWithButton>
      <CardBox>

        <CardBoxModal v-model="isAddModalActive" title="Local Proxy" hide-buttons>
          <CardBox :class="cardClass" is-form @submit.prevent="addSubmit">
            <FormField label="Name" help="">
              <FormControl v-model="create.name" type="input" placeholder="Name" />
            </FormField>
            <FormField label="Local Port" help="">
              <FormControl v-model="create.local_port" type="input" placeholder="Local Port" />
            </FormField>
            <FormField label="Destionation IP/Domain" help="">
              <FormControl v-model="create.host" type="input" placeholder="Destionation IP/Domain" />
            </FormField>
            <FormField label="Remote Port" help="">
              <FormControl v-model="create.remote_port" type="input" placeholder="Remote Port" />
            </FormField>
            <FormField label="Timeout" help="">
              <FormControl v-model="create.timeout" type="input" placeholder="Timeout" />
            </FormField>
            <template #footer>
              <BaseButtons>
                <BaseButton type="submit" color="info" label="Save" />
                <BaseButton color="danger" label="Cancel" @click="isAddModalActive = false" />
              </BaseButtons>
            </template>
          </CardBox>
        </CardBoxModal>

        <DataTable :options="datatableOptions" :data="list">
          <template #column-5="props">
            <BaseButton :label="props.rowData.started ? 'Stop': 'Start'" :icon="mdiDelete()" :color="props.rowData.started ? 'danger': 'success'" rounded-full @click="props.rowData.started ? stopModal(props.rowData) : startModal(props.rowData)" />
            <BaseButton label="Delete" :icon="mdiDelete()" color="whiteDark" rounded-full @click="deleteModal(props.rowData)" />
          </template>
        </DataTable>
      </CardBox>
    </SectionMain>
  </LayoutAuthenticated>
</template>
