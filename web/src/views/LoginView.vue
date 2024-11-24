<script>
import { reactive } from 'vue'
import { useRouter } from 'vue-router'
import SectionFullScreen from '@/components/SectionFullScreen.vue'
import CardBox from '@/components/CardBox.vue'
import FormCheckRadio from '@/components/FormCheckRadio.vue'
import FormField from '@/components/FormField.vue'
import FormControl from '@/components/FormControl.vue'
import BaseButton from '@/components/BaseButton.vue'
import BaseButtons from '@/components/BaseButtons.vue'
import LayoutGuest from '@/layouts/LayoutGuest.vue'
import ApiService from "@/services/ApiService";

export default {
  components: {
    SectionFullScreen,
    CardBox,
    FormCheckRadio,
    FormField,
    LayoutGuest,
    FormControl,
    BaseButtons,
    BaseButton,
  },
  data() {
    return{
      router: useRouter(),
      login: 'admin',
      pass: 'admin',
      remember: true
    }
  },
  methods: {
    async submit() {
      let response;
      response = await ApiService.login(this.login, this.pass)
      console.log(response)
      this.router.push('/dashboard')
    }
  }

}

</script>

<template>
    <LayoutGuest>
      <SectionFullScreen v-slot="{ cardClass }" bg="purplePink">
        <CardBox :class="cardClass" is-form @submit.prevent="submit">
          <FormField label="Login" help="Please enter your login">
            <FormControl
              v-model="login"
              name="login"
              autocomplete="username"
            />
          </FormField>

          <FormField label="Password" help="Please enter your password">
            <FormControl
              v-model="pass"
              type="password"
              name="password"
              autocomplete="current-password"
            />
          </FormField>

          <FormCheckRadio
            v-model="remember"
            name="remember"
            label="Remember"
            :input-value="true"
          />

          <template #footer>
            <BaseButtons>
              <BaseButton type="submit" color="info" label="Login" />
              <BaseButton to="/dashboard" color="info" outline label="Back" />
            </BaseButtons>
          </template>
        </CardBox>
      </SectionFullScreen>
    </LayoutGuest>
</template>
