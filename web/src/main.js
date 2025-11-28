import { createPinia } from 'pinia'
import { createApp } from 'vue'

import ApiService from "@/services/ApiService"
import { useMainStore } from '@/stores/main.js'
import App from './App.vue'
import router from './router'

import 'datatables.net-dt/css/dataTables.dataTables.css'
import './css/main.css'

import DataTables from 'datatables.net'
import DataTablesVue from 'datatables.net-vue3'
import DataTable from 'datatables.net-vue3'
import DataTablesLib from 'datatables.net';
import Toast from "vue-toastification";
import "vue-toastification/dist/index.css";


DataTable.use(DataTablesLib);
DataTables.use(DataTablesVue);


// Init Pinia
const pinia = createPinia()

// Create Vue app
const app = createApp(App)
ApiService.init(app);
app.use(router).use(pinia).mount('#app')
app.use(Toast, {});

// Init main store
const mainStore = useMainStore(pinia)

// Fetch sample data
mainStore.fetchSampleClients()
mainStore.fetchSampleHistory()

// Dark mode
// Uncomment, if you'd like to restore persisted darkMode setting, or use `prefers-color-scheme: dark`. Make sure to uncomment localStorage block in src/stores/darkMode.js
import { useDarkModeStore } from './stores/darkMode'

const darkModeStore = useDarkModeStore(pinia)

// if (
//   (!localStorage['darkMode'] && window.matchMedia('(prefers-color-scheme: dark)').matches) ||
//   localStorage['darkMode'] === '1'
// ) {
//   darkModeStore.set(true)
// }
darkModeStore.set(true)

// Default title tag
const defaultDocumentTitle = 'Redock'

// Set document title from route meta
router.afterEach((to) => {
  document.title = to.meta?.title
    ? `${to.meta.title} — ${defaultDocumentTitle}`
    : defaultDocumentTitle
})
