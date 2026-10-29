import { createSSRApp } from "@dcloudio/uni-h5-vue"
import App from "./App.uvue"

export function createApp() {
  const app = createSSRApp(App)
  return { app }
}
