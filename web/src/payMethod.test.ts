import { afterEach, describe, expect, it } from "vitest";
import {
  confirmPaymentAfterReturn,
  consumePaymentReturn,
  consumePaymentReturnSuccess,
  getSavedPayMethod,
  invoiceRemaining,
  isInvoiceUnpaid,
  isIsolirStatus,
  isPayMethodId,
  notePaymentReturnFromLocation,
  PAY_METHOD_DUITKU,
  paymentMethodLabel,
  payOptionsHasDuitkuSandbox,
  portalPaymentReturnURL,
  setSavedPayMethod,
} from "./payMethod";

describe("payMethod", () => {
  afterEach(() => {
    localStorage.clear();
    sessionStorage.clear();
  });

  it("persists last method", () => {
    expect(getSavedPayMethod("acme")).toBe(PAY_METHOD_DUITKU);
    setSavedPayMethod(PAY_METHOD_DUITKU, "acme");
    expect(getSavedPayMethod("acme")).toBe("duitku");
    expect(isPayMethodId("duitku")).toBe(true);
    expect(isPayMethodId("doku")).toBe(true);
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
    expect(paymentMethodLabel("duitku")).toBe("DUITKU");
    expect(paymentMethodLabel("doku")).toBe("DOKU");
    expect(paymentMethodLabel("")).toBe("—");
    expect(paymentMethodLabel("midtrans")).toBe("midtrans");
    expect(
      payOptionsHasDuitkuSandbox([{ provider: "duitku", label: "Duitku", description: "", kind: "popup", sandbox: true }]),
    ).toBe(true);
    expect(
      payOptionsHasDuitkuSandbox([{ provider: "duitku", label: "Duitku", description: "", kind: "popup", sandbox: false }]),
    ).toBe(false);
    expect(
      payOptionsHasDuitkuSandbox([{ provider: "doku", label: "DOKU", description: "", kind: "redirect", sandbox: true }]),
    ).toBe(false);
  });

  it("marks and consumes payment return from query", () => {
    window.history.replaceState(null, "", "/client/dashboard?payment=return&x=1");
    expect(portalPaymentReturnURL()).toContain("payment=return");
    expect(portalPaymentReturnURL()).not.toContain("payment=success");
    notePaymentReturnFromLocation();
    expect(consumePaymentReturnSuccess()).toBe(true);
    expect(window.location.search).not.toContain("payment=return");
    expect(consumePaymentReturnSuccess()).toBe(false);
  });

  it("still consumes legacy payment=success return marker", () => {
    window.history.replaceState(null, "", "/client/dashboard?payment=success");
    expect(consumePaymentReturnSuccess()).toBe(true);
    expect(window.location.search).not.toContain("payment=success");
  });

  it("reads Duitku resultCode and confirms paid after webhook already settled", async () => {
    window.history.replaceState(null, "", "/client/dashboard?payment=return&resultCode=00&merchantOrderId=INV-1");
    const info = consumePaymentReturn();
    expect(info.returned).toBe(true);
    expect(info.resultCode).toBe("00");
    expect(window.location.search).not.toContain("resultCode");

    const confirmed = await confirmPaymentAfterReturn({
      resultCode: info.resultCode,
      attempts: 1,
      delayMs: 1,
      fetchInvoices: async () => [
        { id: "1", invoice_number: "INV-1", status: "paid", total_amount: 100, paid_amount: 100 },
      ],
      fetchPaymentIntent: async () => null,
      fetchPayments: async () => [{ status: "paid", paid_at: new Date().toISOString() }],
    });
    expect(confirmed).toBe(true);
  });

  it("does not confirm when Duitku resultCode is canceled", async () => {
    const confirmed = await confirmPaymentAfterReturn({
      resultCode: "02",
      attempts: 2,
      delayMs: 1,
      fetchInvoices: async () => [
        { id: "1", invoice_number: "INV-1", status: "issued", total_amount: 100, paid_amount: 0 },
      ],
      fetchPaymentIntent: async () => ({ status: "pending" }),
    });
    expect(confirmed).toBe(false);
  });
});
