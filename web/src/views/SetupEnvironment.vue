<script>

import BaseButton from "@/components/BaseButton.vue";
import BaseButtons from "@/components/BaseButtons.vue";
import CardBox from "@/components/CardBox.vue";
import FormControl from "@/components/FormControl.vue";
import FormField from "@/components/FormField.vue";
import SectionMain from "@/components/SectionMain.vue";
import LayoutAuthenticated from "@/layouts/LayoutAuthenticated.vue";
import ApiService from "@/services/ApiService";

export default {
  components: {
    SectionMain,
    CardBox,
    FormField,
    FormControl,
    BaseButton,
    BaseButtons,
    LayoutAuthenticated
  },
  data() {
    return {
      env: '',
      cardClass: ''
    }
  },
  mounted() {
    this.getEnv()
  },

  methods: {
    getEnv() {
      ApiService.getEnv().then(res => {
        this.env = res.data.data.env;
      })
    },
    submit() {
      let env = this.env
      ApiService.setEnv(env).then(res => {
        console.log(res)
        this.env = res.data.data.env
      })
    }
  }

}

</script>

<template>
  <LayoutAuthenticated>
    <SectionMain>
      <CardBox :class="cardClass" is-form @submit.prevent="submit">
        <FormField label="Env" help="Your env.">
          <FormControl height="auto" type="textarea" placeholder="Env" v-model="env" />
        </FormField>

        <template #footer>
          <BaseButtons>
            <BaseButton type="submit" color="info" label="Save" />
            <BaseButton to="/dashboard" color="info" outline label="Back" />
          </BaseButtons>
        </template>
      </CardBox>
    </SectionMain>

  </LayoutAuthenticated>
</template>
