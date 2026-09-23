import { useEffect, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { api } from "./api";
import { toastError, toastSuccess } from "./swal";
import { FormDialog, formatRp } from "./ui";
import type { PayableInvoice } from "./payMethod";

export type PortalSub = {
  id?: string;
  username: string;
  plan_id?: string;
  plan_name: string;
  status: string;
  customer_id?: string;
  customer_name?: string;
  customer_code?: string;
  service_type?: string;
};

export type PortalPlan = {
  id: string;
  name: string;
  code: string;
  price: number;
  original_price?: number;
  discount_label?: string;
  download_mbps: number;
  upload_mbps: number;
  service_type: string;
  billing_cycle: string;
};

type PlanChangeQuote = {
  old_plan_name: string;
  new_plan_name: string;
  old_price: number;
  new_price: number;
  old_download_mbps?: number;
  old_upload_mbps?: number;
  new_download_mbps?: number;
  new_upload_mbps?: number;
  remaining_days: number;
  period_days: number;
  old_credit: number;
  new_charge: number;
  delta_subtotal: number;
  tax_amount: number;
  total_amount: number;
  direction: string;
  requires_charge: boolean;
};

const CHANGEABLE = new Set(["active", "suspended", "overdue"]);

export function canChangePortalPlan(status?: string) {
  return CHANGEABLE.has(String(status || "").toLowerCase());
}

export function planSpeedLabel(p: Pick<PortalPlan, "download_mbps" | "upload_mbps">) {
  const ul = p.upload_mbps || p.download_mbps;
  return `${p.download_mbps}/${ul} Mbps`;
}

export function billingCycleLabel(cycle?: string) {
  switch (String(cycle || "").toLowerCase()) {
    case "yearly":
      return "per tahun";
    case "weekly":
      return "per minggu";
    case "daily":
      return "per hari";
    default:
      return "per bulan";
  }
}

export function PortalPlanCatalog({
  plans,
  currentPlanIds,
  onPick,
}: {
  plans: PortalPlan[];
  currentPlanIds?: Set<string>;
  onPick?: (plan: PortalPlan) => void;
}) {
  if (plans.length === 0) {
    return <p className="text-sm text-[var(--muted)]">Belum ada paket yang ditawarkan.</p>;
  }
  return (
    <div className="portal-plan-catalog">
      {plans.map((p) => {
        const current = currentPlanIds?.has(p.id) === true;
        const clickable = Boolean(onPick) && !current;
        return (
          <article key={p.id} className={`portal-plan-card${current ? " is-current" : ""}`}>
            <div className="min-w-0">
              <p className="text-sm font-semibold">{p.name}</p>
              <p className="text-xs text-[var(--muted)]">
                {planSpeedLabel(p)}
                {p.billing_cycle ? ` · ${billingCycleLabel(p.billing_cycle)}` : ""}
              </p>
            </div>
            <div className="flex shrink-0 flex-col items-end gap-1">
              {p.original_price && p.original_price > p.price ? (
                <p className="text-xs text-[var(--muted)] line-through">{formatRp(p.original_price)}</p>
              ) : null}
              <p className="text-sm font-bold">{formatRp(p.price)}</p>
              {p.discount_label ? (
                <span className="text-[10px] font-semibold text-[var(--accent)]">{p.discount_label}</span>
              ) : null}
              {current ? (
                <span className="text-[10px] font-semibold uppercase tracking-wide text-[var(--accent)]">Paket Anda</span>
              ) : clickable ? (
                <button type="button" className="btn" onClick={() => onPick?.(p)}>
                  Pilih
                </button>
              ) : null}
            </div>
          </article>
        );
      })}
    </div>
  );
}

export function PortalChangePlanDialog({
  sub,
  headers,
  initialPlanId,
  onClose,
  onChanged,
}: {
  sub: PortalSub | null;
  headers?: HeadersInit;
  initialPlanId?: string;
  onClose: () => void;
  onChanged: (invoice?: PayableInvoice) => void;
}) {
  const [planId, setPlanId] = useState("");
  const [acceptNoRefund, setAcceptNoRefund] = useState(false);

  useEffect(() => {
    setPlanId(initialPlanId || "");
    setAcceptNoRefund(false);
  }, [sub?.id, initialPlanId]);

  const plansQ = useQuery({
    queryKey: ["portal-sub-plans", sub?.id],
    queryFn: () => api<{ data: PortalPlan[] }>(`/api/portal/subscriptions/${sub!.id}/plans`, { headers }),
    enabled: Boolean(sub?.id && headers),
    retry: false,
  });

  const previewQ = useQuery({
    queryKey: ["portal-change-plan-preview", sub?.id, planId],
    queryFn: () =>
      api<PlanChangeQuote>(`/api/portal/subscriptions/${sub!.id}/change-plan/preview`, {
        method: "POST",
        headers,
        body: JSON.stringify({ plan_id: planId }),
      }),
    enabled: Boolean(sub?.id && planId && headers),
    retry: false,
  });

  const apply = useMutation({
    mutationFn: () =>
      api<{
        quote: PlanChangeQuote;
        invoice?: PayableInvoice & { total_amount: number; invoice_number: string; status: string };
        status: string;
        pending_payment?: boolean;
      }>(`/api/portal/subscriptions/${sub!.id}/change-plan`, {
        method: "POST",
        headers,
        body: JSON.stringify({ plan_id: planId }),
      }),
    onSuccess: (res) => {
      const q = res.quote;
      if (q.direction === "downgrade") {
        void toastSuccess("Paket diturunkan. Sisa tagihan tidak di-refund; kecepatan baru langsung berlaku.");
      } else if (res.pending_payment && res.invoice) {
        void toastSuccess(
          `Upgrade dicatat. Lunasi tagihan selisih ${formatRp(res.invoice.total_amount)} dulu, kecepatan baru aktif setelah lunas.`,
        );
      } else if (q.requires_charge && res.invoice) {
        void toastSuccess(`Paket diupgrade. Tagihan selisih ${formatRp(res.invoice.total_amount)}.`);
      } else {
        void toastSuccess("Paket diganti");
      }
      onChanged(res.invoice);
    },
    onError: (e: Error) => void toastError(e.message || "Gagal Upgrade / Downgrade"),
  });

  const plans = Array.isArray(plansQ.data?.data) ? plansQ.data.data : [];
  const quote = previewQ.data;
  const isDowngrade = quote?.direction === "downgrade";
  const directionLabel =
    quote?.direction === "upgrade"
      ? "Upgrade"
      : quote?.direction === "downgrade"
        ? "Downgrade"
        : "Upgrade / Downgrade";
  const canSubmit =
    Boolean(planId) &&
    Boolean(quote) &&
    !previewQ.isFetching &&
    !apply.isPending &&
    (!isDowngrade || acceptNoRefund);

  return (
    <FormDialog open={Boolean(sub)} title={directionLabel} onClose={onClose} wide>
      {sub ? (
        <form
          className="grid gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (!canSubmit) return;
            apply.mutate();
          }}
        >
          <p className="text-sm text-[var(--muted)]">
            Sekarang: <span className="font-medium text-[var(--text)]">{sub.plan_name || "—"}</span>
            {sub.username ? ` · ${sub.username}` : ""}
          </p>
          {plansQ.isLoading ? (
            <p className="text-sm text-[var(--muted)]">Memuat paket…</p>
          ) : plans.length === 0 ? (
            <p className="text-sm text-[var(--muted)]">Belum ada paket lain yang ditawarkan untuk akun ini.</p>
          ) : (
            <div className="grid gap-2">
              {plans.map((p) => {
                const selected = planId === p.id;
                return (
                  <button
                    key={p.id}
                    type="button"
                    className={`portal-plan-option${selected ? " is-selected" : ""}`}
                    onClick={() => {
                      setPlanId(p.id);
                      setAcceptNoRefund(false);
                    }}
                  >
                    <span className="min-w-0">
                      <span className="block text-sm font-semibold">{p.name}</span>
                      <span className="block text-xs text-[var(--muted)]">{planSpeedLabel(p)}</span>
                    </span>
                    <span className="shrink-0 text-sm font-bold">{formatRp(p.price)}</span>
                  </button>
                );
              })}
            </div>
          )}
          {previewQ.isFetching ? <p className="text-sm text-[var(--muted)]">Menghitung selisih…</p> : null}
          {previewQ.isError ? (
            <p className="text-sm text-[var(--danger)]">{(previewQ.error as Error).message}</p>
          ) : null}
          {quote ? (
            <div className="rounded-lg border border-[var(--border)] bg-[var(--panel-muted)]/50 p-3 text-sm">
              <p className="mb-2 text-xs font-bold uppercase tracking-wide text-[var(--muted)]">
                {quote.direction === "upgrade"
                  ? "Upgrade (speed naik)"
                  : quote.direction === "downgrade"
                    ? "Downgrade (speed turun)"
                    : "Sama speed"}
                {" · "}sisa {quote.remaining_days}/{quote.period_days} hari
              </p>
              <div className="flex justify-between gap-2">
                <span className="text-[var(--muted)]">Harga lama</span>
                <span>{formatRp(quote.old_price)}</span>
              </div>
              <div className="mt-1 flex justify-between gap-2">
                <span className="text-[var(--muted)]">Harga baru</span>
                <span>{formatRp(quote.new_price)}</span>
              </div>
              <div className="mt-2 flex justify-between gap-2 border-t border-[var(--border)] pt-2 font-medium">
                <span>{quote.requires_charge ? "Ditagih sekarang" : "Tagihan sekarang"}</span>
                <span>{quote.requires_charge ? formatRp(quote.total_amount) : "Rp 0"}</span>
              </div>
              <p className="mt-2 text-xs text-[var(--muted)]">
                Tanggal tagihan berikutnya tidak berubah. Siklus berikutnya memakai harga paket baru.
                {quote.direction === "upgrade" && quote.requires_charge
                  ? " Upgrade aktif + router diupdate setelah tagihan selisih lunas."
                  : ""}
              </p>
            </div>
          ) : null}
          {isDowngrade ? (
            <label className="flex items-start gap-2 rounded-lg border border-[var(--warn)]/40 bg-[color-mix(in_srgb,var(--warn)_12%,transparent)] p-3 text-sm">
              <input
                type="checkbox"
                className="mt-1"
                checked={acceptNoRefund}
                onChange={(e) => setAcceptNoRefund(e.target.checked)}
              />
              <span>
                <strong>Sisa tagihan tidak bisa di-refund.</strong> Downgrade langsung memakai kecepatan baru;
                selisih harga lama tidak dikembalikan uang.
              </span>
            </label>
          ) : null}
          <div className="flex flex-wrap gap-2">
            <button className="btn" disabled={!canSubmit}>
              {apply.isPending ? "Memproses…" : `Konfirmasi ${directionLabel}`}
            </button>
            <button type="button" className="btn-ghost" onClick={onClose}>
              Batal
            </button>
          </div>
        </form>
      ) : null}
    </FormDialog>
  );
}
