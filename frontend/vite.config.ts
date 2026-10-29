import { defineConfig, transformWithEsbuild } from "vite"
import uni from "@dcloudio/vite-plugin-uni"
import uniAppUts from "@dcloudio/uni-app-uts"

export default defineConfig(({ mode }) => {
  if (mode === "test") {
    return {
      plugins: [
        {
          name: "test-uts-typescript",
          async transform(code, id) {
            if (!id.endsWith(".uts")) return
            return transformWithEsbuild(code, id, { loader: "ts" })
          },
        },
      ],
      test: {
        environment: "node",
        include: ["tests/unit/**/*.test.ts"],
      },
    }
  }

  return {
    plugins: [uni(), ...uniAppUts()],
  }
})
