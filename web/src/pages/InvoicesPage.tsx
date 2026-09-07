import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, apiDownload } from "../api";
import { ListToolbar, useDebouncedValue } from "../ListToolbar";
import { useAppDialog } from "../confirm";
import { toastError } from "../swal";
import { formatRp, Section, Table, Button } from "../ui";
import { InvoiceActions } from "../AdminExtra";

export function InvoicesPage() {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();
  const [status, setStatus] = useState("");
  const [search, setSearch] = useState("");
  const debouncedSearch = useDebouncedValue(search, 300);
  const [page, setPage] = useState(0);
  const limit = 20;
  const q = useQuery({
    queryKey: ["invoices", status, debouncedSearch, page],
    queryFn: () => {
      const params = new URLSearchParams({
        limit: String(limit),
        offset: String(page * limit),
      });
      if (status) params.set("status", status);
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
        }[];
        total: number;
      }>(`/api/invoices?${params}`);
    },
  });
  const rows = q.data?.data ?? [];
  const total = q.data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / limit));

  async function exportCsv() {
    const ok = await confirm({
      title: "Export tagihan",
      description: "Unduh data tagihan (mengikuti filter & pencarian aktif) sebagai CSV?",
      confirmLabel: "Unduh",
    });
    if (!ok) return;
    try {
      const params = new URLSearchParams();
      if (status) params.set("status", status);
      if (debouncedSearch.trim()) params.set("search", debouncedSearch.trim());
      const qs = params.toString();
      await apiDownload(`/api/reports/invoices.csv${qs ? `?${qs}` : ""}`, "invoices.csv");
    } catch (e: unknown) {
      void toastError(e instanceof Error ? e.message : "Export gagal");
    }
  }

  return (
    <Section title="Tagihan">
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
              { value: "issued", label: "Issued" },
              { value: "partial", label: "Partial" },
              { value: "overdue", label: "Overdue" },
              { value: "paid", label: "Paid" },
            ],
          },
        ]}
        page={page}
        pageCount={pages}
        onPageChange={setPage}
        total={total}
      >
        <Button type="button" variant="outline" onClick={() => void exportCsv()}>
          Export CSV
        </Button>
      </ListToolbar>
      <Table
        columns={["Nomor", "Pelanggan", "Jatuh tempo", "Total", "Terbayar", "Status", "Aksi"]}
        rows={rows.map((i) => [
          i.invoice_number,
          i.customer_name,
          i.due_date ? new Date(i.due_date).toLocaleDateString("id-ID") : "—",
          formatRp(i.total_amount),
          formatRp(i.paid_amount ?? 0),
          i.status,
          <InvoiceActions
            key={i.id}
            id={i.id}
            invoiceNumber={i.invoice_number}
            status={i.status}
            onDone={() => qc.invalidateQueries({ queryKey: ["invoices"] })}
          />,
        ])}
      />
    </Section>
  );
}

