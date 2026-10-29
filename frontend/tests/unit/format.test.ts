import { describe, expect, it } from "vitest"
import {
  formatCount,
  formatHealth,
  formatTimestamp,
  formatValue,
} from "../../src/utils/format.uts"

describe("formatHealth", () => {
  it("formats unknown health without claiming success", () => {
    expect(formatHealth("unknown")).toEqual({
      label: "未检测",
      tone: "neutral",
    })
  })

  it("formats healthy and unhealthy states for the status badge", () => {
    expect(formatHealth("healthy")).toEqual({ label: "正常", tone: "success" })
    expect(formatHealth("unhealthy")).toEqual({ label: "异常", tone: "error" })
  })
})

describe("formatCount", () => {
  it("formats zero usage as a stable value", () => {
    expect(formatCount(0)).toBe("0")
  })

  it("formats large counts with grouping", () => {
    expect(formatCount(1234567)).toBe("1,234,567")
  })
})

describe("formatValue", () => {
  it("uses a readable placeholder for empty values", () => {
    expect(formatValue("")).toBe("未配置")
    expect(formatValue(null)).toBe("未配置")
  })
})

describe("formatTimestamp", () => {
  it("formats valid timestamps and falls back for invalid values", () => {
    expect(formatTimestamp("2026-10-29T10:20:30Z")).toContain("2026")
    expect(formatTimestamp("not-a-date")).toBe("未记录")
  })
})
