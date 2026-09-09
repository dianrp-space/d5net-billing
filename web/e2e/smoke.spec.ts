import { test, expect } from "@playwright/test";

test("health endpoint", async ({ request }) => {
  const res = await request.get("http://127.0.0.1:8087/api/health");
  expect(res.ok()).toBeTruthy();
  const body = await res.json();
  expect(body.status).toBe("ok");
});
