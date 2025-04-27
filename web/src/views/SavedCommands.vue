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
      savedCommands: [],
      isEditModalActive: false,
      isAddModalActive: false,
      modalPath: {},
      savedcommandContent: '',
      isDeleteModalActive: false,
      datatableOptions: {
        columns: [
          { title: "Command", data: "command" },
          { title: "Action" }
        ],
        lengthMenu: [15, 50, 100],
        pageLength: 15,
        ordering: false
      },
      createSavedCommand: {
        command: '',
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
    this.getAllSavedCommands()
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
    getAllSavedCommands() {
      this.savedCommands = [];
      ApiService.getAllSavedCommands().then(value => {
        this.savedCommands = value.data.data
      })
    },
    deleteSavedCommand(data) {
      this.isDeleteModalActive = true
      this.modalPath = data
      
    },
    deleteSubmit() {
      ApiService.deleteSavedCommand({"command": this.modalPath.command}).then(() => {
        this.isDeleteModalActive = false
        this.getAllSavedCommands()
      })
    },
    addSubmit() {
      let data = this.createSavedCommand
      console.log(data)
      ApiService.addSavedCommand(data).then(() => {
        this.isAddModalActive = false
        this.getAllSavedCommands()
      })
    }
  }
}

</script>

<template>
  <LayoutAuthenticated>
    <SectionMain>
      <SectionTitleLineWithButton :icon="mdiAccountMultiple()" title="Saved Commands">
        <BaseButton type="submit" label="Create" color="info" @click="isAddModalActive = true" />
      </SectionTitleLineWithButton>
      <CardBox>

        <CardBoxModal v-model="isAddModalActive" title="Command" hide-buttons>
          <CardBox :class="cardClass" is-form @submit.prevent="addSubmit">
            <FormField label="Command" help="">
              <FormControl v-model="createSavedCommand.command" type="input" placeholder="Command" />
            </FormField>
            <template #footer>
              <BaseButtons>
                <BaseButton type="submit" color="info" label="Save" />
                <BaseButton color="danger" label="Cancel" @click="isAddModalActive = false" />
              </BaseButtons>
            </template>
          </CardBox>
        </CardBoxModal>

        <CardBoxModal v-model="isDeleteModalActive" title="SavedCommand" hide-buttons>
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
        
          <DataTable :options="datatableOptions" :data="savedCommands">
            <thead>
              <tr>
                <th>Command</th>
                <th></th>
              </tr>
            </thead>
            <template #column-1="props">
              <BaseButton label="Delete" :icon="mdiDelete()" color="whiteDark" @click="deleteSavedCommand(props.rowData)"
                rounded-full />
            </template>
          </DataTable>
        </CardBox>

    </SectionMain>
  </LayoutAuthenticated>
</template>
