import { describe, expect, it } from "vitest";
import { buildClientBellItems, type BellInvoice, type BellPayment } from "./ClientBell";

function payment(over: Partial<BellPayment> & { amount: number }): BellPayment {
  return { status: "paid", method: "qris", ...over };
}

describe("buildClientBellItems", () => {
  it("notifies for an issued unpaid invoice", () => {
    const items = buildClientBellItems(
      [
        {
          id: "inv-1",
          invoice_number: "INV-001",
          total_amount: 150000,
          paid_amount: 0,
          status: "issued",
          due_date: "2999-01-01",
        },
      ],
      [],
      [],
      false,
    );
    expect(items).toHaveLength(1);
    expect(items[0].key).toBe("inv-inv-1");
    expect(items[0].title).toBe("Tagihan belum dibayar");
    expect(items[0].severity).toBe("warn");
  });

  it("keeps the newest paid notification even with more than five payments", () => {
    // Portal mengembalikan pembayaran terbaru lebih dulu (DESC).
    const payments = Array.from({ length: 8 }, (_, k) =>
      payment({ amount: (k + 1) * 1000, invoice_number: `INV-${k + 1}`, paid_at: `2026-09-0${k + 1}T00:00:00Z` }),
    ).reverse();

    const items = buildClientBellItems([], payments, [], false);
    const paid = items.filter((i) => i.title === "Pembayaran diterima");
    expect(paid).toHaveLength(5);
    // Yang terbaru (INV-8) harus muncul, bukan yang paling lama.
    expect(paid.map((i) => i.message).join(" ")).toContain("INV-8");
    expect(paid.some((i) => /INV-1\b/.test(i.message))).toBe(false);
  });

  it("notifies payment received and clears the matching unpaid invoice", () => {
    const invoices: BellInvoice[] = [
      {
        id: "inv-9",
        invoice_number: "INV-009",
        total_amount: 150000,
        paid_amount: 150000,
        status: "paid",
        due_date: "2026-01-01",
      },
    ];
    const payments = [payment({ amount: 150000, invoice_number: "INV-009", paid_at: "2026-09-14T00:00:00Z" })];
    const items = buildClientBellItems(invoices, payments, [], false);
    expect(items.some((i) => i.title === "Tagihan belum dibayar")).toBe(false);
    const paid = items.find((i) => i.title === "Pembayaran diterima");
    expect(paid).toBeDefined();
    expect(paid?.severity).toBe("ok");
  });

  it("ignores non-paid payments", () => {
    const items = buildClientBellItems(
      [],
      [payment({ amount: 50000, status: "pending", invoice_number: "INV-010" })],
      [],
      false,
    );
    expect(items).toHaveLength(0);
  });
});
