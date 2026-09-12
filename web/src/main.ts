import { createApp } from "vue";
import { createRouter, createWebHistory } from "vue-router";
import App from "./App.vue";
import { i18n } from "./i18n";
import HomeView from "./views/HomeView.vue";
import AuthView from "./views/AuthView.vue";
import JoinView from "./views/JoinView.vue";
import RoomView from "./views/RoomView.vue";
import DisplayView from "./views/DisplayView.vue";
import "./style.css";
import "./layout.css";
import "./activities.css";
import "./motion.css";
import "./background.css";
import { installMotionPreferences, motionReveal } from "./motion";
import DialogTransition from "./components/DialogTransition.vue";
const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: "/", component: HomeView },
    { path: "/login", component: AuthView },
    { path: "/setup", component: AuthView },
    { path: "/join", component: JoinView },
    { path: "/room/:id", component: RoomView },
    { path: "/display/:id?", component: DisplayView },
    { path: "/:pathMatch(.*)*", redirect: "/" },
  ],
});
installMotionPreferences();
createApp(App)
  .use(i18n)
  .use(router)
  .directive("motion", motionReveal)
  .directive("pretty-select", prettySelect)
  .component("DialogTransition", DialogTransition)
  .mount("#app");
import "./interactions.css";

import { prettySelect } from "./prettySelect";
import "./polish.css";
import "./board.css";
