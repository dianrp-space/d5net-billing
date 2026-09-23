import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import ReactEChartsCore from "echarts-for-react/lib/core";
import echarts from "./echarts";
import { api, apiDownload, getToken } from "./api";
import { formatDate, formatDateTime } from "./tenantTime";
import { useAppDialog } from "./confirm";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { IconBanknote, IconBan, IconCheck, IconDownload, IconPencil, IconTicket, IconTrash, IconUndo, IconZap } from "./icons";
import { ListToolbar, matchesQuery, useDebouncedValue } from "./ListToolbar";
import { toastError, toastSuccess } from "./swal";
import { PAY_METHOD_TUNAI, payOptionsHasDuitkuSandbox, type PayOption } from "./payMethod";
import { usePersistedTab } from "./navPersist";
import {
  Button,
  Card,
  FormDialog,
  IconButton,
  Input,
  Label,
  formatRp,
  SearchableSelect,
  Section,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Table,
} from "./ui";

export type ResellerOpt = { id: string; name: string; phone?: string | null; balance?: number; commission_percent?: number; is_active?: boolean };
export type StaffOpt = { user_id: string; full_name: string; email: string; is_active: boolean; role_slug?: string };

/** Pilih Reseller ATAU Sales (saling eksklusif). */
export function AttributionSelects({
  resellerId,
  salesUserId,
  onChange,
  resellers,
  users,
}: {
  resellerId: string;
  salesUserId: string;
  onChange: (next: { reseller_id: string; sales_user_id: string }) => void;
  resellers: ResellerOpt[];
  users: StaffOpt[];
}) {
  const activeUsers = users.filter((u) => u.is_active);
  return (
    <div className="grid gap-3 sm:grid-cols-2 sm:col-span-2">
      <div>
        <Label className="mb-1.5 block">Reseller (opsional)</Label>
        <Select
          value={resellerId || "__none__"}
          onValueChange={(v) => {
            const id = v === "__none__" ? "" : v;
            onChange({ reseller_id: id, sales_user_id: id ? "" : salesUserId });
          }}
        >
          <SelectTrigger>
            <SelectValue placeholder="Reseller" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__none__">— Tidak ada —</SelectItem>
            {resellers.map((r) => (
              <SelectItem key={r.id} value={r.id}>
                {r.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div>
        <Label className="mb-1.5 block">Sales (opsional)</Label>
        <Select
          value={salesUserId || "__none__"}
          onValueChange={(v) => {
            const id = v === "__none__" ? "" : v;
            onChange({ reseller_id: id ? "" : resellerId, sales_user_id: id });
          }}
          disabled={Boolean(resellerId)}
        >
          <SelectTrigger>
            <SelectValue placeholder="Sales" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__none__">— Tidak ada —</SelectItem>
            {activeUsers.map((u) => (
              <SelectItem key={u.user_id} value={u.user_id}>
                {u.full_name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {resellerId ? (
          <p className="mt-1 text-xs text-[var(--muted)]">Kosongkan reseller untuk memilih sales.</p>
        ) : (
          <p className="mt-1 text-xs text-[var(--muted)]">Salah satu: reseller atau sales (semua user aktif).</p>
        )}
      </div>
    </div>
  );
}

export const COMMISSION_BASIS_OPTIONS = [
  { value: "new_customer_flat", label: "Pelanggan baru" },
  { value: "acquisition_flat", label: "Akuisisi" },
] as const;

export function commissionBasisLabel(basis: string) {
  return COMMISSION_BASIS_OPTIONS.find((o) => o.value === basis)?.label || basis || "—";
}

/** Jenis komisi flat (pelanggan baru vs akuisisi). */
export function CommissionBasisSelect({
  value,
  onChange,
  className,
}: {
  value: string;
  onChange: (v: string) => void;
  className?: string;
}) {
  return (
    <div className={className ?? "sm:col-span-2"}>
      <Label className="mb-1.5 block">Jenis komisi</Label>
      <Select value={value || "new_customer_flat"} onValueChange={onChange}>
        <SelectTrigger>
          <SelectValue placeholder="Jenis komisi" />
        </SelectTrigger>
        <SelectContent>
          {COMMISSION_BASIS_OPTIONS.map((o) => (
            <SelectItem key={o.value} value={o.value}>
              {o.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <p className="mt-1 text-xs text-[var(--muted)]">
        Nominal diambil dari setting Reseller &amp; Komisi sesuai jenis ini.
      </p>
    </div>
  );
}

export function AlertsPanel() {
  const q = useQuery({
    queryKey: ["alerts-panel"],
    queryFn: () => api<{ data: { severity: string; title: string; message: string }[] }>("/api/alerts?limit=8"),
    refetchInterval: 15000,
  });
  const rows = Array.isArray(q.data?.data) ? q.data.data : [];
  return (
    <div className="mb-2">
      <h2 className="eyebrow mb-3">Alert realtime</h2>
      <Table
        columns={["Severity", "Judul", "Pesan"]}
        rows={rows.slice(0, 8).map((a) => [a.severity, a.title, a.message])}
      />
    </div>
  );
}

export function useDashboardSSE() {
  const [live, setLive] = useState<Record<string, unknown> | null>(null);
  useEffect(() => {
    const token = getToken();
    let tid = "0";
    if (token) {
      try {
        const payload = JSON.parse(atob(token.split(".")[1] || "")) as { tid?: string };
        if (payload.tid) tid = String(payload.tid);
      } catch {
        /* ignore */
      }
    }
    const es = new EventSource(`/events/stream?tenant_id=${tid}${token ? `&access_token=${encodeURIComponent(token)}` : ""}`);
    es.onmessage = (ev) => {
      try {
        setLive(JSON.parse(ev.data) as Record<string, unknown>);
      } catch {
        /* ignore */
      }
    };
    return () => es.close();
  }, []);
  return live;
}

type CustomerPaymentRow = {
  date: string;
  customer_id: string;
  customer_code: string;
  customer_name: string;
  invoice_number?: string;
  category: string;
  method: string;
  reference?: string;
  sandbox: boolean;
  amount: number;
};

type CustomerPaymentReport = {
  from: string;
  to: string;
  total_count: number;
  total_amount: number;
  data: CustomerPaymentRow[];
  by_customer: { customer_id: string; customer_code: string; customer_name: string; count: number; amount: number }[];
  by_category: { category: string; count: number; amount: number }[];
};

type PnLLine = { code: string; name: string; amount: number };
type PnLReport = {
  from: string;
  to: string;
  period: string;
  revenue: number;
  expense: number;
  profit: number;
  revenue_accounts: PnLLine[];
  expense_accounts: PnLLine[];
};

type TrialBalanceRow = {
  account_id: string;
  code: string;
  name: string;
  type: string;
  debit: number;
  credit: number;
  balance: number;
};
type TrialBalance = { from: string; to: string; rows: TrialBalanceRow[]; total_debit: number; total_credit: number };

type BalanceSheetLine = { code: string; name: string; amount: number };
type BalanceSheetReport = {
  from: string;
  to: string;
  assets: BalanceSheetLine[];
  liabilities: BalanceSheetLine[];
  equity: BalanceSheetLine[];
  total_assets: number;
  total_liabilities: number;
  total_equity: number;
  current_earnings: number;
  total_liabilities_equity: number;
  balanced: boolean;
};

type JournalEntryView = {
  id: string;
  entry_date: string;
  reference?: string;
  description: string;
  source_type?: string;
  total_debit: number;
  total_credit: number;
  lines: { account_id: string; account_code: string; account_name: string; account_type: string; debit: number; credit: number }[];
};

type AccountLedger = {
  account_id: string;
  account_code: string;
  account_name: string;
  account_type: string;
  from: string;
  to: string;
  opening: number;
  lines: { entry_id: string; entry_date: string; reference?: string; description: string; debit: number; credit: number; balance: number }[];
  total_debit: number;
  total_credit: number;
  closing: number;
};

const PAYMENT_CATEGORY_OPTIONS = ["Langganan", "Instalasi", "Denda", "Pajak", "Lainnya"];

function monthStartISO(): string {
  const d = new Date();
  return new Date(d.getFullYear(), d.getMonth(), 1).toISOString().slice(0, 10);
}

function todayISO(): string {
  return new Date().toISOString().slice(0, 10);
}

export function AccountingPage() {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();
  const [tab, setTab] = usePersistedTab("accounting", "ringkasan", [
    "ringkasan",
    "beban",
    "pemasukan",
    "jurnal",
    "neraca_saldo",
    "neraca",
    "coa",
  ] as const);
  const [expOpen, setExpOpen] = useState(false);
  const [payFrom, setPayFrom] = useState(monthStartISO);
  const [payTo, setPayTo] = useState(todayISO);
  const [paySearch, setPaySearch] = useState("");
  const [payCategory, setPayCategory] = useState("__all__");
  const [paySandbox, setPaySandbox] = useState(false);
  const paySearchDebounced = useDebouncedValue(paySearch, 300);
  const [finFrom, setFinFrom] = useState(monthStartISO);
  const [finTo, setFinTo] = useState(todayISO);
  const [jrnAccount, setJrnAccount] = useState("__all__");
  const [jrnSearch, setJrnSearch] = useState("");
  const jrnSearchDebounced = useDebouncedValue(jrnSearch, 300);
  const [jrnOffset, setJrnOffset] = useState(0);
  const [exp, setExp] = useState({
    amount: "",
    category: "ops",
    description: "",
    expense_date: new Date().toISOString().slice(0, 10),
  });
  const [expErr, setExpErr] = useState("");

  const pnl = useQuery({
    queryKey: ["pnl", finFrom, finTo],
    queryFn: () => api<PnLReport>(`/api/accounting/pnl?from=${finFrom}&to=${finTo}`),
  });
  const trialBalance = useQuery({
    queryKey: ["trial-balance", finFrom, finTo],
    queryFn: () => api<TrialBalance>(`/api/accounting/trial-balance?from=${finFrom}&to=${finTo}`),
    enabled: tab === "neraca_saldo",
  });
  const balanceSheet = useQuery({
    queryKey: ["balance-sheet", finFrom, finTo],
    queryFn: () => api<BalanceSheetReport>(`/api/accounting/balance-sheet?from=${finFrom}&to=${finTo}`),
    enabled: tab === "neraca",
  });
  const journal = useQuery({
    queryKey: ["journal", finFrom, finTo, jrnAccount, jrnSearchDebounced, jrnOffset],
    queryFn: () => {
      const p = new URLSearchParams({ from: finFrom, to: finTo, limit: "50", offset: String(jrnOffset) });
      if (jrnAccount !== "__all__") p.set("account_id", jrnAccount);
      if (jrnSearchDebounced.trim()) p.set("search", jrnSearchDebounced.trim());
      return api<{ data: JournalEntryView[]; total: number }>(`/api/accounting/journal?${p.toString()}`);
    },
    enabled: tab === "jurnal" && jrnAccount === "__all__",
  });
  const ledger = useQuery({
    queryKey: ["ledger", finFrom, finTo, jrnAccount],
    queryFn: () =>
      api<AccountLedger>(`/api/accounting/ledger?account_id=${jrnAccount}&from=${finFrom}&to=${finTo}`),
    enabled: tab === "jurnal" && jrnAccount !== "__all__",
  });
  const cf = useQuery({
    queryKey: ["cashflow"],
    queryFn: () => api<{ month: string; inflow: number; outflow: number; net: number }[]>("/api/accounting/cashflow?months=6"),
  });
  const business = useQuery({
    queryKey: ["business-report"],
    queryFn: () =>
      api<{
        mrr?: number;
        arpu?: number;
        unpaid_invoices?: number;
        active_customers?: number;
        aging?: { current?: number; d30?: number; d60?: number; d90?: number };
      }>("/api/reports/business"),
  });
  const churn = useQuery({
    queryKey: ["churn"],
    queryFn: () =>
      api<{
        canceled_30d?: number;
        dismantled_30d?: number;
        active?: number;
        churn_ratio?: number;
        window_days?: number;
      }>("/api/reports/churn"),
  });
  const expensesQ = useQuery({
    queryKey: ["expenses"],
    queryFn: () =>
      api<{ data: { id: string; amount: number; category: string; description?: string | null; expense_date: string }[]; total: number }>(
        "/api/accounting/expenses?limit=50",
      ),
  });
  const accountsQ = useQuery({
    queryKey: ["accounts"],
    queryFn: () => api<{ id: string; code: string; name: string; type: string }[]>("/api/accounting/accounts"),
  });

  const payParams = useMemo(() => {
    const p = new URLSearchParams();
    if (payFrom) p.set("from", payFrom);
    if (payTo) p.set("to", payTo);
    if (paySearchDebounced.trim()) p.set("search", paySearchDebounced.trim());
    if (payCategory !== "__all__") p.set("category", payCategory);
    if (paySandbox) p.set("sandbox", "include");
    return p;
  }, [payFrom, payTo, paySearchDebounced, payCategory, paySandbox]);

  const payReport = useQuery({
    queryKey: ["customer-payments", payParams.toString()],
    queryFn: () => api<CustomerPaymentReport>(`/api/reports/customer-payments?${payParams.toString()}`),
    enabled: tab === "pemasukan",
  });

  const createExpense = useMutation({
    mutationFn: () => {
      const amount = Math.floor(Number(exp.amount) || 0);
      if (amount <= 0) throw new Error("Nominal beban harus lebih dari 0");
      return api("/api/accounting/expenses", {
        method: "POST",
        body: JSON.stringify({
          amount,
          category: exp.category,
          description: exp.description.trim() || undefined,
          expense_date: exp.expense_date || undefined,
        }),
      });
    },
    onSuccess: () => {
      setExpOpen(false);
      setExp({
        amount: "",
        category: "ops",
        description: "",
        expense_date: new Date().toISOString().slice(0, 10),
      });
      setExpErr("");
      qc.invalidateQueries({ queryKey: ["pnl"] });
      qc.invalidateQueries({ queryKey: ["cashflow"] });
      qc.invalidateQueries({ queryKey: ["expenses"] });
      qc.invalidateQueries({ queryKey: ["journal"] });
      qc.invalidateQueries({ queryKey: ["trial-balance"] });
      qc.invalidateQueries({ queryKey: ["balance-sheet"] });
      qc.invalidateQueries({ queryKey: ["ledger"] });
      void toastSuccess("Beban dicatat");
    },
    onError: (e: Error) => {
      setExpErr(e.message);
      void toastError(e.message);
    },
  });

  const flow = Array.isArray(cf.data) ? cf.data : [];
  const accounts = Array.isArray(accountsQ.data) ? accountsQ.data : [];
  const expenses = expensesQ.data?.data ?? [];
  const aging = business.data?.aging ?? {};
  const trialRows = trialBalance.data?.rows ?? [];
  const journalEntries = journal.data?.data ?? [];
  const journalTotal = journal.data?.total ?? 0;
  const ledgerData = ledger.data;
  const typeLabel: Record<string, string> = {
    asset: "Aset",
    liability: "Liabilitas",
    equity: "Ekuitas",
    revenue: "Pendapatan",
    expense: "Beban",
  };
  const catLabel: Record<string, string> = {
    ops: "Operasional",
    gaji: "Gaji",
    sewa: "Sewa",
    marketing: "Marketing",
    lainnya: "Lainnya",
  };

  async function exportInvoices(kind: "csv" | "xlsx") {
    const ok = await confirm({
      title: "Export tagihan",
      description: `Unduh data tagihan sebagai ${kind.toUpperCase()}?`,
      confirmLabel: "Unduh",
    });
    if (!ok) return;
    try {
      await apiDownload(`/api/reports/invoices.${kind}`, `invoices.${kind}`);
    } catch (e: unknown) {
      void toastError(e instanceof Error ? e.message : "Export gagal");
    }
  }

  async function exportCustomerPayments(kind: "csv" | "xlsx") {
    try {
      await apiDownload(`/api/reports/customer-payments.${kind}?${payParams.toString()}`, `customer-payments.${kind}`);
    } catch (e: unknown) {
      void toastError(e instanceof Error ? e.message : "Export gagal");
    }
  }

  const payReportData = payReport.data;
  const payRows = payReportData?.data ?? [];
  const payByCategory = payReportData?.by_category ?? [];
  const payByCustomer = payReportData?.by_customer ?? [];

  return (
    <Section
      title="Akunting & laporan"
      actions={
        <div className="flex flex-wrap gap-2">
          <Button type="button" variant="outline" onClick={() => void exportInvoices("csv")}>
            <IconDownload /> CSV
          </Button>
          <Button type="button" variant="outline" onClick={() => void exportInvoices("xlsx")}>
            <IconDownload /> XLSX
          </Button>
          <Button type="button" onClick={() => setExpOpen(true)}>
            + Catat beban
          </Button>
        </div>
      }
    >
      <Tabs value={tab} onValueChange={setTab}>
        <TabsList aria-label="Akunting" className="mb-5">
          <TabsTrigger value="ringkasan">Ringkasan</TabsTrigger>
          <TabsTrigger value="beban">Beban</TabsTrigger>
          <TabsTrigger value="pemasukan">Pemasukan</TabsTrigger>
          <TabsTrigger value="jurnal">Jurnal</TabsTrigger>
          <TabsTrigger value="neraca_saldo">Neraca saldo</TabsTrigger>
          <TabsTrigger value="neraca">Neraca</TabsTrigger>
          <TabsTrigger value="coa">Bagan akun</TabsTrigger>
        </TabsList>

        <TabsContent value="ringkasan" className="space-y-6">
          <div className="flex flex-wrap items-end gap-3">
            <div>
              <Label className="mb-1.5 block">P&amp;L dari</Label>
              <Input type="date" value={finFrom} onChange={(e) => setFinFrom(e.target.value)} />
            </div>
            <div>
              <Label className="mb-1.5 block">sampai</Label>
              <Input type="date" value={finTo} onChange={(e) => setFinTo(e.target.value)} />
            </div>
          </div>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
            <Card title="Pendapatan" value={formatRp(pnl.data?.revenue ?? 0)} />
            <Card title="Beban" value={formatRp(pnl.data?.expense ?? 0)} />
            <Card title="Laba / rugi" value={formatRp(pnl.data?.profit ?? 0)} />
            <Card title="MRR" value={formatRp(Math.round(business.data?.mrr ?? 0))} />
          </div>
          {(pnl.data?.revenue_accounts?.length ?? 0) > 0 || (pnl.data?.expense_accounts?.length ?? 0) > 0 ? (
            <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
              <div className="panel-card panel-card-pad">
                <h3 className="eyebrow mb-3">Rincian pendapatan</h3>
                <Table
                  columns={["Kode", "Akun", "Nominal"]}
                  hideRowNumber
                  rows={(pnl.data?.revenue_accounts ?? []).map((a) => [a.code, a.name, formatRp(a.amount)])}
                />
              </div>
              <div className="panel-card panel-card-pad">
                <h3 className="eyebrow mb-3">Rincian beban</h3>
                <Table
                  columns={["Kode", "Akun", "Nominal"]}
                  hideRowNumber
                  rows={(pnl.data?.expense_accounts ?? []).map((a) => [a.code, a.name, formatRp(a.amount)])}
                />
              </div>
            </div>
          ) : null}
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
            <Card title="ARPU" value={formatRp(Math.round(business.data?.arpu ?? 0))} />
            <Card title="Tagihan belum lunas" value={business.data?.unpaid_invoices ?? 0} />
            <Card title="Pelanggan aktif" value={business.data?.active_customers ?? 0} />
            <Card
              title={`Churn ${churn.data?.window_days ?? 30}h`}
              value={`${(((churn.data?.churn_ratio ?? 0) as number) * 100).toFixed(1)}%`}
            />
          </div>

          <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
            <div className="panel-card panel-card-pad lg:col-span-2">
              <div className="mb-4 flex items-center justify-between gap-2">
                <h3 className="eyebrow">Arus kas</h3>
                <span className="text-xs text-[var(--muted)]">6 bulan terakhir</span>
              </div>
              <ReactEChartsCore
                echarts={echarts}
                style={{ height: 280 }}
                option={{
                  backgroundColor: "transparent",
                  textStyle: { color: "var(--chart-axis)", fontFamily: "Plus Jakarta Sans" },
                  legend: { data: ["Masuk", "Keluar"], textStyle: { color: "var(--muted)" } },
                  grid: { left: 48, right: 12, top: 36, bottom: 32 },
                  xAxis: {
                    type: "category",
                    data: flow.map((x) => x.month),
                    axisLine: { lineStyle: { color: "var(--border)" } },
                  },
                  yAxis: {
                    type: "value",
                    splitLine: { lineStyle: { color: "var(--chart-grid)" } },
                  },
                  series: [
                    {
                      name: "Masuk",
                      type: "bar",
                      data: flow.map((x) => x.inflow),
                      itemStyle: { color: "var(--chart-bar)", borderRadius: [4, 4, 0, 0] },
                      barMaxWidth: 18,
                    },
                    {
                      name: "Keluar",
                      type: "bar",
                      data: flow.map((x) => x.outflow),
                      itemStyle: { color: "var(--warn)", borderRadius: [4, 4, 0, 0] },
                      barMaxWidth: 18,
                    },
                  ],
                  tooltip: { trigger: "axis" },
                }}
              />
              <Table
                columns={["Bulan", "Masuk", "Keluar", "Net"]}
                rows={flow.map((r) => [r.month, formatRp(r.inflow), formatRp(r.outflow), formatRp(r.net)])}
              />
            </div>

            <div className="panel-card panel-card-pad space-y-4">
              <h3 className="eyebrow">Aging piutang</h3>
              <Table
                columns={["Bucket", "Nominal"]}
                rows={[
                  ["Current", formatRp(aging.current ?? 0)],
                  ["1–30 hari", formatRp(aging.d30 ?? 0)],
                  ["31–60 hari", formatRp(aging.d60 ?? 0)],
                  ["> 60 hari", formatRp(aging.d90 ?? 0)],
                ]}
              />
              <div className="rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--panel-muted)]/50 p-3 text-sm">
                <p className="font-medium text-[var(--text)]">Churn</p>
                <p className="mt-1 text-[var(--muted)]">
                  {churn.data?.dismantled_30d ?? churn.data?.canceled_30d ?? 0} cabut / {churn.data?.active ?? 0}{" "}
                  pelanggan aktif (
                  {(((churn.data?.churn_ratio ?? 0) as number) * 100).toFixed(1)}%) dalam{" "}
                  {churn.data?.window_days ?? 30} hari.
                </p>
              </div>
            </div>
          </div>
        </TabsContent>

        <TabsContent value="beban" className="space-y-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-sm text-[var(--muted)]">{expensesQ.data?.total ?? 0} catatan beban</p>
            <Button type="button" onClick={() => setExpOpen(true)}>
              + Catat beban
            </Button>
          </div>
          <Table
            columns={["Tanggal", "Kategori", "Keterangan", "Nominal"]}
            rows={expenses.map((e) => [
              e.expense_date,
              catLabel[e.category] || e.category,
              e.description || "—",
              formatRp(e.amount),
            ])}
          />
        </TabsContent>

        <TabsContent value="pemasukan" className="space-y-6">
          <div className="flex flex-wrap items-end gap-3">
            <div>
              <Label className="mb-1.5 block">Dari</Label>
              <Input type="date" value={payFrom} onChange={(e) => setPayFrom(e.target.value)} />
            </div>
            <div>
              <Label className="mb-1.5 block">Sampai</Label>
              <Input type="date" value={payTo} onChange={(e) => setPayTo(e.target.value)} />
            </div>
            <div className="min-w-48 flex-1">
              <Label className="mb-1.5 block">Cari</Label>
              <Input
                placeholder="Pelanggan / invoice / referensi"
                value={paySearch}
                onChange={(e) => setPaySearch(e.target.value)}
              />
            </div>
            <div className="min-w-44">
              <Label className="mb-1.5 block">Kategori</Label>
              <Select value={payCategory} onValueChange={setPayCategory}>
                <SelectTrigger>
                  <SelectValue placeholder="Semua kategori" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="__all__">Semua kategori</SelectItem>
                  {PAYMENT_CATEGORY_OPTIONS.map((c) => (
                    <SelectItem key={c} value={c}>
                      {c}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <label className="flex items-center gap-2 pb-2 text-sm text-[var(--muted)]">
              <input type="checkbox" checked={paySandbox} onChange={(e) => setPaySandbox(e.target.checked)} />
              Tampilkan sandbox
            </label>
            <div className="flex gap-2">
              <Button type="button" variant="outline" onClick={() => void exportCustomerPayments("csv")}>
                <IconDownload /> CSV
              </Button>
              <Button type="button" variant="outline" onClick={() => void exportCustomerPayments("xlsx")}>
                <IconDownload /> XLSX
              </Button>
            </div>
          </div>

          <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
            <Card title="Total pemasukan" value={formatRp(payReportData?.total_amount ?? 0)} />
            <Card title="Baris pendapatan" value={payReportData?.total_count ?? 0} />
            <Card title="Jumlah pelanggan" value={payByCustomer.length} />
          </div>

          <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
            <div className="panel-card panel-card-pad">
              <h3 className="eyebrow mb-3">Pemasukan per kategori</h3>
              <Table
                columns={["Kategori", "Jumlah", "Nominal"]}
                rows={payByCategory.map((c) => [c.category, c.count, formatRp(c.amount)])}
                hideRowNumber
              />
            </div>
            <div className="panel-card panel-card-pad">
              <h3 className="eyebrow mb-3">Pemasukan per pelanggan</h3>
              <Table
                columns={["Pelanggan", "Jumlah", "Nominal"]}
                rows={payByCustomer
                  .slice(0, 20)
                  .map((c) => [c.customer_name || c.customer_code || "—", c.count, formatRp(c.amount)])}
                hideRowNumber
              />
            </div>
          </div>

          <div className="panel-card panel-card-pad">
            <div className="mb-3 flex items-center justify-between gap-2">
              <h3 className="eyebrow">Detail pembayaran</h3>
              {payReport.isFetching ? <span className="text-xs text-[var(--muted)]">Memuat…</span> : null}
            </div>
            <Table
              columns={["Tanggal", "Kode", "Pelanggan", "Invoice", "Kategori", "Metode", "Nominal"]}
              rows={payRows.slice(0, 200).map((r) => [
                formatDate(r.date),
                r.customer_code || "—",
                r.customer_name || "—",
                r.invoice_number || "—",
                r.category,
                r.method || "—",
                formatRp(r.amount),
              ])}
            />
            {payRows.length > 200 ? (
              <p className="mt-2 text-xs text-[var(--muted)]">
                Menampilkan 200 dari {payRows.length} baris. Export untuk data lengkap.
              </p>
            ) : null}
          </div>
        </TabsContent>

        <TabsContent value="jurnal" className="space-y-6">
          <div className="flex flex-wrap items-end gap-3">
            <div>
              <Label className="mb-1.5 block">Dari</Label>
              <Input type="date" value={finFrom} onChange={(e) => setFinFrom(e.target.value)} />
            </div>
            <div>
              <Label className="mb-1.5 block">Sampai</Label>
              <Input type="date" value={finTo} onChange={(e) => setFinTo(e.target.value)} />
            </div>
            <div className="min-w-52">
              <Label className="mb-1.5 block">Akun</Label>
              <Select
                value={jrnAccount}
                onValueChange={(v) => {
                  setJrnAccount(v);
                  setJrnOffset(0);
                }}
              >
                <SelectTrigger>
                  <SelectValue placeholder="Semua akun" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="__all__">Semua akun (jurnal umum)</SelectItem>
                  {accounts.map((a) => (
                    <SelectItem key={a.id} value={a.id}>
                      {a.code} · {a.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            {jrnAccount === "__all__" ? (
              <div className="min-w-48 flex-1">
                <Label className="mb-1.5 block">Cari</Label>
                <Input
                  placeholder="Keterangan / referensi"
                  value={jrnSearch}
                  onChange={(e) => {
                    setJrnSearch(e.target.value);
                    setJrnOffset(0);
                  }}
                />
              </div>
            ) : null}
          </div>

          {jrnAccount === "__all__" ? (
            <>
              <div className="space-y-3">
                {journal.isFetching ? <p className="text-sm text-[var(--muted)]">Memuat…</p> : null}
                {journalEntries.map((e) => (
                  <div key={e.id} className="panel-card p-3">
                    <div className="mb-2 flex flex-wrap items-center justify-between gap-2 text-sm">
                      <div className="min-w-0">
                        <span className="font-medium">{e.description}</span>
                        {e.reference ? <span className="ml-2 text-[var(--muted)]">· {e.reference}</span> : null}
                        {e.source_type ? (
                          <span className="ml-2 rounded-full border border-[var(--border)] px-2 py-0.5 text-[10px] uppercase text-[var(--muted)]">
                            {e.source_type}
                          </span>
                        ) : null}
                      </div>
                      <span className="shrink-0 text-[var(--muted)]">
                        {formatDate(e.entry_date)}
                      </span>
                    </div>
                    <Table
                      columns={["Akun", "Debit", "Kredit"]}
                      hideRowNumber
                      rows={e.lines.map((l) => [
                        `${l.account_code} · ${l.account_name}`,
                        l.debit ? formatRp(l.debit) : "—",
                        l.credit ? formatRp(l.credit) : "—",
                      ])}
                    />
                  </div>
                ))}
              </div>
              <div className="flex items-center justify-between gap-2">
                <p className="text-xs text-[var(--muted)]">
                  Menampilkan {journalEntries.length} dari {journalTotal} entri.
                </p>
                <div className="flex gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    disabled={jrnOffset === 0}
                    onClick={() => setJrnOffset((o) => Math.max(0, o - 50))}
                  >
                    Sebelumnya
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    disabled={jrnOffset + 50 >= journalTotal}
                    onClick={() => setJrnOffset((o) => o + 50)}
                  >
                    Berikutnya
                  </Button>
                </div>
              </div>
            </>
          ) : (
            <div className="panel-card panel-card-pad">
              <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
                <h3 className="eyebrow">
                  Buku besar · {ledgerData?.account_code} {ledgerData?.account_name}
                </h3>
                <span className="text-xs text-[var(--muted)]">
                  Saldo awal {formatRp(ledgerData?.opening ?? 0)}
                </span>
              </div>
              <Table
                columns={["Tanggal", "Keterangan", "Referensi", "Debit", "Kredit", "Saldo"]}
                rows={(ledgerData?.lines ?? []).map((l) => [
                  formatDate(l.entry_date),
                  l.description,
                  l.reference || "—",
                  l.debit ? formatRp(l.debit) : "—",
                  l.credit ? formatRp(l.credit) : "—",
                  formatRp(l.balance),
                ])}
              />
              <div className="mt-3 flex flex-wrap justify-end gap-4 text-sm">
                <span>
                  Total debit: <strong>{formatRp(ledgerData?.total_debit ?? 0)}</strong>
                </span>
                <span>
                  Total kredit: <strong>{formatRp(ledgerData?.total_credit ?? 0)}</strong>
                </span>
                <span>
                  Saldo akhir: <strong>{formatRp(ledgerData?.closing ?? 0)}</strong>
                </span>
              </div>
            </div>
          )}
        </TabsContent>

        <TabsContent value="neraca_saldo" className="space-y-4">
          <div className="flex flex-wrap items-end justify-between gap-3">
            <div className="flex flex-wrap items-end gap-3">
              <div>
                <Label className="mb-1.5 block">Dari</Label>
                <Input type="date" value={finFrom} onChange={(e) => setFinFrom(e.target.value)} />
              </div>
              <div>
                <Label className="mb-1.5 block">Sampai</Label>
                <Input type="date" value={finTo} onChange={(e) => setFinTo(e.target.value)} />
              </div>
            </div>
            <span
              className="rounded-full px-3 py-1 text-xs font-medium"
              style={{
                background: "color-mix(in srgb, var(--accent) 12%, transparent)",
                color: "var(--accent)",
              }}
            >
              Debit {formatRp(trialBalance.data?.total_debit ?? 0)} · Kredit{" "}
              {formatRp(trialBalance.data?.total_credit ?? 0)}
            </span>
          </div>
          <div className="panel-card panel-card-pad">
            <h3 className="eyebrow mb-3">
              Neraca saldo {trialBalance.data?.from} – {trialBalance.data?.to}
            </h3>
            <Table
              columns={["Kode", "Akun", "Tipe", "Debit", "Kredit", "Saldo"]}
              rows={trialRows.map((r) => [
                r.code,
                r.name,
                typeLabel[r.type] || r.type,
                formatRp(r.debit),
                formatRp(r.credit),
                formatRp(r.balance),
              ])}
            />
          </div>
        </TabsContent>

        <TabsContent value="neraca" className="space-y-4">
          <div className="flex flex-wrap items-end justify-between gap-3">
            <div className="flex flex-wrap items-end gap-3">
              <div>
                <Label className="mb-1.5 block">Dari</Label>
                <Input type="date" value={finFrom} onChange={(e) => setFinFrom(e.target.value)} />
              </div>
              <div>
                <Label className="mb-1.5 block">Sampai</Label>
                <Input type="date" value={finTo} onChange={(e) => setFinTo(e.target.value)} />
              </div>
            </div>
            <span
              className="rounded-full px-3 py-1 text-xs font-medium"
              style={{
                background: balanceSheet.data?.balanced
                  ? "color-mix(in srgb, var(--ok, #2b9a66) 14%, transparent)"
                  : "color-mix(in srgb, var(--danger) 14%, transparent)",
                color: balanceSheet.data?.balanced ? "var(--ok, #2b9a66)" : "var(--danger)",
              }}
            >
              {balanceSheet.data?.balanced ? "Seimbang" : "Selisih / tidak seimbang"}
            </span>
          </div>
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
            <div className="panel-card panel-card-pad">
              <h3 className="eyebrow mb-3">Aset</h3>
              <Table
                columns={["Kode", "Akun", "Nominal"]}
                hideRowNumber
                rows={(balanceSheet.data?.assets ?? []).map((a) => [a.code, a.name, formatRp(a.amount)])}
              />
              <p className="mt-3 text-right text-sm">
                Total aset: <strong>{formatRp(balanceSheet.data?.total_assets ?? 0)}</strong>
              </p>
            </div>
            <div className="panel-card panel-card-pad space-y-5">
              <div>
                <h3 className="eyebrow mb-3">Liabilitas</h3>
                <Table
                  columns={["Kode", "Akun", "Nominal"]}
                  hideRowNumber
                  rows={(balanceSheet.data?.liabilities ?? []).map((a) => [a.code, a.name, formatRp(a.amount)])}
                />
                <p className="mt-2 text-right text-sm text-[var(--muted)]">
                  Total liabilitas: {formatRp(balanceSheet.data?.total_liabilities ?? 0)}
                </p>
              </div>
              <div>
                <h3 className="eyebrow mb-3">Ekuitas</h3>
                <Table
                  columns={["Kode", "Akun", "Nominal"]}
                  hideRowNumber
                  rows={[
                    ...(balanceSheet.data?.equity ?? []).map((a): [string, string, string] => [a.code, a.name, formatRp(a.amount)]),
                    ["—", "Laba periode berjalan", formatRp(balanceSheet.data?.current_earnings ?? 0)],
                  ]}
                />
                <p className="mt-2 text-right text-sm text-[var(--muted)]">
                  Total ekuitas: {formatRp((balanceSheet.data?.total_equity ?? 0) + (balanceSheet.data?.current_earnings ?? 0))}
                </p>
              </div>
              <p className="text-right text-sm">
                Total liabilitas + ekuitas:{" "}
                <strong>{formatRp(balanceSheet.data?.total_liabilities_equity ?? 0)}</strong>
              </p>
            </div>
          </div>
        </TabsContent>

        <TabsContent value="coa" className="space-y-4">
          <p className="text-sm text-[var(--muted)]">
            Chart of accounts. Digunakan untuk jurnal pembayaran (kas &amp; pendapatan).
          </p>
          <Table
            columns={["Kode", "Nama", "Tipe"]}
            rows={accounts.map((a) => [a.code, a.name, typeLabel[a.type] || a.type])}
          />
        </TabsContent>
      </Tabs>

      <FormDialog
        open={expOpen}
        title="Catat beban"
        onClose={() => {
          setExpOpen(false);
          setExpErr("");
        }}
      >
        <form
          className="grid gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            createExpense.mutate();
          }}
        >
          <div>
            <Label className="mb-1.5 block">Nominal (Rp)</Label>
            <Input
              type="number"
              min={1}
              step={1000}
              value={exp.amount}
              onChange={(e) => setExp({ ...exp, amount: e.target.value })}
              required
            />
          </div>
          <div>
            <Label className="mb-1.5 block">Kategori</Label>
            <Select value={exp.category} onValueChange={(v) => setExp({ ...exp, category: v })}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Object.entries(catLabel).map(([k, label]) => (
                  <SelectItem key={k} value={k}>
                    {label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div>
            <Label className="mb-1.5 block">Tanggal</Label>
            <Input
              type="date"
              value={exp.expense_date}
              onChange={(e) => setExp({ ...exp, expense_date: e.target.value })}
              required
            />
          </div>
          <div>
            <Label className="mb-1.5 block">Keterangan</Label>
            <Input
              placeholder="Opsional"
              value={exp.description}
              onChange={(e) => setExp({ ...exp, description: e.target.value })}
            />
          </div>
          <div className="flex flex-wrap gap-2">
            <Button type="submit" disabled={createExpense.isPending}>
              {createExpense.isPending ? "Menyimpan…" : "Simpan"}
            </Button>
            <Button type="button" variant="secondary" onClick={() => setExpOpen(false)}>
              Batal
            </Button>
          </div>
          {expErr ? <p className="text-sm text-[var(--danger)]">{expErr}</p> : null}
        </form>
      </FormDialog>
    </Section>
  );
}

export function ResellersPage() {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();

  type CommissionRow = {
    id: string;
    customer_name?: string;
    customer_code?: string;
    reseller_name?: string;
    sales_user_name?: string;
    amount: number;
    basis?: string;
    status: string;
    created_at: string;
    paid_at?: string | null;
  };

  const [commStatus, setCommStatus] = useState("");
  const [commPage, setCommPage] = useState(0);
  const [commLimit, setCommLimit] = useState(25);
  function setCommPageSize(n: number) {
    setCommLimit(n);
    setCommPage(0);
  }
  const [newAmountInput, setNewAmountInput] = useState("");
  const [acqAmountInput, setAcqAmountInput] = useState("");
  const [form, setForm] = useState({ name: "", phone: "", commission_percent: 0, is_active: true });
  const [editId, setEditId] = useState<string | null>(null);
  const [open, setOpen] = useState(false);

  const q = useQuery({
    queryKey: ["resellers"],
    queryFn: () => api<ResellerOpt[]>("/api/resellers"),
  });
  const settingsQ = useQuery({
    queryKey: ["commission-settings"],
    queryFn: () => api<{ new_customer_amount: number; acquisition_amount: number }>("/api/settings/commission"),
  });
  const commissionsQ = useQuery({
    queryKey: ["commissions", commStatus, commPage, commLimit],
    queryFn: () =>
      api<{ data: CommissionRow[]; total: number }>(
        `/api/commissions?limit=${commLimit}&offset=${commPage * commLimit}${commStatus ? `&status=${encodeURIComponent(commStatus)}` : ""}`,
      ),
  });

  useEffect(() => {
    if (settingsQ.data) {
      setNewAmountInput(String(settingsQ.data.new_customer_amount ?? 0));
      setAcqAmountInput(String(settingsQ.data.acquisition_amount ?? 0));
    }
  }, [settingsQ.data]);

  const saveSettings = useMutation({
    mutationFn: () =>
      api("/api/settings/commission", {
        method: "PUT",
        body: JSON.stringify({
          new_customer_amount: Math.max(0, Math.floor(Number(newAmountInput) || 0)),
          acquisition_amount: Math.max(0, Math.floor(Number(acqAmountInput) || 0)),
        }),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["commission-settings"] });
      void toastSuccess("Setting komisi disimpan");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const saveReseller = useMutation({
    mutationFn: () => {
      const body = {
        name: form.name.trim(),
        phone: form.phone.trim(),
        commission_percent: form.commission_percent,
        is_active: form.is_active,
      };
      if (editId) return api(`/api/resellers/${editId}`, { method: "PUT", body: JSON.stringify(body) });
      return api("/api/resellers", { method: "POST", body: JSON.stringify(body) });
    },
    onSuccess: () => {
      const wasEdit = Boolean(editId);
      setOpen(false);
      setEditId(null);
      setForm({ name: "", phone: "", commission_percent: 0, is_active: true });
      qc.invalidateQueries({ queryKey: ["resellers"] });
      void toastSuccess(wasEdit ? "Reseller diperbarui" : "Reseller ditambahkan");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const removeReseller = useMutation({
    mutationFn: (id: string) => api(`/api/resellers/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["resellers"] });
      void toastSuccess("Reseller dihapus");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const markPaid = useMutation({
    mutationFn: (id: string) => api(`/api/commissions/${id}/paid`, { method: "POST" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["commissions"] });
      void toastSuccess("Komisi ditandai dibayar");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const voidComm = useMutation({
    mutationFn: (id: string) => api(`/api/commissions/${id}/void`, { method: "POST" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["commissions"] });
      qc.invalidateQueries({ queryKey: ["resellers"] });
      void toastSuccess("Komisi dibatalkan");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const list = Array.isArray(q.data) ? q.data : [];
  const [resellerSearch, setResellerSearch] = useState("");
  const [commSearch, setCommSearch] = useState("");
  const filteredResellers = useMemo(
    () => list.filter((r) => matchesQuery(resellerSearch, r.name, r.phone)),
    [list, resellerSearch],
  );
  const commissions = commissionsQ.data?.data ?? [];
  const filteredCommissions = useMemo(
    () =>
      commissions.filter((c) =>
        matchesQuery(commSearch, c.customer_name, c.customer_code, c.reseller_name, c.sales_user_name, c.basis),
      ),
    [commissions, commSearch],
  );
  const commTotal = commissionsQ.data?.total ?? 0;
  const commPageCount = Math.max(1, Math.ceil(commTotal / commLimit));
  const statusLabel: Record<string, string> = { pending: "Pending", paid: "Dibayar", void: "Void" };

  const now = new Date();
  const [commMonth, setCommMonth] = useState(
    `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}`,
  );
  const [exporting, setExporting] = useState("");

  function monthRange(m: string): { from: string; to: string } {
    if (!m) return { from: "", to: "" };
    const [y, mo] = m.split("-").map(Number);
    if (!y || !mo) return { from: "", to: "" };
    const last = new Date(y, mo, 0).getDate();
    return { from: `${m}-01`, to: `${m}-${String(last).padStart(2, "0")}` };
  }

  async function exportCommissions(group: "recipient" | "detail") {
    const { from, to } = monthRange(commMonth);
    const params = new URLSearchParams({ group });
    if (from) params.set("from", from);
    if (to) params.set("to", to);
    if (commStatus) params.set("status", commStatus);
    setExporting(group);
    try {
      await apiDownload(
        `/api/commissions/export.csv?${params.toString()}`,
        group === "detail" ? "komisi-detail.csv" : "komisi-rekap.csv",
      );
      void toastSuccess("Export komisi diunduh");
    } catch (e: unknown) {
      void toastError(e instanceof Error ? e.message : "Export gagal");
    } finally {
      setExporting("");
    }
  }

  return (
    <Section
      title="Reseller & Komisi"
      actions={
        <Button
          type="button"
          onClick={() => {
            setEditId(null);
            setForm({ name: "", phone: "", commission_percent: 0, is_active: true });
            setOpen(true);
          }}
        >
          + Reseller
        </Button>
      }
    >
      <p className="mb-4 text-sm text-[var(--muted)]">
        Komisi flat per jenis (pelanggan baru / akuisisi). Saldo reseller naik saat entry dibuat; staff dilacak di riwayat
        saja.
      </p>

      <div className="panel-card mb-6 p-4">
        <h3 className="mb-2 text-sm font-semibold">Setting komisi</h3>
        <form
          className="flex flex-wrap items-end gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            saveSettings.mutate();
          }}
        >
          <div className="min-w-[200px]">
            <Label className="mb-1.5 block">Pelanggan baru (Rp)</Label>
            <Input
              type="number"
              min={0}
              step={1000}
              value={newAmountInput}
              onChange={(e) => setNewAmountInput(e.target.value)}
              placeholder="0 = nonaktif"
            />
          </div>
          <div className="min-w-[200px]">
            <Label className="mb-1.5 block">Akuisisi (Rp)</Label>
            <Input
              type="number"
              min={0}
              step={1000}
              value={acqAmountInput}
              onChange={(e) => setAcqAmountInput(e.target.value)}
              placeholder="0 = nonaktif"
            />
          </div>
          <Button type="submit" disabled={saveSettings.isPending}>
            {saveSettings.isPending ? "Menyimpan…" : "Simpan"}
          </Button>
        </form>
        <p className="mt-2 text-xs text-[var(--muted)]">
          Baru: {formatRp(settingsQ.data?.new_customer_amount ?? 0)} · Akuisisi:{" "}
          {formatRp(settingsQ.data?.acquisition_amount ?? 0)}. Nilai 0 = tidak buat entry untuk jenis itu.
        </p>
      </div>

      <h3 className="mb-2 text-sm font-semibold">Reseller</h3>
      <ListToolbar
        search={resellerSearch}
        onSearchChange={setResellerSearch}
        searchPlaceholder="Nama atau telepon…"
        total={filteredResellers.length}
      />
      <Table
        columns={["Nama", "Telepon", "Saldo", "Aktif", "Aksi"]}
        rows={filteredResellers.map((r) => [
          r.name,
          r.phone ?? "—",
          formatRp(r.balance ?? 0),
          r.is_active === false ? "nonaktif" : "aktif",
          <span key={r.id} className="flex flex-wrap items-center gap-1.5">
            <IconButton
              label="Edit reseller"
              onClick={() => {
                setEditId(r.id);
                setForm({
                  name: r.name,
                  phone: r.phone || "",
                  commission_percent: r.commission_percent ?? 0,
                  is_active: r.is_active !== false,
                });
                setOpen(true);
              }}
            >
              <IconPencil />
            </IconButton>
            <IconButton
              label="Hapus reseller"
              danger
              onClick={async () => {
                const ok = await confirm({
                  title: "Hapus reseller",
                  description: `Hapus reseller "${r.name}"?`,
                  confirmLabel: "Hapus",
                });
                if (!ok) return;
                removeReseller.mutate(r.id);
              }}
            >
              <IconTrash />
            </IconButton>
          </span>,
        ])}
      />

      <div className="mt-8 mb-3">
        <div className="mb-3 flex flex-wrap items-end justify-between gap-3">
          <div>
            <h3 className="text-sm font-semibold">Riwayat komisi</h3>
            <p className="text-xs text-[var(--muted)]">
              Export <strong>Rekap per penerima</strong> = total komisi tiap sales/reseller (pending, dibayar, total)
              untuk bulan dipilih — cocok untuk rekap pembayaran komisi.
            </p>
          </div>
          <div className="flex flex-wrap items-end gap-2">
            <label className="grid gap-1 text-xs">
              <span className="text-[var(--muted)]">Bulan rekap</span>
              <input
                type="month"
                className="input"
                value={commMonth}
                onChange={(e) => setCommMonth(e.target.value)}
              />
            </label>
            <button
              type="button"
              className="btn-ghost inline-flex items-center gap-1.5"
              disabled={exporting !== ""}
              onClick={() => void exportCommissions("recipient")}
            >
              <IconDownload /> {exporting === "recipient" ? "Mengunduh…" : "Export rekap"}
            </button>
            <button
              type="button"
              className="btn-ghost inline-flex items-center gap-1.5"
              disabled={exporting !== ""}
              onClick={() => void exportCommissions("detail")}
            >
              <IconDownload /> {exporting === "detail" ? "Mengunduh…" : "Export detail"}
            </button>
          </div>
        </div>
        <ListToolbar
          search={commSearch}
          onSearchChange={setCommSearch}
          searchPlaceholder="Pelanggan, reseller, sales…"
          filters={[
            {
              key: "status",
              label: "Status",
              value: commStatus,
              onChange: (v) => {
                setCommStatus(v);
                setCommPage(0);
              },
              options: [
                { value: "pending", label: "Pending" },
                { value: "paid", label: "Dibayar" },
                { value: "void", label: "Void" },
              ],
            },
          ]}
          total={commTotal}
          page={commPage}
          pageCount={commPageCount}
          onPageChange={setCommPage}
          pageSize={commLimit}
          onPageSizeChange={setCommPageSize}
        />
      </div>
      <Table
        rowNumberStart={commPage * commLimit + 1}
        columns={["Tanggal", "Pelanggan", "Jenis", "Penerima", "Nominal", "Status", "Aksi"]}
        rows={filteredCommissions.map((c) => [
          c.created_at ? formatDateTime(c.created_at) : "—",
          c.customer_code ? `${c.customer_code} · ${c.customer_name || ""}` : c.customer_name || "—",
          commissionBasisLabel(c.basis || ""),
          c.reseller_name ? `Reseller: ${c.reseller_name}` : c.sales_user_name ? `Sales: ${c.sales_user_name}` : "—",
          formatRp(c.amount),
          statusLabel[c.status] || c.status,
          <span key={c.id} className="flex flex-wrap items-center gap-1.5">
            {c.status === "pending" ? (
              <IconButton label="Tandai dibayar" onClick={() => markPaid.mutate(c.id)}>
                <IconCheck />
              </IconButton>
            ) : null}
            {c.status === "pending" || c.status === "paid" ? (
              <IconButton
                label="Batalkan komisi"
                danger
                onClick={async () => {
                  const ok = await confirm({
                    title: "Void komisi",
                    description: "Batalkan entry komisi ini? Saldo reseller akan dikurangi jika ada.",
                    confirmLabel: "Void",
                  });
                  if (!ok) return;
                  voidComm.mutate(c.id);
                }}
              >
                <IconBan />
              </IconButton>
            ) : null}
          </span>,
        ])}
      />

      <FormDialog
        open={open}
        title={editId ? "Edit reseller" : "Tambah reseller"}
        onClose={() => {
          setOpen(false);
          setEditId(null);
        }}
      >
        <form
          className="grid gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            saveReseller.mutate();
          }}
        >
          <Input
            placeholder="Nama"
            value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
            required
          />
          <Input
            placeholder="Telepon"
            value={form.phone}
            onChange={(e) => setForm({ ...form, phone: e.target.value })}
          />
          {editId ? (
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={form.is_active}
                onChange={(e) => setForm({ ...form, is_active: e.target.checked })}
              />
              Aktif
            </label>
          ) : null}
          <div className="flex flex-wrap gap-2">
            <Button type="submit" disabled={saveReseller.isPending}>
              {saveReseller.isPending ? "Menyimpan…" : "Simpan"}
            </Button>
            <Button type="button" variant="secondary" onClick={() => setOpen(false)}>
              Batal
            </Button>
          </div>
        </form>
      </FormDialog>
    </Section>
  );
}

export function InvoiceActions({
  id,
  invoiceNumber,
  status,
  trashed,
  onDone,
}: {
  id: string;
  invoiceNumber: string;
  status: string;
  trashed?: boolean;
  onDone?: () => void;
}) {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();
  const unpaid = !trashed && status !== "paid" && status !== "void" && status !== "cancelled";
  const payOpts = useQuery({
    queryKey: ["pay-options"],
    queryFn: () => api<PayOption[]>("/api/integrations/pay-options"),
    staleTime: 60_000,
  });
  const duitkuSandbox = payOptionsHasDuitkuSandbox(payOpts.data);
  function refreshBilling() {
    void qc.invalidateQueries({ queryKey: ["invoices"] });
    void qc.invalidateQueries({ queryKey: ["invoices-recent"] });
    void qc.invalidateQueries({ queryKey: ["payments"] });
    onDone?.();
  }
  const pay = useMutation({
    mutationFn: () => api(`/api/invoices/${id}/pay`, { method: "POST", body: JSON.stringify({ method: PAY_METHOD_TUNAI }) }),
    onSuccess: () => {
      void toastSuccess("Pembayaran dicatat");
      refreshBilling();
    },
    onError: (e: Error) => void toastError(e.message || "Gagal bayar"),
  });
  const sandboxPay = useMutation({
    mutationFn: () => api(`/api/invoices/${id}/sandbox-pay`, { method: "POST" }),
    onSuccess: () => {
      void toastSuccess("Sandbox: tagihan ditandai lunas (alur webhook Duitku)");
      refreshBilling();
    },
    onError: (e: Error) => void toastError(e.message || "Gagal uji sandbox"),
  });
  const remove = useMutation({
    mutationFn: () => api(`/api/invoices/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      void toastSuccess("Tagihan dipindah ke sampah");
      refreshBilling();
    },
    onError: (e: Error) => void toastError(e.message || "Gagal hapus tagihan"),
  });
  const restore = useMutation({
    mutationFn: () => api(`/api/invoices/${id}/restore`, { method: "POST" }),
    onSuccess: () => {
      void toastSuccess("Tagihan dipulihkan");
      refreshBilling();
    },
    onError: (e: Error) => void toastError(e.message || "Gagal pulihkan tagihan"),
  });
  const purge = useMutation({
    mutationFn: () => api(`/api/invoices/${id}/purge`, { method: "DELETE" }),
    onSuccess: () => {
      void toastSuccess("Tagihan dihapus permanen");
      refreshBilling();
    },
    onError: (e: Error) => void toastError(e.message || "Gagal hapus permanen"),
  });
  const applyDiscount = useMutation({
    mutationFn: () =>
      api<{ discount_name: string; discount_amount: number; total_amount: number }>(
        `/api/invoices/${id}/apply-discount`,
        { method: "POST" },
      ),
    onSuccess: (r) => {
      void toastSuccess(`Diskon "${r.discount_name}" diterapkan (${formatRp(r.discount_amount)}). Total baru ${formatRp(r.total_amount)}`);
      refreshBilling();
    },
    onError: (e: Error) => void toastError(e.message || "Gagal menerapkan diskon"),
  });
  const [pdfBusy, setPdfBusy] = useState(false);

  return (
    <span className="flex flex-wrap items-center gap-1.5">
      {unpaid && (
        <IconButton
          label="Bayar manual"
          disabled={pay.isPending}
          onClick={() => {
            void confirm({
              title: "Catat pembayaran?",
              description: `Tandai lunas invoice ${invoiceNumber} (pembayaran manual)?`,
              confirmLabel: "Bayar",
            }).then((ok) => {
              if (ok) pay.mutate();
            });
          }}
        >
          <IconBanknote />
        </IconButton>
      )}
      {unpaid && duitkuSandbox && (
        <IconButton
          label="Uji sandbox: tandai lunas"
          disabled={sandboxPay.isPending}
          onClick={() => {
            void confirm({
              title: "Uji sandbox Duitku?",
              description: `Tandai lunas ${invoiceNumber} lewat alur webhook Duitku (hanya sandbox). Dashboard Duitku tidak punya tombol ini.`,
              confirmLabel: "Tandai lunas",
            }).then((ok) => {
              if (ok) sandboxPay.mutate();
            });
          }}
        >
          <IconZap />
        </IconButton>
      )}
      {unpaid && (
        <IconButton
          label="Terapkan aturan diskon"
          disabled={applyDiscount.isPending}
          onClick={() => {
            void confirm({
              title: "Terapkan aturan diskon?",
              description: `Hitung ulang ${invoiceNumber} dengan aturan diskon yang berlaku hari ini (diskon terbaik untuk pelanggan & paket ini). Hanya untuk tagihan yang belum memuat diskon.`,
              confirmLabel: "Terapkan",
            }).then((ok) => {
              if (ok) applyDiscount.mutate();
            });
          }}
        >
          <IconTicket />
        </IconButton>
      )}
      <IconButton
        label="Unduh PDF"
        disabled={pdfBusy}
        onClick={() => {
          setPdfBusy(true);
          void apiDownload(`/api/invoices/${id}/pdf`, `${invoiceNumber || id}.pdf`)
            .catch((e: Error) => toastError(e.message || "PDF gagal"))
            .finally(() => setPdfBusy(false));
        }}
      >
        <IconDownload />
      </IconButton>
      {trashed ? (
        <>
          <IconButton
            label="Pulihkan tagihan"
            disabled={restore.isPending}
            onClick={() => {
              void confirm({
                title: "Pulihkan tagihan",
                description: `Kembalikan tagihan ${invoiceNumber || id} beserta pembayaran yang ikut terhapus?`,
                confirmLabel: "Pulihkan",
              }).then((ok) => {
                if (ok) restore.mutate();
              });
            }}
          >
            <IconUndo />
          </IconButton>
          <IconButton
            label="Hapus permanen"
            danger
            disabled={purge.isPending}
            onClick={() => {
              void confirm({
                title: "Hapus permanen?",
                description: `Tagihan ${invoiceNumber || id} dihapus SELAMANYA beserta item-nya. Hanya bisa untuk yang belum dibayar. Tidak bisa dikembalikan.`,
                confirmLabel: "Hapus permanen",
                danger: true,
              }).then((ok) => {
                if (ok) purge.mutate();
              });
            }}
          >
            <IconTrash />
          </IconButton>
        </>
      ) : (
        <IconButton
          label="Hapus tagihan"
          danger
          disabled={remove.isPending}
          onClick={() => {
            void confirm({
              title: "Hapus tagihan",
              description: `Pindahkan tagihan ${invoiceNumber || id} ke sampah? Pembayaran terkait ikut disembunyikan. Bisa dipulihkan dari filter Sampah.`,
              confirmLabel: "Hapus",
            }).then((ok) => {
              if (ok) remove.mutate();
            });
          }}
        >
          <IconTrash />
        </IconButton>
      )}
    </span>
  );
}
