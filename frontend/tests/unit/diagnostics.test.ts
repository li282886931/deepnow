import { describe, expect, it } from "vitest"
import {
  diagnosticTone,
  normalizeDiagnostics,
} from "../../src/services/policies.uts"

describe("diagnostics", () => {
  it("renders a failed diagnostic as danger rather than healthy", () => {
    expect(diagnosticTone({ status: "error" })).toBe("danger")
    expect(diagnosticTone({ status: "ok" })).toBe("success")
  })

  it("normalizes database and service checks without inventing health", () => {
    expect(normalizeDiagnostics({
      database: {
        system: { ok: true },
        keys: { ok: false },
        rag: { ok: true },
      },
      active_model: false,
      embedding_model: true,
      build: "2026-10-29T10:00:00Z",
    })).toEqual({
      checks: [
        { key: "database.system", label: "系统数据库", status: "ok" },
        { key: "database.keys", label: "Key 数据库", status: "error" },
        { key: "database.rag", label: "知识库数据库", status: "ok" },
        { key: "active_model", label: "主模型", status: "error" },
        { key: "embedding_model", label: "Embedding 模型", status: "ok" },
      ],
      build: "2026-10-29T10:00:00Z",
    })
  })
})
