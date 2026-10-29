import { describe, expect, it } from "vitest"
import {
  defaultPolicyConfig,
  validatePolicyConfig,
  validateSystemConfig,
} from "../../src/services/policies.uts"
import type { SystemConfig } from "../../src/types/domain.uts"

describe("policy and system validation", () => {
  it("rejects a non-numeric or out-of-range port", () => {
    const config: SystemConfig = {
      endpoint_ip: "0.0.0.0",
      endpoint_port: "not-a-port",
      webui_ip: "0.0.0.0",
      webui_port: "8080",
      domain_name: "",
      max_total_conns: 1000,
      max_conns_per_ip: 50,
      cert_file: "server.crt",
      key_file: "server.key",
    }

    expect(validateSystemConfig(config)).toEqual({
      endpoint_port: "端口必须是 1 到 65535 之间的数字",
    })
    expect(validateSystemConfig({ ...config, endpoint_port: "70000" })).toEqual({
      endpoint_port: "端口必须是 1 到 65535 之间的数字",
    })
  })

  it("rejects missing required model policy fields", () => {
    expect(validatePolicyConfig({ rag_slider: -1 })).toEqual({
      rag_slider: "RAG 策略必须是 0、1 或 2",
    })
    expect(validatePolicyConfig({ rag_slider: 1 })).toEqual({})
    expect(defaultPolicyConfig()).toEqual({ rag_slider: 1 })
  })
})
