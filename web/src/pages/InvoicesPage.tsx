import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, apiDownload } from "../api";
import { ListToolbar, useDebouncedValue } from "../ListToolbar";
import { useAppDialog } from "../confirm";
import { toastError, toastSuccess } from "../swal";
import { formatRp, FormDialog, Input, SearchableSelect, Section, Table, Button, invoiceStatusLabel } from "../ui";
import { Label } from "@/components/ui/label";
import { InvoiceActions } from "../AdminExtra";

type CustomerLookup = {
  id: string;
  customer_code: string;
  full_name: string;
  phone: string;
};

type UnpaidSource = {
  invoice: { invoice_number: string; total_amount: number; paid_amount: number };
  items: { description: string; quantity: number; unit_price: number }[];
};

type IssueItem = { description: string; quantity: number; unit_price: number };

export function InvoicesPage() {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();
  const [status, setStatus] = useState("");
  const [search, setSearch] = useState("");
  const debouncedSearch = useDebouncedValue(search, 300);
  const [page, setPage] = useState(0);
  const [limit, setLimit] = useState(20);
  function setPageSize(n: number) {
    setLimit(n);
    setPage(0);
  }
  const q = useQuery({
    queryKey: ["invoices", status, debouncedSearch, page, limit],
    queryFn: () => {
      const params = new URLSearchParams({
        limit: String(limit),
        offset: String(page * limit),
      });
      if (status === "trashed") {
        params.set("trashed", "true");
      } else if (status) {
        params.set("status", status);
      }
      if (debouncedSearch.trim()) params.set("search", debouncedSearch.trim());
      return api<{
        data: {
          id: string;
          invoice_number: string;
          customer_name: string;
          total_amount: number;
          paid_amount: number;
          status: string;
          due_date: string;
          deleted_at?: string | null;
        }[];
        total: number;
      }>(`/api/invoices?${params}`);
    },
  });
  const rows = q.data?.data ?? [];
  const total = q.data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / limit));
  const trashed = status === "trashed";

  // Batch select: hapus / tandai bayar (tunai) / pulihkan (di sampah).
  const [selected, setSelected] = useState<string[]>([]);
  useEffect(() => {
    setSelected([]);
  }, [status, debouncedSearch, page]);
  const isSelected = (id: string) => selected.includes(id);
  function toggleSelect(id: string) {
    setSelected((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]));
  }
  function toggleSelectPage() {
    setSelected((prev) => {
      const pageIds = rows.map((r) => r.id);
      const allIn = pageIds.length > 0 && pageIds.every((id) => prev.includes(id));
      if (allIn) return prev.filter((id) => !pageIds.includes(id));
      return Array.from(new Set([...prev, ...pageIds]));
    });
  }
  const [batchBusy, setBatchBusy] = useState("");

  async function runInvoiceBatch(kind: "pay" | "delete" | "restore" | "purge") {
    let ids = selected;
    if (kind === "pay") {
      ids = selected.filter((id) => {
        const r = rows.find((x) => x.id === id);
        return r && ["issued", "partial", "overdue"].includes(r.status);
      });
      if (ids.length === 0) {
        void toastError("Tidak ada tagihan belum lunas yang dipilih");
        return;
      }
    }
    const label =
      kind === "pay"
        ? "Tandai lunas (tunai)"
        : kind === "delete"
          ? "Hapus ke sampah"
          : kind === "purge"
            ? "Hapus permanen"
            : "Pulihkan";
    const ok = await confirm({
      title: `${label} ${ids.length} tagihan?`,
      description:
        kind === "pay"
          ? "Mencatat pembayaran tunai penuh untuk tagihan yang dipilih."
          : kind === "delete"
            ? "Tagihan yang dipilih dipindah ke sampah (bisa dipulihkan)."
            : kind === "purge"
              ? "PERMANEN: tagihan yang dipilih hilang selamanya beserta item-nya. Hanya yang belum dibayar."
              : "Tagihan yang dipilih dikembalikan dari sampah.",
      confirmLabel: label,
      ...(kind === "purge" ? { danger: true } : {}),
    });
    if (!ok) return;
    setBatchBusy(kind);
    let done = 0;
    let firstErr = "";
    for (const id of ids) {
      try {
        if (kind === "pay") {
          await api(`/api/invoices/${id}/pay`, { method: "POST", body: JSON.stringify({ method: "tunai" }) });
        } else if (kind === "delete") {
          await api(`/api/invoices/${id}`, { method: "DELETE" });
        } else if (kind === "purge") {
          await api(`/api/invoices/${id}/purge`, { method: "DELETE" });
        } else {
          await api(`/api/invoices/${id}/restore`, { method: "POST" });
        }
        done++;
      } catch (e: unknown) {
        if (!firstErr) firstErr = e instanceof Error ? e.message : "gagal";
      }
    }
    setBatchBusy("");
    setSelected([]);
    qc.invalidateQueries({ queryKey: ["invoices"] });
    if (done === ids.length) {
      void toastSuccess(`${label}: ${done} berhasil`);
    } else {
      void toastError(`${label}: ${done}/${ids.length} berhasil${firstErr ? ` — ${firstErr}` : ""}`);
    }
  }

  const [issueOpen, setIssueOpen] = useState(false);
  const [issueErr, setIssueErr] = useState("");
  const [customerId, setCustomerId] = useState("");
  const [items, setItems] = useState<IssueItem[]>([{ description: "", quantity: 1, unit_price: 0 }]);
  const [discount, setDiscount] = useState(0);
  const [taxPct, setTaxPct] = useState(0);
  const [dueDate, setDueDate] = useState("");

  const customersQ = useQuery({
    queryKey: ["customers-lookup"],
    queryFn: () => api<{ data: CustomerLookup[] }>("/api/customers?limit=300"),
    enabled: issueOpen,
  });
  const customerOpts = (customersQ.data?.data ?? []).map((c) => ({
    value: c.id,
    label: `${c.full_name} (${c.customer_code})`,
    keywords: `${c.full_name} ${c.customer_code} ${c.phone}`,
  }));

  // Tagihan belum lunas pelanggan → item terisi otomatis (pengganti tagihan
  // yang terhapus/salah). Hanya diisi ulang saat pelanggan berganti.
  const unpaidQ = useQuery({
    queryKey: ["customer-unpaid", customerId],
    queryFn: () => api<{ data: UnpaidSource[] }>(`/api/customers/${customerId}/unpaid-invoices`),
    enabled: issueOpen && Boolean(customerId),
  });
  const filledFor = useRef("");
  const unpaidSources = unpaidQ.data?.data ?? [];
  useEffect(() => {
    if (!issueOpen || !customerId || filledFor.current === customerId) return;
    if (unpaidQ.isLoading || unpaidQ.isError || !unpaidQ.data) return;
    filledFor.current = customerId;
    const prefill: IssueItem[] = [];
    for (const src of unpaidSources) {
      for (const it of src.items) {
        prefill.push({ description: it.description, quantity: it.quantity, unit_price: it.unit_price });
      }
      if (src.items.length === 0) {
        const remaining = Math.max(0, src.invoice.total_amount - src.invoice.paid_amount);
        if (remaining > 0) {
          prefill.push({ description: `Sisa ${src.invoice.invoice_number}`, quantity: 1, unit_price: remaining });
        }
      }
    }
    if (prefill.length > 0) setItems(prefill);
  }, [issueOpen, customerId, unpaidQ.data, unpaidQ.isLoading, unpaidQ.isError, unpaidSources]);

  function openIssue() {
    setIssueErr("");
    setCustomerId("");
    filledFor.current = "";
    setItems([{ description: "", quantity: 1, unit_price: 0 }]);
    setDiscount(0);
    setTaxPct(0);
    setDueDate("");
    setIssueOpen(true);
  }

  const subtotal = items.reduce((s, it) => s + Math.max(0, Math.floor(Number(it.quantity) || 0)) * Math.max(0, Math.floor(Number(it.unit_price) || 0)), 0);
  const discountClamped = Math.max(0, Math.min(subtotal, Math.floor(Number(discount) || 0)));
  const tax = Math.round((subtotal - discountClamped) * Math.max(0, Number(taxPct) || 0) / 100);
  const grandTotal = subtotal - discountClamped + tax;

  function closeIssue() {
    setIssueOpen(false);
    setIssueErr("");
  }

  const issueInvoice = useMutation({
    mutationFn: () =>
      api<{ invoice_number: string; total_amount: number; whatsapp_queued?: boolean }>("/api/invoices", {
        method: "POST",
        body: JSON.stringify({
          customer_id: customerId,
          due_date: dueDate || undefined,
          discount_amount: discountClamped || undefined,
          tax_percent: taxPct || undefined,
          items: items.map((it) => ({
            description: it.description.trim(),
            quantity: Math.max(1, Math.floor(Number(it.quantity) || 1)),
            unit_price: Math.max(0, Math.floor(Number(it.unit_price) || 0)),
          })),
        }),
      }),
    onSuccess: (res) => {
      closeIssue();
      qc.invalidateQueries({ queryKey: ["invoices"] });
      void toastSuccess(
        res.whatsapp_queued
          ? `Tagihan ${res.invoice_number} diterbitkan (${formatRp(res.total_amount)}). Notifikasi WhatsApp diantrikan.`
          : `Tagihan ${res.invoice_number} diterbitkan (${formatRp(res.total_amount)})`,
      );
    },
    onError: (e: Error) => {
      setIssueErr(e.message);
      void toastError(e.message);
    },
  });

  async function exportCsv() {
    const ok = await confirm({
      title: "Export tagihan",
      description: "Unduh data tagihan (mengikuti filter & pencarian aktif) sebagai CSV?",
      confirmLabel: "Unduh",
    });
    if (!ok) return;
    try {
      const params = new URLSearchParams();
      if (status && status !== "trashed") params.set("status", status);
      if (debouncedSearch.trim()) params.set("search", debouncedSearch.trim());
      const qs = params.toString();
      await apiDownload(`/api/reports/invoices.csv${qs ? `?${qs}` : ""}`, "invoices.csv");
    } catch (e: unknown) {
      void toastError(e instanceof Error ? e.message : "Export gagal");
    }
  }

  return (
    <Section
      title="Tagihan"
      actions={
        <button type="button" className="btn" onClick={openIssue}>
          + Terbitkan tagihan
        </button>
      }
    >
      <p className="mb-3 text-sm text-[var(--muted)]">
        {status === "trashed"
          ? "Tagihan di sampah. Pulihkan jika terhapus karena kesalahan. Portal pelanggan tidak menampilkan item ini."
          : "Hapus memindahkan tagihan ke sampah (filter Sampah), bukan menghapus permanen."}
      </p>
      <ListToolbar
        search={search}
        onSearchChange={(v) => {
          setSearch(v);
          setPage(0);
        }}
        searchPlaceholder="No. tagihan atau nama pelanggan…"
        filters={[
          {
            key: "status",
            label: "Status",
            value: status,
            onChange: (v) => {
              setStatus(v);
              setPage(0);
            },
            options: [
              { value: "issued", label: "Belum bayar" },
              { value: "partial", label: "Bayar sebagian" },
              { value: "overdue", label: "Jatuh tempo" },
              { value: "paid", label: "Sudah bayar" },
              { value: "trashed", label: "Sampah" },
            ],
          },
        ]}
        page={page}
        pageCount={pages}
        onPageChange={setPage}
        total={total}
        pageSize={limit}
        onPageSizeChange={setPageSize}
      >
        <Button type="button" variant="outline" onClick={() => void exportCsv()}>
          Export CSV
        </Button>
        <label className="flex cursor-pointer items-center gap-1.5 text-sm text-[var(--muted)]">
          <input
            type="checkbox"
            checked={rows.length > 0 && rows.every((r) => selected.includes(r.id))}
            onChange={toggleSelectPage}
            title="Pilih semua di halaman ini"
          />
          Pilih halaman
        </label>
      </ListToolbar>
      {selected.length > 0 ? (
        <div className="mb-3 flex flex-wrap items-center gap-2 rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--panel)] px-3 py-2 text-sm">
          <strong>{selected.length} dipilih</strong>
          {!trashed ? (
            <>
              <Button
                type="button"
                size="sm"
                disabled={batchBusy !== ""}
                onClick={() => void runInvoiceBatch("pay")}
              >
                {batchBusy === "pay" ? "Memproses…" : "Tandai bayar"}
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={batchBusy !== ""}
                onClick={() => void runInvoiceBatch("delete")}
              >
                {batchBusy === "delete" ? "Memproses…" : "Hapus"}
              </Button>
            </>
          ) : (
            <>
              <Button
                type="button"
                size="sm"
                disabled={batchBusy !== ""}
                onClick={() => void runInvoiceBatch("restore")}
              >
                {batchBusy === "restore" ? "Memproses…" : "Pulihkan"}
              </Button>
              <Button
                type="button"
                variant="destructive"
                size="sm"
                disabled={batchBusy !== ""}
                onClick={() => void runInvoiceBatch("purge")}
              >
                {batchBusy === "purge" ? "Memproses…" : "Hapus permanen"}
              </Button>
            </>
          )}
          <Button type="button" variant="ghost" size="sm" disabled={batchBusy !== ""} onClick={() => setSelected([])}>
            Batal
          </Button>
        </div>
      ) : null}
      <Table
        rowNumberStart={page * limit + 1}
        columns={["", "Nomor", "Pelanggan", "Jatuh tempo", "Total", "Terbayar", "Status", "Aksi"]}
        rows={rows.map((i) => [
          <input
            key={`sel-${i.id}`}
            type="checkbox"
            checked={isSelected(i.id)}
            onChange={() => toggleSelect(i.id)}
            onClick={(e) => e.stopPropagation()}
            title={`Pilih ${i.invoice_number}`}
          />,
          i.invoice_number,
          i.customer_name,
          i.due_date ? new Date(i.due_date).toLocaleDateString("id-ID") : "—",
          formatRp(i.total_amount),
          formatRp(i.paid_amount ?? 0),
          invoiceStatusLabel(i.status),
          <InvoiceActions
            key={i.id}
            id={i.id}
            invoiceNumber={i.invoice_number}
            status={i.status}
            trashed={status === "trashed"}
            onDone={() => qc.invalidateQueries({ queryKey: ["invoices"] })}
          />,
        ])}
      />

      <FormDialog open={issueOpen} wide title="Terbitkan tagihan manual" onClose={closeIssue}>
        <form
          className="flex flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            issueInvoice.mutate();
          }}
        >
          <div className="flex min-w-0 flex-col gap-1.5">
            <Label htmlFor="issue-customer">Pelanggan</Label>
            <SearchableSelect
              id="issue-customer"
              required
              placeholder="— Pilih pelanggan —"
              searchPlaceholder="Cari nama, kode, atau HP…"
              emptyText={customersQ.isLoading ? "Memuat…" : "Tidak ada hasil"}
              value={customerId}
              onValueChange={setCustomerId}
              options={customerOpts}
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label>Item tagihan</Label>
            {customerId && unpaidQ.isLoading ? (
              <p className="text-xs text-[var(--muted)]">Memeriksa tagihan belum lunas pelanggan…</p>
            ) : unpaidSources.length > 0 ? (
              <p className="text-xs text-[var(--muted)]">
                Otomatis dari {unpaidSources.length} tagihan belum lunas:{" "}
                {unpaidSources
                  .map(
                    (s) =>
                      `${s.invoice.invoice_number} (sisa ${formatRp(Math.max(0, s.invoice.total_amount - s.invoice.paid_amount))})`,
                  )
                  .join(", ")}
                . Bebas diubah/dihapus.
                {unpaidSources.some((s) => s.invoice.paid_amount > 0)
                  ? " Sebagian sudah dibayar — sesuaikan nominal bila perlu."
                  : null}
              </p>
            ) : null}
            {items.map((it, idx) => (
              <div key={idx} className="grid grid-cols-[1fr_72px_130px_auto] items-center gap-2">
                <Input
                  placeholder={`Item ${idx + 1} (mis. Biaya instalasi)`}
                  value={it.description}
                  onChange={(e) => setItems(items.map((x, i) => (i === idx ? { ...x, description: e.target.value } : x)))}
                  required
                />
                <Input
                  type="number"
                  min={1}
                  title="Qty"
                  placeholder="Qty"
                  value={it.quantity}
                  onChange={(e) => setItems(items.map((x, i) => (i === idx ? { ...x, quantity: Number(e.target.value) } : x)))}
                  required
                />
                <Input
                  type="number"
                  min={0}
                  title="Harga satuan (Rp)"
                  placeholder="Harga (Rp)"
                  value={it.unit_price}
                  onChange={(e) => setItems(items.map((x, i) => (i === idx ? { ...x, unit_price: Number(e.target.value) } : x)))}
                  required
                />
                <button
                  type="button"
                  className="btn-ghost px-2"
                  title="Hapus item"
                  disabled={items.length <= 1}
                  onClick={() => setItems(items.filter((_, i) => i !== idx))}
                >
                  ✕
                </button>
              </div>
            ))}
            <button
              type="button"
              className="btn-ghost self-start"
              onClick={() => setItems([...items, { description: "", quantity: 1, unit_price: 0 }])}
            >
              + Tambah item
            </button>
          </div>

          <div className="grid gap-4 sm:grid-cols-3">
            <div className="flex min-w-0 flex-col gap-1.5">
              <Label htmlFor="issue-discount">Diskon (Rp)</Label>
              <Input
                id="issue-discount"
                type="number"
                min={0}
                value={discount}
                onChange={(e) => setDiscount(Number(e.target.value))}
              />
            </div>
            <div className="flex min-w-0 flex-col gap-1.5">
              <Label htmlFor="issue-tax">Pajak (%)</Label>
              <Input
                id="issue-tax"
                type="number"
                min={0}
                max={100}
                value={taxPct}
                onChange={(e) => setTaxPct(Number(e.target.value))}
              />
            </div>
            <div className="flex min-w-0 flex-col gap-1.5">
              <Label htmlFor="issue-due">Jatuh tempo</Label>
              <Input id="issue-due" type="date" value={dueDate} onChange={(e) => setDueDate(e.target.value)} />
              <p className="text-xs text-[var(--muted)]">Kosong = ikut pengaturan umum.</p>
            </div>
          </div>

          <div className="rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--panel-muted)]/40 px-3 py-2 text-sm">
            <div className="flex justify-between text-[var(--muted)]">
              <span>Subtotal</span>
              <span>{formatRp(subtotal)}</span>
            </div>
            {discountClamped > 0 && (
              <div className="flex justify-between text-[var(--muted)]">
                <span>Diskon</span>
                <span>−{formatRp(discountClamped)}</span>
              </div>
            )}
            {tax > 0 && (
              <div className="flex justify-between text-[var(--muted)]">
                <span>Pajak</span>
                <span>{formatRp(tax)}</span>
              </div>
            )}
            <div className="mt-1 flex justify-between border-t border-[var(--border)] pt-1 font-bold">
              <span>Total</span>
              <span>{formatRp(grandTotal)}</span>
            </div>
          </div>

          <div className="flex flex-wrap gap-2">
            <Button type="submit" disabled={issueInvoice.isPending || !customerId || grandTotal <= 0}>
              {issueInvoice.isPending ? "Menerbitkan…" : "Terbitkan"}
            </Button>
            <Button type="button" variant="secondary" onClick={closeIssue}>
              Batal
            </Button>
          </div>
          <p className="text-xs text-[var(--muted)]">
            Jika nomor WA pelanggan terisi, notifikasi tagihan baru diantrikan otomatis (template “Tagihan baru”).
          </p>
          {issueErr && <p className="text-sm text-[var(--danger)]">{issueErr}</p>}
        </form>
      </FormDialog>
    </Section>
  );
}

