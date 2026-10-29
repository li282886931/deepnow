import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import {
  getConfig,
  getDiagnostics,
  getRules,
  saveConfig,
  saveRules,
} from "../../src/services/policies.uts"
import { listAuditEvents } from "../../src/services/audit.uts"
import type { SystemConfig } from "../../src/types/domain.uts"

describe("operations services", () => {
  const fetchMock = vi.fn()
  const config: SystemConfig = {
    endpoint_ip: "0.0.0.0",
    endpoint_port: "8443",
    webui_ip: "0.0.0.0",
    webui_port: "8080",
    domain_name: "",
    max_total_conns: 1000,
    max_conns_per_ip: 50,
    cert_file: "server.crt",
    key_file: "server.key",
  }

  beforeEach(() => {
    vi.stubGlobal("fetch", fetchMock)
    fetchMock.mockReset()
    fetchMock.mockImplementation(async (url: string) => new Response(JSON.stringify({
      success: true,
      message: "ok",
      data: url === "/api/rules/get"
        ? { rag_slider: 2 }
        : url === "/api/config/get"
          ? config
          : url === "/api/diagnostics"
            ? { database: {}, active_model: true, embedding_model: false, build: "build-1" }
            : [{ id: 1, actor: "admin", action: "config.save", resource_type: "config", resource_id: "", result: "success", created_at: "2026-10-29" }],
      request_id: "req-1",
    }), { status: 200, headers: { "content-type": "application/json" } }))
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it("reads all operational data from the real backend endpoints", async () => {
    await expect(getRules()).resolves.toEqual({ rag_slider: 2 })
    await expect(getConfig()).resolves.toEqual(config)
    await expect(getDiagnostics()).resolves.toMatchObject({ build: "build-1" })
    await expect(listAuditEvents(25)).resolves.toHaveLength(1)
    expect(fetchMock.mock.calls.map(([url]) => String(url))).toEqual([
      "/api/rules/get",
      "/api/config/get",
      "/api/diagnostics",
      "/api/audit/events?limit=25",
    ])
  })

  it("sends validated saves to the real endpoints", async () => {
    await saveRules({ rag_slider: 0 })
    await saveConfig(config)
    expect(fetchMock.mock.calls[0][0]).toBe("/api/rules/save")
    expect(fetchMock.mock.calls[0][1]).toMatchObject({
      method: "POST",
      body: JSON.stringify({ rag_slider: 0 }),
    })
    expect(fetchMock.mock.calls[1][0]).toBe("/api/config/save")
    expect(fetchMock.mock.calls[1][1]).toMatchObject({
      method: "POST",
      body: JSON.stringify(config),
    })
  })

  it("rejects invalid saves before making a request so the form can retain input", async () => {
    await expect(saveConfig({ ...config, endpoint_port: "0" })).rejects.toMatchObject({
      code: "validation",
    })
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
