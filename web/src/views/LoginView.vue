<script>
import BaseButton from '@/components/BaseButton.vue'
import BaseButtons from '@/components/BaseButtons.vue'
import CardBox from '@/components/CardBox.vue'
import FormCheckRadio from '@/components/FormCheckRadio.vue'
import FormControl from '@/components/FormControl.vue'
import FormField from '@/components/FormField.vue'
import SectionFullScreen from '@/components/SectionFullScreen.vue'
import LayoutGuest from '@/layouts/LayoutGuest.vue'
import ApiService from "@/services/ApiService"
import { useRouter } from 'vue-router'
import { useToast } from 'vue-toastification'

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
      toast: useToast(),
      login: '',
      pass: '',
      remember: true,
      isRegister: false,
      registerLogin: '',
      registerEmail: '',
      registerPass: '',
      registerPassRepeat: ''
    }
  },
  methods: {
    async submit() {
      let response;
      response = await ApiService.tunnelLogin(this.login, this.pass)
      if (response.data.data.message != "") {
        this.toast.error(response.data.data.message, {
          timeout: 2000
        });
      }
      if (response.data.data.token != "") {
        this.router.push('/dashboard')
      }
    },
    async submitRegister() {
      if (this.registerPass !== this.registerPassRepeat) {
        this.toast.error("Passwords do not match!", { timeout: 2000 });
        return;
      }
      let response;
      response = await ApiService.tunnelRegister(this.registerEmail, this.registerLogin, this.registerPass)
      if (response.data.data.message != "") {
        this.toast.error(response.data.data.message, {
          timeout: 2000
        });
      }
      if (response.data.data.token != "") {
        this.toast.success("Registration successful!", { timeout: 2000 });
        this.isRegister = false;
        this.login = this.registerLogin;
        this.pass = this.registerPass;
        this.router.push('/dashboard')
      }
    },
    toggleForm() {
      this.isRegister = !this.isRegister;
    }
  },
  mounted() {
    ApiService.userInfo().then(value => {
      if (value.data.data.id > 0) {
        this.router.push('/dashboard')
      }
    })
  }
}
</script>

<template>
  <LayoutGuest>
    <SectionFullScreen v-slot="{ cardClass }" bg="purplePink">
      <CardBox :class="cardClass" is-form @submit.prevent="isRegister ? submitRegister() : submit()">
        <template v-if="!isRegister">
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
        </template>
        <template v-else>
          <FormField label="Login" help="Please enter your login">
            <FormControl
              v-model="registerLogin"
              name="registerLogin"
              autocomplete="username"
            />
          </FormField>
          <FormField label="Email" help="Please enter your email">
            <FormControl
              v-model="registerEmail"
              name="registerEmail"
              autocomplete="email"
            />
          </FormField>
          <FormField label="Password" help="Please enter your password">
            <FormControl
              v-model="registerPass"
              type="password"
              name="registerPass"
              autocomplete="new-password"
            />
          </FormField>
          <FormField label="Repeat Password" help="Please repeat your password">
            <FormControl
              v-model="registerPassRepeat"
              type="password"
              name="registerPassRepeat"
              autocomplete="new-password"
            />
          </FormField>
        </template>
        <template #footer>
          <BaseButtons>
            <BaseButton
              type="submit"
              color="info"
              :label="isRegister ? 'Register' : 'Login'"
            />
            <BaseButton
              type="button"
              color="info"
              outline
              :label="isRegister ? 'Back to Login' : 'Register'"
              @click="toggleForm"
            />
          </BaseButtons>
        </template>
      </CardBox>
    </SectionFullScreen>
  </LayoutGuest>
</template>
