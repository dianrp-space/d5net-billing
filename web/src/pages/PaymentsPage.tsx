import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { ListToolbar, useDebouncedValue } from "../ListToolbar";
import { IconTrash, IconUndo } from "../icons";
import { useAppDialog } from "../confirm";
import { toastError, toastSuccess } from "../swal";
import { formatRp, IconButton, paymentStatusLabel, Section, Table } from "../ui";
import { paymentMethodLabel } from "../payMethod";

type PaymentRow = {
  id: string;
  customer_name?: string;
  customer_code?: string;
  invoice_number?: string;
  amount: number;
  method: string;
  reference?: string | null;
  status: string;
  paid_at?: string | null;
  created_at: string;
  deleted_at?: string | null;
};

function paymentWhen(p: PaymentRow) {
  const raw = p.paid_at || p.created_at;
  return raw ? new Date(raw).toLocaleString("id-ID") : "—";
}

function customerLabel(p: PaymentRow) {
  const name = (p.customer_name || "").trim();
  const code = (p.customer_code || "").trim();
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
  const limit = 20;
  const q = useQuery({
    queryKey: ["payments", debouncedSearch, trashed, page],
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
  const rows = q.data?.data ?? [];
  const total = q.data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / limit));

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

  return (
    <Section title="Pembayaran">
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
      />
      <Table
        columns={["Tanggal", "Pelanggan", "Tagihan", "Metode", "Jumlah", "Status", "Aksi"]}
        rows={rows.map((p) => [
          paymentWhen(p),
          customerLabel(p),
          p.invoice_number || "—",
          paymentMethodLabel(p.method),
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
    </Section>
  );
}
