import { describe, expect, it } from "vitest";
import { currentMonthKey, monthKeyLabel, shiftMonthKey } from "./ClientHome";

describe("usage month helpers", () => {
  it("builds YYYY-MM keys", () => {
    expect(currentMonthKey(new Date(2026, 0, 15))).toBe("2026-01");
    expect(currentMonthKey(new Date(2026, 8, 21))).toBe("2026-09");
  });

  it("shifts across year boundaries", () => {
    expect(shiftMonthKey("2026-01", -1)).toBe("2025-12");
    expect(shiftMonthKey("2025-12", 1)).toBe("2026-01");
    expect(shiftMonthKey("2026-09", 1)).toBe("2026-10");
  });

  it("labels months in Indonesian", () => {
    expect(monthKeyLabel("2026-09")).toBe("September 2026");
    expect(monthKeyLabel("2026-01")).toBe("Januari 2026");
  });
});
