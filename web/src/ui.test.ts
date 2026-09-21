import { describe, expect, it } from "vitest";
import { formatBpsID, formatBytesID } from "./ui";

describe("formatBytesID", () => {
  it("formats bytes with Indonesian units", () => {
    expect(formatBytesID(0)).toBe("0 B");
    expect(formatBytesID(512)).toBe("512 B");
    expect(formatBytesID(1500)).toContain("KB");
    expect(formatBytesID(1024 * 1024 * 2.5)).toContain("MB");
    expect(formatBytesID(1024 ** 3 * 123.4)).toContain("GB");
    expect(formatBytesID(-5)).toBe("0 B");
  });
});

describe("formatBpsID", () => {
  it("formats bits per second", () => {
    expect(formatBpsID(0)).toBe("0 bps");
    expect(formatBpsID(900)).toContain("bps");
    expect(formatBpsID(1500000)).toContain("Mbps");
    expect(formatBpsID(-1)).toBe("0 bps");
  });
});
