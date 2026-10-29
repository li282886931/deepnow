import { describe, expect, it } from "vitest"
import { maskSecret } from "../../src/utils/mask.uts"

describe("maskSecret", () => {
  it("never returns the complete secret", () => {
    const secret = "sk-live-secret"
    const masked = maskSecret(secret)

    expect(masked).not.toContain(secret)
    expect(masked).toBe("sk-****cret")
  })

  it("keeps empty values empty", () => {
    expect(maskSecret("")).toBe("")
  })
})
