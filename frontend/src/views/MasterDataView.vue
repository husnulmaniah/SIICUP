<script setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { getConfig } from '../config/tables'
import CrudManager from '../components/CrudManager.vue'
import Message from 'primevue/message'

const route = useRoute()
const config = computed(() => getConfig(route.params.tableKey))
</script>

<template>
  <div class="page-wrap">
    <CrudManager v-if="config" :key="route.params.tableKey" :config="config" />
    <Message v-else severity="warn" :closable="false">Menu tidak ditemukan.</Message>
  </div>
</template>
