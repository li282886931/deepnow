import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { request, setUnauthorizedHandler } from "../../src/services/http.uts"

describe("request", () => {
  const fetchMock = vi.fn()

  beforeEach(() => {
    vi.stubGlobal("fetch", fetchMock)
    fetchMock.mockReset()
    setUnauthorizedHandler(undefined)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it("adds credentials and returns a successful API envelope", async () => {
    fetchMock.mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          message: "ok",
          data: { healthy: true },
          request_id: "req-test",
        }),
        { status: 200, headers: { "content-type": "application/json" } },
      ),
    )

    const response = await request<{ healthy: boolean }>("/api/system/status", {
      method: "GET",
    })

    expect(response.data.healthy).toBe(true)
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/system/status",
      expect.objectContaining({ credentials: "include", method: "GET" }),
    )
  })

  it("maps unauthorized responses to session_expired", async () => {
    const unauthorized = vi.fn()
    setUnauthorizedHandler(unauthorized)
    fetchMock.mockResolvedValue(new Response("unauthorized", { status: 401 }))

    await expect(request("/api/session")).rejects.toMatchObject({
      code: "session_expired",
    })
    expect(unauthorized).toHaveBeenCalledOnce()
  })

  it("converts a non-success API envelope to ConsoleError", async () => {
    fetchMock.mockResolvedValue(
      new Response(
        JSON.stringify({
          success: false,
          message: "invalid configuration",
          data: null,
          request_id: "req-invalid",
        }),
        { status: 400, headers: { "content-type": "application/json" } },
      ),
    )

    await expect(request("/api/config/save", { method: "POST" })).rejects.toMatchObject({
      code: "validation",
      requestId: "req-invalid",
    })
  })
})
