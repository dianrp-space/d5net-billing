import { afterEach, describe, expect, it } from "vitest";
import {
  getSavedPayMethod,
  invoiceRemaining,
  isInvoiceUnpaid,
  isIsolirStatus,
  isPayMethodId,
  PAY_METHOD_DUITKU,
  paymentMethodLabel,
  setSavedPayMethod,
} from "./payMethod";

describe("payMethod", () => {
  afterEach(() => {
    localStorage.clear();
  });

  it("persists last method", () => {
    expect(getSavedPayMethod("acme")).toBe(PAY_METHOD_DUITKU);
    setSavedPayMethod(PAY_METHOD_DUITKU, "acme");
    expect(getSavedPayMethod("acme")).toBe("duitku");
    expect(isPayMethodId("duitku")).toBe(true);
    expect(isPayMethodId("qris")).toBe(false);
    expect(isPayMethodId("va")).toBe(false);
  });

  it("detects isolir and unpaid invoices", () => {
    expect(isIsolirStatus("suspended")).toBe(true);
    expect(isIsolirStatus("ISOLIR")).toBe(true);
    expect(isIsolirStatus("active")).toBe(false);
    expect(invoiceRemaining({ total_amount: 150000, paid_amount: 50000 })).toBe(100000);
    expect(isInvoiceUnpaid({ id: "1", invoice_number: "INV", status: "issued", total_amount: 100, paid_amount: 0 })).toBe(true);
    expect(isInvoiceUnpaid({ id: "1", invoice_number: "INV", status: "paid", total_amount: 100, paid_amount: 100 })).toBe(false);
    expect(paymentMethodLabel("qris")).toBe("QRIS");
    expect(paymentMethodLabel("drp")).toBe("QRIS");
    expect(paymentMethodLabel("duitku")).toBe("Duitku Payment Gateway");
    expect(paymentMethodLabel("")).toBe("—");
    expect(paymentMethodLabel("midtrans")).toBe("midtrans");
  });
});
