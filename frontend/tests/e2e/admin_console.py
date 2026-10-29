#!/usr/bin/env python3
"""Smoke-test the H5 admin console with mocked admin API responses."""

from __future__ import annotations

import argparse
from typing import Any
from urllib.parse import urlparse

from playwright.sync_api import Page, Route, expect, sync_playwright


VIEWPORTS = ((1440, 900), (1024, 768), (390, 844))
MODEL = {
    "provider": "openai",
    "api_type": "LM",
    "model_name": "gpt-smoke",
    "api_key": "sk-****abcd",
    "api_key_masked": "sk-****abcd",
    "ip_port": "https://api.example.com",
    "endpoint": "https://api.example.com",
}
ENVELOPE: dict[str, Any] = {
    "success": True,
    "message": "",
    "request_id": "req_playwright_smoke",
}


def response(data: Any) -> dict[str, Any]:
    return {**ENVELOPE, "data": data}


def mock_admin_api(route: Route) -> None:
    """Return only deterministic test responses; no application data is changed."""
    request = route.request
    if not urlparse(request.url).path.startswith("/api/"):
        route.continue_()
        return
    path = request.url.split("?", 1)[0].rstrip("/").split("/api", 1)[-1]

    if path == "/login":
        body = response(None)
    elif path == "/session":
        body = response({"username": "admin", "display_name": "Smoke Admin"})
    elif path == "/system/status":
        body = response(
            {
                "needs_setup": False,
                "healthy": True,
                "active_pri": MODEL,
                "active_sec": None,
                "active_rag": None,
            }
        )
    elif path == "/system/build":
        body = response("playwright-smoke")
    elif path == "/models/get":
        body = response([MODEL])
    elif path == "/keys/list":
        body = response(
            [
                {
                    "id": "key-smoke",
                    "key": "sk-aibase-****1234",
                    "name": "Smoke key",
                    "status": "active",
                    "request_count": 3,
                    "input_tokens": 10,
                    "output_tokens": 8,
                    "error_count": 0,
                    "created_at": "2026-10-29T00:00:00Z",
                    "last_used_at": None,
                }
            ]
        )
    elif path == "/rag/status":
        body = response(
            {"total_chunks": 2, "total_files": 1, "last_update": "2026-10-29"}
        )
    elif path == "/rag/documents":
        body = response(
            [
                {
                    "id": 1,
                    "filename": "smoke.md",
                    "status": "ready",
                    "size": 2048,
                    "chunk_count": 2,
                    "error_message": "",
                    "created_at": "2026-10-29T00:00:00Z",
                }
            ]
        )
    elif path == "/logout":
        body = response(None)
    else:
        body = response({})

    route.fulfill(
        status=200,
        content_type="application/json",
        body=_json(body),
    )


def _json(value: Any) -> str:
    import json

    return json.dumps(value)


def assert_no_horizontal_overflow(page: Page) -> None:
    assert page.evaluate(
        "() => document.documentElement.scrollWidth <= window.innerWidth "
        "&& document.body.scrollWidth <= window.innerWidth"
    ), "page has horizontal overflow"


def login(page: Page, base_url: str) -> None:
    page.goto(f"{base_url}/#/pages/login/login")
    page.wait_for_load_state("networkidle")
    page.wait_for_timeout(500)
    expect(page.get_by_text("登录管理控制台", exact=True)).to_be_visible()
    page.locator("input").nth(0).fill("admin")
    page.locator("input").nth(1).fill("password")
    page.get_by_text("登录", exact=True).click()
    expect(page.get_by_text("服务状态", exact=True)).to_be_visible()


def exercise_navigation(page: Page, viewport_width: int) -> None:
    if viewport_width <= 767:
        page.locator(".tabbar").get_by_text("模型", exact=True).click()
        expect(page.get_by_text("模型配置", exact=True)).to_be_visible()
        page.locator(".tabbar").get_by_text("Key", exact=True).click()
        expect(page.get_by_text("API Key", exact=True)).to_be_visible()
        page.locator(".tabbar").get_by_text("知识库", exact=True).click()
    elif viewport_width >= 1200:
        page.locator(".sidebar").get_by_text("模型管理", exact=True).click()
        expect(page.get_by_text("模型配置", exact=True)).to_be_visible()
        page.locator(".sidebar").get_by_text("访问控制", exact=True).click()
        expect(page.get_by_text("API Key", exact=True)).to_be_visible()
        page.locator(".sidebar").get_by_text("知识库", exact=True).click()
    else:
        for path, heading in (
            ("models/models", "模型配置"),
            ("access-keys/access-keys", "API Key"),
            ("knowledge/knowledge", "知识库管理"),
        ):
            page.goto(f"{page.url.split('#', 1)[0]}#/pages/{path}")
            expect(page.get_by_text(heading, exact=True)).to_be_visible()
        return

    expect(page.get_by_text("知识库管理", exact=True)).to_be_visible()


def run(base_url: str) -> None:
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch()
        try:
            for width, height in VIEWPORTS:
                page = browser.new_page(viewport={"width": width, "height": height})
                page.route("**/api/**", mock_admin_api)
                login(page, base_url)
                assert_no_horizontal_overflow(page)
                exercise_navigation(page, width)
                assert_no_horizontal_overflow(page)
                page.close()
                print(f"PASS viewport={width}x{height}")
        finally:
            browser.close()


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--base-url",
        default="http://127.0.0.1:5173",
        help="running H5 server URL (default: http://127.0.0.1:5173)",
    )
    args = parser.parse_args()
    run(args.base_url.rstrip("/"))


if __name__ == "__main__":
    main()
