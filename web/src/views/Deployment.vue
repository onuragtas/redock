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
      mainStore: useMainStore(),
      isAddModalActive: false,
      isEditModalActive: false,
      isSettingsModalActive: false,
      credentials: {
        username: '',
        token: ''
      },
      list: [],
      datatableOptions: {
        columns: [
            { title: "Url", data: "url" },
              { title: "Path", data: "path" },
              { title: "Branch", data: "branch" },
              // { title: "Check", data: "check" },
              // { title: "Script", data: "script" },
              { title: "Last Deployed", data: "last_deployed" },
              { title: "Last Checked", data: "last_checked" },
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
        branch: '',
        check: '',
        script: ''
      },
      edit: {},
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
      ApiService.deploymentList().then(value => {
        this.list = value.data.data
      })
    },
    deleteModal(data) {
      ApiService.deploymentDelete({ path: data.path }).then(() => {
        this.getList()
      })
    },
    addSubmit() {
      ApiService.deploymentAdd(this.create).then(() => {
        this.isAddModalActive = false
        this.getList()
      })
    },
    editModal(item) {
      this.edit = { ...item }
      this.isEditModalActive = true
    },
    editSubmit() {
      ApiService.deploymentUpdate(this.edit).then(() => {
        this.isEditModalActive = false
        this.getList()
      })
    },
    async openSettingsModal() {
      // Ayarları backend'den çek
      const res = await ApiService.deploymentGetSettings();
      if (res.data && res.data.data) {
        this.credentials.username = res.data.data.username || '';
        this.credentials.token = res.data.data.token || '';
        this.credentials.checkTime = res.data.data.checkTime || 60;
      }
      this.isSettingsModalActive = true;
    },
    saveCredentials() {
      // checkTime'ı integer olarak gönder
      const data = {
        username: this.credentials.username,
        token: this.credentials.token,
        checkTime: parseInt(this.credentials.checkTime)
      };
      ApiService.deploymentSetCredentials(data).then(() => {
        this.isSettingsModalActive = false;
      });
    },
    formatDate(dateStr) {
      if (!dateStr) return "-";
      const date = new Date(dateStr);
      if (isNaN(date)) return dateStr;
      // Örnek: 2024-06-30 14:22:10
      return date.toLocaleString('tr-TR', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit'
      });
    },
  }
}
</script>

<template>
  <LayoutAuthenticated>
    <SectionMain>
      <SectionTitleLineWithButton :icon="mdiAccountMultiple()" title="Deployment">
        <BaseButtons>
          <BaseButton type="submit" label="Create" color="info" @click="isAddModalActive = true" />
          <BaseButton type="button" label="Settings" color="warning" @click="openSettingsModal" />
        </BaseButtons>
        <CardBoxModal v-model="isSettingsModalActive" title="Deployment Settings" hide-buttons>
          <CardBox :class="cardClass" is-form @submit.prevent="saveCredentials">
            <FormField label="Username" help="">
              <FormControl v-model="credentials.username" type="input" placeholder="Username" />
            </FormField>
            <FormField label="Token" help="">
              <FormControl v-model="credentials.token" type="input" placeholder="Token" />
            </FormField>
            <FormField label="Check Time (seconds)" help="">
              <FormControl v-model="credentials.checkTime" type="input" placeholder="60" />
            </FormField>
            <template #footer>
              <BaseButtons>
                <BaseButton type="submit" color="info" label="Save" />
                <BaseButton color="danger" label="Cancel" @click="isSettingsModalActive = false" />
              </BaseButtons>
            </template>
          </CardBox>
        </CardBoxModal>
      </SectionTitleLineWithButton>
      <CardBox>
        <CardBoxModal v-model="isAddModalActive" title="Deployment" hide-buttons>
          <CardBox :class="cardClass" is-form @submit.prevent="addSubmit">
            <FormField label="Name" help="">
              <FormControl v-model="create.name" type="input" placeholder="Name" />
            </FormField>
            <FormField label="Path" help="">
              <FormControl v-model="create.path" type="input" placeholder="/var/www/html/PROJECT" />
            </FormField>
            <FormField label="Url" help="">
              <FormControl v-model="create.url" type="input" placeholder="Git URL" />
            </FormField>
            <FormField label="Branch" help="">
              <FormControl v-model="create.branch" type="input" placeholder="Branch" />
            </FormField>
            <FormField
              label="Check"
              help="If you want the deployment to start, the command output must contain 'start_deployment'."
            >
              <FormControl type="textarea" v-model="create.check" placeholder="Check Command (optional)" rows="3" />
            </FormField>
            <FormField label="Script" help="">
              <FormControl type="textarea" v-model="create.script" placeholder="Deploy Script" rows="5" />
            </FormField>
            <template #footer>
              <BaseButtons>
                <BaseButton type="submit" color="info" label="Save" />
                <BaseButton color="danger" label="Cancel" @click="isAddModalActive = false" />
              </BaseButtons>
            </template>
          </CardBox>
        </CardBoxModal>

        <CardBoxModal v-model="isEditModalActive" title="Edit Deployment" hide-buttons>
          <CardBox :class="cardClass" is-form @submit.prevent="editSubmit">
            <FormField label="Name" help="">
              <FormControl v-model="edit.name" type="input" placeholder="Name" />
            </FormField>
            <FormField label="Path" help="">
              <FormControl v-model="edit.path" type="input" placeholder="/var/www/html/PROJECT" />
            </FormField>
            <FormField label="Url" help="">
              <FormControl v-model="edit.url" type="input" placeholder="Git URL" />
            </FormField>
            <FormField label="Branch" help="">
              <FormControl v-model="edit.branch" type="input" placeholder="Branch" />
            </FormField>
            <FormField
              label="Check"
              help="If you want the deployment to start, the command output must contain 'start_deployment'."
            >
              <FormControl type="textarea" v-model="edit.check" placeholder="Check Command (optional)" rows="3" />
            </FormField>
            <FormField label="Script" help="">
              <FormControl type="textarea" v-model="edit.script" placeholder="Deploy Script" rows="5"></FormControl>
            </FormField>
            <template #footer>
              <BaseButtons>
                <BaseButton type="submit" color="info" label="Save" />
                <BaseButton color="danger" label="Cancel" @click="isEditModalActive = false" />
              </BaseButtons>
            </template>
          </CardBox>
        </CardBoxModal>

        <DataTable :options="datatableOptions" :data="list">
          <template #column-3="{ rowData }">
            {{ formatDate(rowData.last_deployed) }}
          </template>
          <template #column-4="{ rowData }">
            {{ formatDate(rowData.last_checked) }}
          </template>
          <template #column-5="props">
            <BaseButton label="Edit" :icon="mdiEdit()" color="info" rounded-full @click="editModal(props.rowData)" />
            <BaseButton label="Delete" :icon="mdiDelete()" color="danger" rounded-full @click="deleteModal(props.rowData)" />
          </template>
        </DataTable>
      </CardBox>
    </SectionMain>
  </LayoutAuthenticated>
</template>
