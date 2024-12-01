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
      isConfigurationModalActive: false,
      isStartModalActive: false,
      settings: {},
      datatableOptions: {
        columns: [
          { title: "Name", data: "name" },
          { title: "Path", data: "path" },
          { title: "Url", data: "url" },
          { title: "Action" }
        ],
        lengthMenu: [15, 50, 100],
        pageLength: 15,
        ordering: true
      },
      create: {
        name: '',
        path: '',
        url: '',
      },
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
      ApiService.getXDebugAdapterSettings().then(value => {
        this.settings = value.data.data
      })
    },
    deleteModal(data) {
      ApiService.removeXDebugAdapterSettings(data).then(() => {
        this.getList()
      })
    },
    addSubmit() {
      ApiService.addXDebugAdapterSettings(this.create).then(() => {
        this.isAddModalActive = false
        this.getList()
      })
    },
    saveConfiguration() {
      ApiService.updateXDebugAdapterSettings(this.settings).then(() => {
        this.isConfigurationModalActive = false
        this.getList()
      })
    },
    start() {
      ApiService.startXDebugAdapter()
    },
    stop() {
      ApiService.stopXDebugAdapter()
    }
  }
}

</script>

<template>
  <LayoutAuthenticated>
    <SectionMain>
      <SectionTitleLineWithButton :icon="mdiAccountMultiple()" title="PHP XDebug Adapter">
        <BaseButtons>
          <BaseButton type="submit" label="Create" color="info" @click="isAddModalActive = true" />
          <BaseButton type="submit" label="Configuration" color="info" @click="isConfigurationModalActive = true" />
          <BaseButton type="submit" label="Start" color="info" @click="start()" />
          <BaseButton type="submit" label="Stop" color="info" @click="stop()" />
        </BaseButtons>
      </SectionTitleLineWithButton>
      <CardBox>

        <CardBoxModal v-model="isAddModalActive" title="PHP XDebug Adapter" hide-buttons>
          <CardBox :class="cardClass" is-form @submit.prevent="addSubmit">
            <FormField label="Name" help="">
              <FormControl v-model="create.name" type="input" placeholder="Name" />
            </FormField>
            <FormField label="Path" help="">
              <FormControl v-model="create.path" type="input" placeholder="/var/www/html/PROJECT" />
            </FormField>
            <FormField label="URL" help="">
              <FormControl v-model="create.url" type="input" placeholder="127.0.0.1:9981" />
            </FormField>
            <template #footer>
              <BaseButtons>
                <BaseButton type="submit" color="info" label="Save" />
                <BaseButton color="danger" label="Cancel" @click="isAddModalActive = false" />
              </BaseButtons>
            </template>
          </CardBox>
        </CardBoxModal>

        <CardBoxModal v-model="isConfigurationModalActive" title="PHP XDebug Adapter Configuration" hide-buttons>
          <CardBox :class="cardClass" is-form @submit.prevent="saveConfiguration">
            <FormField label="Name" help="">
              <FormControl v-model="settings.listen" type="input" placeholder="Local Bind Address" />
            </FormField>
            <template #footer>
              <BaseButtons>
                <BaseButton type="submit" color="info" label="Save" />
                <BaseButton color="danger" label="Cancel" @click="isConfigurationModalActive = false" />
              </BaseButtons>
            </template>
          </CardBox>
        </CardBoxModal>

        <DataTable :options="datatableOptions" :data="settings.mappings">
          <template #column-3="props">
            <BaseButton label="Delete" :icon="mdiDelete()" color="whiteDark" rounded-full @click="deleteModal(props.rowData)" />
          </template>
        </DataTable>
      </CardBox>
    </SectionMain>
  </LayoutAuthenticated>
</template>
