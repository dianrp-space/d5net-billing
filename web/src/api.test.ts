import { describe, expect, it } from "vitest";
import { getToken, setToken, clearToken } from "./api";

describe("token storage", () => {
  it("stores and clears token", () => {
    setToken("abc");
    expect(getToken()).toBe("abc");
    clearToken();
    expect(getToken()).toBeNull();
  });
});
