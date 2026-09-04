<script setup>
import { reactive } from 'vue'
import { useI18n } from 'vue-i18n'
import { useMainStore } from '@/stores/main'
import { mdiAccount, mdiMail, mdiAsterisk, mdiFormTextboxPassword, mdiGithub } from '@mdi/js'
import SectionMain from '@/components/SectionMain.vue'
import CardBox from '@/components/CardBox.vue'
import BaseDivider from '@/components/BaseDivider.vue'
import FormField from '@/components/FormField.vue'
import FormControl from '@/components/FormControl.vue'
import FormFilePicker from '@/components/FormFilePicker.vue'
import BaseButton from '@/components/BaseButton.vue'
import BaseButtons from '@/components/BaseButtons.vue'
import UserCard from '@/components/UserCard.vue'
import SectionTitleLineWithButton from '@/components/SectionTitleLineWithButton.vue'

const { t } = useI18n()
const mainStore = useMainStore()

const profileForm = reactive({
  name: mainStore.userName,
  email: mainStore.userEmail
})

const passwordForm = reactive({
  password_current: '',
  password: '',
  password_confirmation: ''
})

const submitProfile = () => {
  mainStore.setUser(profileForm)
}

const submitPass = () => {
  //
}
</script>

<template>
  <SectionMain>
    <SectionTitleLineWithButton :icon="mdiAccount" :title="t('prof.title')" main>
      <BaseButton
        href="https://github.com/justboil/admin-one-vue-tailwind"
        target="_blank"
        :icon="mdiGithub"
        :label="t('prof.starGithub')"
        color="contrast"
        rounded-full
        small
      />
    </SectionTitleLineWithButton>

    <UserCard class="mb-6" />

    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
      <CardBox is-form @submit.prevent="submitProfile">
        <FormField :label="t('prof.avatar')" :help="t('prof.avatarHelp')">
          <FormFilePicker :label="t('prof.upload')" />
        </FormField>

        <FormField :label="t('prof.name')" :help="t('prof.nameHelp')">
          <FormControl
            v-model="profileForm.name"
            :icon="mdiAccount"
            name="username"
            required
            autocomplete="username"
          />
        </FormField>
        <FormField :label="t('prof.email')" :help="t('prof.emailHelp')">
          <FormControl
            v-model="profileForm.email"
            :icon="mdiMail"
            type="email"
            name="email"
            required
            autocomplete="email"
          />
        </FormField>

        <template #footer>
          <BaseButtons>
            <BaseButton color="info" type="submit" :label="t('prof.submit')" />
            <BaseButton color="info" :label="t('prof.options')" outline />
          </BaseButtons>
        </template>
      </CardBox>

      <CardBox is-form @submit.prevent="submitPass">
        <FormField :label="t('prof.currentPassword')" :help="t('prof.currentPasswordHelp')">
          <FormControl
            v-model="passwordForm.password_current"
            :icon="mdiAsterisk"
            name="password_current"
            type="password"
            required
            autocomplete="current-password"
          />
        </FormField>

        <BaseDivider />

        <FormField :label="t('prof.newPassword')" :help="t('prof.newPasswordHelp')">
          <FormControl
            v-model="passwordForm.password"
            :icon="mdiFormTextboxPassword"
            name="password"
            type="password"
            required
            autocomplete="new-password"
          />
        </FormField>

        <FormField :label="t('prof.confirmPassword')" :help="t('prof.confirmPasswordHelp')">
          <FormControl
            v-model="passwordForm.password_confirmation"
            :icon="mdiFormTextboxPassword"
            name="password_confirmation"
            type="password"
            required
            autocomplete="new-password"
          />
        </FormField>

        <template #footer>
          <BaseButtons>
            <BaseButton type="submit" color="info" :label="t('prof.submit')" />
            <BaseButton color="info" :label="t('prof.options')" outline />
          </BaseButtons>
        </template>
      </CardBox>
    </div>
  </SectionMain>
</template>
