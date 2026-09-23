import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { formatDateTime } from "../tenantTime";
import { ListToolbar, useDebouncedValue } from "../ListToolbar";
import { IconTrash, IconUndo } from "../icons";
import { useAppDialog } from "../confirm";
import { toastError, toastSuccess } from "../swal";
import { formatRp, IconButton, paymentStatusLabel, Section, Table } from "../ui";
import { paymentMethodLabel } from "../payMethod";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

type PaymentRow = {
  id: string;
  customer_name?: string;
  customer_code?: string;
  invoice_number?: string;
  amount: number;
  method: string;
  sandbox?: boolean;
  reference?: string | null;
  status: string;
  paid_at?: string | null;
  created_at: string;
  deleted_at?: string | null;
};

type WalletTopupRow = {
  id: string;
  customer_id: string;
  customer_name?: string;
  customer_code?: string;
  amount: number;
  type: string;
  reference?: string;
  description?: string;
  created_at: string;
};

const TOPUP_TYPE_LABEL: Record<string, string> = {
  topup: "Topup online",
  topup_admin: "Topup admin",
};

function paymentWhen(p: PaymentRow) {
  const raw = p.paid_at || p.created_at;
  return formatDateTime(raw);
}

function customerLabel(p: PaymentRow) {
  const name = (p.customer_name || "").trim();
  const code = (p.customer_code || "").trim();
  if (name && code) return `${name} (${code})`;
  return name || code || "—";
}

function topupCustomerLabel(t: WalletTopupRow) {
  const name = (t.customer_name || "").trim();
  const code = (t.customer_code || "").trim();
  if (name && code) return `${name} (${code})`;
  return name || code || "—";
}

export function PaymentsPage() {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();
  const [search, setSearch] = useState("");
  const debouncedSearch = useDebouncedValue(search, 300);
  const [trashed, setTrashed] = useState(false);
  const [page, setPage] = useState(0);
  const [limit, setLimit] = useState(20);
  const [tab, setTab] = useState<"payments" | "topups">("payments");

  const featuresQ = useQuery({
    queryKey: ["features"],
    queryFn: () => api<{ wallet_enabled: boolean; wallet_min_topup: number }>("/api/features"),
  });
  const walletEnabled = Boolean(featuresQ.data?.wallet_enabled);
  const activeTab: "payments" | "topups" = walletEnabled ? tab : "payments";

  function setPageSize(n: number) {
    setLimit(n);
    setPage(0);
  }

  const q = useQuery({
    queryKey: ["payments", debouncedSearch, trashed, page, limit],
    queryFn: () => {
      const params = new URLSearchParams({
        limit: String(limit),
        offset: String(page * limit),
      });
      if (debouncedSearch.trim()) params.set("search", debouncedSearch.trim());
      if (trashed) params.set("trashed", "true");
      return api<{ data: PaymentRow[]; total: number }>(`/api/payments?${params}`);
    },
  });

  const topupQ = useQuery({
    queryKey: ["wallet-topups", debouncedSearch, page, limit],
    enabled: walletEnabled && activeTab === "topups",
    queryFn: () => {
      const params = new URLSearchParams({
        limit: String(limit),
        offset: String(page * limit),
      });
      if (debouncedSearch.trim()) params.set("search", debouncedSearch.trim());
      return api<{ data: WalletTopupRow[]; total: number }>(`/api/wallet/topups?${params}`);
    },
  });

  const remove = useMutation({
    mutationFn: (id: string) => api(`/api/payments/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      void toastSuccess("Pembayaran dipindah ke sampah");
      void qc.invalidateQueries({ queryKey: ["payments"] });
      void qc.invalidateQueries({ queryKey: ["invoices"] });
      void qc.invalidateQueries({ queryKey: ["invoices-recent"] });
    },
    onError: (e: Error) => void toastError(e.message || "Gagal hapus pembayaran"),
  });

  const restore = useMutation({
    mutationFn: (id: string) => api(`/api/payments/${id}/restore`, { method: "POST" }),
    onSuccess: () => {
      void toastSuccess("Pembayaran dipulihkan");
      void qc.invalidateQueries({ queryKey: ["payments"] });
      void qc.invalidateQueries({ queryKey: ["invoices"] });
      void qc.invalidateQueries({ queryKey: ["invoices-recent"] });
    },
    onError: (e: Error) => void toastError(e.message || "Gagal pulihkan pembayaran"),
  });

  function renderPayments() {
    const rows = q.data?.data ?? [];
    const total = q.data?.total ?? 0;
    const pages = Math.max(1, Math.ceil(total / limit));
    return (
      <div>
        <p className="mb-3 text-sm text-[var(--muted)]">
          {trashed
            ? "Riwayat di sampah. Pulihkan jika terhapus karena kesalahan. Portal pelanggan tidak menampilkan item ini."
            : "Log pembayaran pelanggan (tunai, transfer, dan payment gateway). Hapus memindahkan ke sampah (filter Sampah) dan menghitung ulang status tagihan."}
        </p>
        <ListToolbar
          search={search}
          onSearchChange={(v) => {
            setSearch(v);
            setPage(0);
          }}
          searchPlaceholder="Nama, kode, nomor tagihan, atau metode…"
          filters={[
            {
              key: "view",
              label: "Tampilan",
              value: trashed ? "trashed" : "",
              onChange: (v) => {
                setTrashed(v === "trashed");
                setPage(0);
              },
              options: [{ value: "trashed", label: "Sampah" }],
            },
          ]}
          page={page}
          pageCount={pages}
          onPageChange={setPage}
          total={total}
          pageSize={limit}
          onPageSizeChange={setPageSize}
        />
        <Table
          rowNumberStart={page * limit + 1}
          columns={["Tanggal", "Pelanggan", "Tagihan", "Metode", "Jumlah", "Status", "Aksi"]}
          rows={rows.map((p) => [
            paymentWhen(p),
            customerLabel(p),
            p.invoice_number || "—",
            paymentMethodLabel(p.method, p.sandbox),
            formatRp(p.amount),
            paymentStatusLabel(p.status),
            trashed ? (
              <IconButton
                key={p.id}
                label="Pulihkan pembayaran"
                disabled={restore.isPending}
                onClick={async () => {
                  const ok = await confirm({
                    title: "Pulihkan pembayaran",
                    description: `Kembalikan pembayaran ${formatRp(p.amount)} untuk ${customerLabel(p)}${p.invoice_number ? ` (tagihan ${p.invoice_number})` : ""}?`,
                    confirmLabel: "Pulihkan",
                  });
                  if (!ok) return;
                  restore.mutate(p.id);
                }}
              >
                <IconUndo />
              </IconButton>
            ) : (
              <IconButton
                key={p.id}
                label="Hapus pembayaran"
                danger
                disabled={remove.isPending}
                onClick={async () => {
                  const who = customerLabel(p);
                  const inv = p.invoice_number ? ` tagihan ${p.invoice_number}` : "";
                  const ok = await confirm({
                    title: "Hapus riwayat pembayaran",
                    description: `Pindahkan pembayaran ${formatRp(p.amount)} untuk ${who}${inv} ke sampah? Status tagihan akan dihitung ulang. Bisa dipulihkan dari filter Sampah.`,
                    confirmLabel: "Hapus",
                  });
                  if (!ok) return;
                  remove.mutate(p.id);
                }}
              >
                <IconTrash />
              </IconButton>
            ),
          ])}
        />
      </div>
    );
  }

  function renderTopups() {
    const rows = topupQ.data?.data ?? [];
    const total = topupQ.data?.total ?? 0;
    const pages = Math.max(1, Math.ceil(total / limit));
    return (
      <div>
        <p className="mb-3 text-sm text-[var(--muted)]">
          Riwayat pelanggan yang mengisi saldo (topup online via payment gateway maupun topup manual admin).
        </p>
        <ListToolbar
          search={search}
          onSearchChange={(v) => {
            setSearch(v);
            setPage(0);
          }}
          searchPlaceholder="Nama, kode, referensi, atau keterangan…"
          page={page}
          pageCount={pages}
          onPageChange={setPage}
          total={total}
          pageSize={limit}
          onPageSizeChange={setPageSize}
        />
        <Table
          rowNumberStart={page * limit + 1}
          columns={["Tanggal", "Pelanggan", "Jenis", "Keterangan", "Jumlah"]}
          rows={rows.map((t) => [
            formatDateTime(t.created_at),
            topupCustomerLabel(t),
            TOPUP_TYPE_LABEL[t.type] || t.type,
            t.description || t.reference || "—",
            formatRp(t.amount),
          ])}
        />
      </div>
    );
  }

  return (
    <Section title="Pembayaran">
      {walletEnabled ? (
        <Tabs
          value={activeTab}
          onValueChange={(v) => {
            setTab(v as "payments" | "topups");
            setPage(0);
            setSearch("");
          }}
        >
          <TabsList aria-label="Pembayaran">
            <TabsTrigger value="payments">Pembayaran</TabsTrigger>
            <TabsTrigger value="topups">Topup Saldo</TabsTrigger>
          </TabsList>
          <TabsContent value="payments">{renderPayments()}</TabsContent>
          <TabsContent value="topups">{renderTopups()}</TabsContent>
        </Tabs>
      ) : (
        renderPayments()
      )}
    </Section>
  );
}
