import { createApp } from 'vue';
import { createPinia } from 'pinia';
import DataTable from 'datatables.net-vue3';
import DataTablesCore from 'datatables.net-dt';
import App from './App.vue';
import router from './router';
import './style.css';

DataTable.use(DataTablesCore);

const app = createApp(App);
app.use(createPinia());
app.use(router);
app.component('DataTable', DataTable);
app.mount('#app');
