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
    CardBox,
    CardBoxModal,
    SectionTitleLineWithButton,
    SectionMain,
    LayoutAuthenticated,
    DataTable,
    BaseButton,
    FormControl,
    FormField,
    BaseButtons
  },
  data() {
    return {
      cardClass: '',
      buttonSettingsModel: ref([]),
      mainStore: useMainStore(),
      virtualHosts: [],
      isEditModalActive: false,
      isAddModalActive: false,
      modalPath: '',
      virtualhostContent: '',
      isDeleteModalActive: false,
      datatableOptions: {
        columns: [
          { title: "Path" }
        ],
        lengthMenu: [15, 50, 100],
        pageLength: 15,
        ordering: false
      },
      createVirtualHost: {
        domain: '',
        service: '',
        configurationType: '',
        proxyPass: '',
        folder: '',
        phpService: '',
      },
      options: ['nginx', 'httpd'],
      configurationTypes: ['Default', 'Proxy Pass'],
      phpServices: []
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
    this.getAllVHosts()
    ApiService.getPhpServices().then(value => {
      this.phpServices = value.data.data
    })
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
    getAllVHosts() {
      this.virtualHosts = [];
      ApiService.getAllVHosts().then(value => {
        for (let i = 0; i < value.data.data.length; i++) {
          this.virtualHosts.push([value.data.data[i]])
        }
      })
    },
    editVirtualHost(data) {
      this.isEditModalActive = true
      this.modalPath = data[0]
      ApiService.getVHostContent(data[0]).then(value => {
        this.virtualhostContent = value.data.data
      })
    },
    deleteVirtualHost(data) {
      this.isDeleteModalActive = true
      this.modalPath = data[0]
    },
    editSubmit() {
      let content = this.virtualhostContent
      ApiService.setVHostContent(this.modalPath, content).then(() => {
        this.isEditModalActive = false
        this.getAllVHosts()
      })
    },
    deleteSubmit() {
      ApiService.deleteVHost(this.modalPath).then(() => {
        this.isDeleteModalActive = false
        this.getAllVHosts()
      })
    },
    addSubmit() {
      let data = this.createVirtualHost
      ApiService.addVHost(data).then(() => {
        this.isAddModalActive = false
        this.getAllVHosts()
      })
    }
  }
}

</script>

<template>
  <LayoutAuthenticated>
    <SectionMain>
      <SectionTitleLineWithButton :icon="mdiAccountMultiple()" title="Virtual Hosts">
        <BaseButton type="submit" label="Create" color="info" @click="isAddModalActive = true" />
      </SectionTitleLineWithButton>
      <CardBox>

        <CardBoxModal v-model="isAddModalActive" title="VHost" hide-buttons>
          <CardBox :class="cardClass" is-form @submit.prevent="addSubmit">
            <FormField label="Service" help="">
              <FormControl v-model="createVirtualHost.service" type="select" placeholder="Domain" :options="options" />
            </FormField>
            <FormField label="Domain" help="">
              <FormControl v-model="createVirtualHost.domain" type="input" placeholder="Domain" />
            </FormField>
            <FormField label="Configuration Type" help="">
              <FormControl v-model="createVirtualHost.configurationType" type="select" placeholder="" :options="configurationTypes" />
            </FormField>
            <FormField v-if="createVirtualHost.configurationType == 'Default'" label="Folder" help="">
              <FormControl v-model="createVirtualHost.folder" type="input" placeholder="" />
            </FormField>
            <FormField v-if="createVirtualHost.configurationType == 'Default'" label="PHP Service" help="">
              <FormControl v-model="createVirtualHost.phpService" type="select" placeholder="" :options="phpServices" />
            </FormField>
            <FormField v-if="createVirtualHost.configurationType == 'Proxy Pass'" label="Port" help="">
              <FormControl v-model="createVirtualHost.proxyPass" type="input" placeholder="" />
            </FormField>
            <template #footer>
              <BaseButtons>
                <BaseButton type="submit" color="info" label="Save" />
                <BaseButton color="danger" label="Cancel" @click="isAddModalActive = false" />
              </BaseButtons>
            </template>
          </CardBox>
        </CardBoxModal>

        <CardBoxModal v-model="isEditModalActive" title="VHost" hide-buttons>
          <CardBox :class="cardClass" is-form @submit.prevent="editSubmit">
            <FormField :label="modalPath" help="VHost Content">
              <FormControl v-model="virtualhostContent" type="textarea" placeholder="Env" />
            </FormField>
            <template #footer>
              <BaseButtons>
                <BaseButton type="submit" color="info" label="Save" />
                <BaseButton color="danger" label="Cancel" @click="isEditModalActive = false" />
              </BaseButtons>
            </template>
          </CardBox>
        </CardBoxModal>

        <CardBoxModal v-model="isDeleteModalActive" title="VHost" hide-buttons>
          <CardBox :class="cardClass" is-form @submit.prevent="deleteSubmit">
            <p>{{ modalPath }} will be deleted. Are you sure?</p>
            <template #footer>
              <BaseButtons>
                <BaseButton type="submit" color="info" label="Save" />
                <BaseButton color="danger" label="Cancel" @click="isDeleteModalActive = false" />
              </BaseButtons>
            </template>
          </CardBox>
        </CardBoxModal>


          <DataTable :options="datatableOptions" :data="virtualHosts">
            <thead>
              <tr>
                <th>Path</th>
                <th></th>
              </tr>
            </thead>
            <template #column-1="props">
              <BaseButton class="mr-2" label="Edit" :icon="mdiEdit()" color="whiteDark"
                @click="editVirtualHost(props.rowData)" rounded-full />
              <BaseButton label="Delete" :icon="mdiDelete()" color="whiteDark" @click="deleteVirtualHost(props.rowData)"
                rounded-full />
            </template>
          </DataTable>
        </CardBox>

    </SectionMain>
  </LayoutAuthenticated>
</template>
