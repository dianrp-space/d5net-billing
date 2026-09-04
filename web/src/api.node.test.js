import { describe, it } from "node:test";
import assert from "node:assert/strict";

const store = new Map();
const localStorage = {
  getItem: (k) => (store.has(k) ? store.get(k) : null),
  setItem: (k, v) => store.set(k, String(v)),
  removeItem: (k) => store.delete(k),
};

const TOKEN_KEY = "drp_admin_token";
function getToken() {
  return localStorage.getItem(TOKEN_KEY);
}
function setToken(token) {
  localStorage.setItem(TOKEN_KEY, token);
}
function clearToken() {
  localStorage.removeItem(TOKEN_KEY);
}

describe("token storage", () => {
  it("stores and clears token", () => {
    setToken("abc");
    assert.equal(getToken(), "abc");
    clearToken();
    assert.equal(getToken(), null);
  });
});
