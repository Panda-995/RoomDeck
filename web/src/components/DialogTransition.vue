<script setup lang="ts">
import { leaveDialog } from "../dialogMotion";
</script>
<template>
  <Transition :css="false" @leave="leaveDialog"><slot /></Transition>
</template>
