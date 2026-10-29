import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import {
  deleteKnowledgeDocument,
  formatDocumentStatus,
  getKnowledgeDocuments,
  getKnowledgeStatus,
  testKnowledgeRetrieval,
} from "../../src/services/knowledge.uts"

describe("typed knowledge service", () => {
  const fetchMock = vi.fn()

  beforeEach(() => {
    vi.stubGlobal("fetch", fetchMock)
    fetchMock.mockReset()
    fetchMock.mockImplementation(async (input: string, init?: RequestInit) => {
      const url = String(input)
      if (url === "/api/rag/status") {
        return new Response(JSON.stringify({
          success: true,
          message: "",
          data: { total_chunks: 12, total_files: 2, last_update: "12:30:00" },
          request_id: "status-1",
        }), { status: 200 })
      }
      if (url === "/api/rag/documents" && (!init || init.method === "GET")) {
        return new Response(JSON.stringify({
          success: true,
          message: "",
          data: [{
            id: 7,
            filename: "guide.txt",
            status: "ready",
            size: 2048,
            chunk_count: 3,
            error_message: "",
            created_at: "2026-10-29T10:00:00Z",
          }],
          request_id: "documents-1",
        }), { status: 200 })
      }
      if (url === "/api/rag/test") {
        return new Response(JSON.stringify({
          success: true,
          message: "检索完成",
          data: { query: "退款", match: "退款规则...", score: 0.91 },
          request_id: "test-1",
        }), { status: 200 })
      }
      if (url === "/api/rag/documents/7" && init?.method === "DELETE") {
        return new Response(JSON.stringify({
          success: true,
          message: "文档已删除",
          data: null,
          request_id: "delete-1",
        }), { status: 200 })
      }
      throw new Error(`unexpected request: ${url}`)
    })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it("maps status and document responses to typed domain values", async () => {
    await expect(getKnowledgeStatus()).resolves.toEqual({
      total_chunks: 12,
      total_files: 2,
      last_update: "12:30:00",
    })
    await expect(getKnowledgeDocuments()).resolves.toEqual([{
      id: 7,
      filename: "guide.txt",
      status: "ready",
      size: 2048,
      chunk_count: 3,
      error: "",
      created_at: "2026-10-29T10:00:00Z",
    }])
  })

  it("uses the retrieval endpoint without invoking model generation", async () => {
    await expect(testKnowledgeRetrieval("退款")).resolves.toEqual({
      query: "退款",
      match: "退款规则...",
      score: 0.91,
    })

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/rag/test",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ query: "退款" }),
      }),
    )
    expect(fetchMock.mock.calls.some(([url]) => String(url).includes("chat/completions"))).toBe(false)
  })

  it("deletes a document by its numeric id", async () => {
    await expect(deleteKnowledgeDocument(7)).resolves.toBeUndefined()
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/rag/documents/7",
      expect.objectContaining({ method: "DELETE" }),
    )
  })
})

describe("document status presentation", () => {
  it("keeps processing, ready, and error states explicit", () => {
    expect(formatDocumentStatus("processing")).toEqual({ label: "处理中", tone: "warning" })
    expect(formatDocumentStatus("ready")).toEqual({ label: "已就绪", tone: "success" })
    expect(formatDocumentStatus("error")).toEqual({ label: "处理失败", tone: "error" })
  })
})
