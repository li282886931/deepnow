declare function createVueApp(rootComponent: unknown, rootProps?: unknown): {
  use(plugin: unknown): unknown
}

interface ImportMeta {
  env: Record<string, string | undefined>
}

declare const uni: {
  navigateTo(options: { url: string }): void
  reLaunch(options: { url: string }): void
}
