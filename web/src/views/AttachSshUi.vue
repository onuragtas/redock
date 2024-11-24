<template>
  <LayoutAuthenticated>
      <SectionTitleLineWithButton :icon="mdiChartTimelineVariant()" title="Attach Shell" main></SectionTitleLineWithButton>
      <iframe title="ssh_ui" :src="getSshUiUrl()" style="position:fixed; top:0; left:0; bottom:0; right:0; width:100%; height:100%; border:none; margin:0; padding:0; overflow:hidden; z-index:999999;"></iframe>
  </LayoutAuthenticated>
</template>

<script>
import SectionTitleLineWithButton from '@/components/SectionTitleLineWithButton.vue';
import LayoutAuthenticated from '@/layouts/LayoutAuthenticated.vue';
import { mdiChartTimelineVariant } from '@mdi/js';

export default {
  components: {
    LayoutAuthenticated,
    SectionTitleLineWithButton,
  },
  data() {
    return {
    };
  },
  mounted() {
  },
  beforeUnmount() {
  },
  methods: {
    mdiChartTimelineVariant() {
      return mdiChartTimelineVariant;
    },
    getSshUiUrl() {
      return window.location.protocol + '//' + window.location.hostname + ':2222';
    },
  },
};
</script>