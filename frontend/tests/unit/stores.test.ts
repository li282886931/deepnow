import { describe, expect, it, vi } from "vitest"
import { createSessionStore } from "../../src/stores/session.uts"
import { breakpointForWidth, navigationModeForWidth } from "../../src/utils/breakpoints.uts"

describe("session store", () => {
  it("restores an authenticated session from the session response", () => {
    const session = createSessionStore()

    session.restoreSession({ id: "admin", display_name: "Administrator" })

    expect(session.authenticated).toBe(true)
    expect(session.user?.id).toBe("admin")
  })

  it("sets and clears the current session without retaining credentials", () => {
    const session = createSessionStore()

    session.setSession({ id: "admin", display_name: "Administrator" })
    expect(session.authenticated).toBe(true)

    session.clearSession()
    expect(session.authenticated).toBe(false)
    expect(session.user).toBeNull()
  })

  it("clears session and redirects on session_expired", () => {
    const navigateTo = vi.fn()
    const session = createSessionStore({ navigateTo })
    session.setAuthenticated(true)

    session.handleRequestError({ code: "session_expired" })

    expect(session.authenticated).toBe(false)
    expect(navigateTo).toHaveBeenCalledWith("/pages/login/login")
  })
})

describe("responsive breakpoints", () => {
  it("maps desktop, tablet, and mobile widths to stable layout modes", () => {
    expect(breakpointForWidth(1200)).toBe("desktop")
    expect(breakpointForWidth(1199)).toBe("tablet")
    expect(breakpointForWidth(767)).toBe("mobile")
    expect(navigationModeForWidth(390)).toBe("bottom-tabbar")
    expect(navigationModeForWidth(1024)).toBe("compact-sidebar")
    expect(navigationModeForWidth(1440)).toBe("sidebar")
  })
})
