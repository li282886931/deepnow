import { describe, expect, it } from "vitest"
import { createOneTimeSecret } from "../../src/services/keys.uts"

describe("one-time API key reveal", () => {
  it("does not retain the one-time generated key after acknowledgement", () => {
    const reveal = createOneTimeSecret("sk-aibase-new")

    reveal.acknowledge()

    expect(reveal.value).toBe("")
  })
})
