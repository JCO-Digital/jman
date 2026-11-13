import { createApp } from "vue";
import { createPinia } from "pinia";
import "./style.css";
import App from "./App.vue";
import router from "./router";
import { installAppUpdateHandlers } from "./utils/appUpdate";
// Applies the remembered color theme and follows the system's.
import "./composables/useTheme";

const app = createApp(App);

app.use(createPinia());
app.use(router);
installAppUpdateHandlers(router);

app.mount("#app");
