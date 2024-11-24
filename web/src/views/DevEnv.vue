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
import { mdiAccountMultiple, mdiMinus, mdiMonitorCellphone, mdiPencil } from "@mdi/js";
import { DataTable } from "datatables.net-vue3";
import { ref } from "vue";
import { useRouter } from 'vue-router';

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
      router: useRouter(),
      cardClass: '',
      buttonSettingsModel: ref([]),
      mainStore: useMainStore(),
      personalContainers: [],
      isEditModalActive: false,
      isAddModalActive: false,
      modalPath: {},
      virtualhostContent: '',
      isDeleteModalActive: false,
      regenerateBtnActive: true,
      datatableOptions: {
        columns: [
          { title: "Path" }
        ],
        lengthMenu: [15, 50, 100],
        pageLength: 15,
        ordering: false
      },
      create: {
        username: '',
        password: '',
        port: 0,
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
    this.getPersonalContainers()
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
    mdiMonitorCellphone() {
      return mdiMonitorCellphone
    },
    getPersonalContainers() {
      this.personalContainers = [];
      ApiService.getPersonalContainers().then(value => {
        for (let i = 0; i < value.data.data.length; i++) {
          this.personalContainers.push([value.data.data[i].username, value.data.data[i].password, value.data.data[i].port])
        }
      })
    },
    editModal(data) {
      this.isEditModalActive = true
      this.modalPath = {
        username: data[0],
        password: data[1],
        port: data[2].toString()
      }
    },
    exec(data) {
      this.router.push('/exec/' + data[0])
    },
    deleteModal(data) {
      this.isDeleteModalActive = true
      this.modalPath = {
        username: data[0],
        password: data[1],
        port: data[2].toString()
      }
    },
    editSubmit() {
      let model = this.modalPath
      ApiService.editPersonalContainer(model).then(() => {
        this.isEditModalActive = false
        this.getPersonalContainers()
      })
    },
    deleteSubmit() {
      ApiService.deletePersonalContainer(this.modalPath).then(() => {
        this.isDeleteModalActive = false
        this.getPersonalContainers()
      })
    },
    addSubmit() {
      let data = this.create
      ApiService.addPersonalContainer(data).then(() => {
        this.isAddModalActive = false
        this.getPersonalContainers()
      })
    },
    regenerate() {
      this.regenerateBtnActive = false
      ApiService.regeneratePersonalContainer().then(() => {
        this.regenerateBtnActive = true
        this.getPersonalContainers()
      })
    }
  }
}

</script>

<template>
  <LayoutAuthenticated>
    <SectionMain>
      <SectionTitleLineWithButton :icon="mdiAccountMultiple()" title="Personal Containers">
        <BaseButtons>
          <BaseButton type="submit" label="Create" color="info" @click="isAddModalActive = true" />
          <BaseButton type="submit" label="Reload" :disabled="regenerateBtnActive != true" color="info"
            @click="regenerate()" />
        </BaseButtons>
      </SectionTitleLineWithButton>
      <CardBox>

        <CardBoxModal v-model="isAddModalActive" title="Personal Container" hide-buttons>
          <CardBox :class="cardClass" is-form @submit.prevent="addSubmit">
            <FormField label="Username" help="">
              <FormControl v-model="create.username" type="input" placeholder="Username" />
            </FormField>
            <FormField label="Password" help="">
              <FormControl v-model="create.password" type="password" placeholder="Password" />
            </FormField>
            <FormField label="Port" help="">
              <FormControl v-model="create.port" type="input" placeholder="Port" />
            </FormField>
            <template #footer>
              <BaseButtons>
                <BaseButton type="submit" color="info" label="Save" />
                <BaseButton color="danger" label="Cancel" @click="isAddModalActive = false" />
              </BaseButtons>
            </template>
          </CardBox>
        </CardBoxModal>

        <CardBoxModal v-model="isEditModalActive" title="Edit Personal Container" hide-buttons>
          <CardBox :class="cardClass" is-form @submit.prevent="editSubmit">
            <FormField label="Username" help="">
              <FormControl v-model="modalPath.username" disabled="" type="input" placeholder="Username" />
            </FormField>
            <FormField label="Password" help="">
              <FormControl v-model="modalPath.password" type="password" placeholder="Password" />
            </FormField>
            <FormField label="Port" help="">
              <FormControl v-model="modalPath.port" type="input" placeholder="Port" />
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
            <p>{{ modalPath.username }} will be deleted. Are you sure?</p>
            <template #footer>
              <BaseButtons>
                <BaseButton type="submit" color="info" label="Save" />
                <BaseButton color="danger" label="Cancel" @click="isDeleteModalActive = false" />
              </BaseButtons>
            </template>
          </CardBox>
        </CardBoxModal>


        <DataTable :options="datatableOptions" :data="personalContainers">
          <thead>
            <tr>
              <th>Username</th>
              <th>Password</th>
              <th>Port</th>
            </tr>
          </thead>
          <template #column-3="props">
            <BaseButton class="mr-1" label="Attach" :icon="mdiMonitorCellphone()" color="info" @click="exec(props.rowData)" />
            <BaseButton class="mr-2" label="Edit" :icon="mdiEdit()" color="whiteDark" @click="editModal(props.rowData)"
              rounded-full />
            <BaseButton label="Delete" :icon="mdiDelete()" color="whiteDark" @click="deleteModal(props.rowData)"
              rounded-full />
          </template>
        </DataTable>
      </CardBox>

    </SectionMain>
  </LayoutAuthenticated>
</template>
