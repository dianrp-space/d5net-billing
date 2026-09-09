import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { IconBox, IconPencil, IconPlug, IconTrash } from "../icons";
import { useAppDialog } from "../confirm";
import { toastError, toastSuccess } from "../swal";
import {
  FormDialog,
  formatRp,
  IconButton,
  Input,
  Label,
  SearchableSelect,
  Section,
  SecretInput,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Table,
  Button,
} from "../ui";
import { Checkbox } from "@/components/ui/checkbox";

function clampCycleStartDay(n: number | undefined) {
  const v = Math.floor(Number(n) || 1);
  if (!Number.isFinite(v) || v < 1) return 1;
  return Math.min(28, v);
}

function nextCycleAnchor(start: Date, day: number) {
  const d = clampCycleStartDay(day);
  const thisMonth = new Date(start.getFullYear(), start.getMonth(), d, 12, 0, 0, 0);
  if (thisMonth.getTime() > start.getTime()) return thisMonth;
  return new Date(start.getFullYear(), start.getMonth() + 1, d, 12, 0, 0, 0);
}

export function SubscriptionsPage({
  customerId,
  createMode = false,
  onBack,
  onOpenList,
  onOpenCreate,
}: {
  customerId: string;
  createMode?: boolean;
  onBack: () => void;
  onOpenList: () => void;
  onOpenCreate: () => void;
}) {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();
  const cycleStartDayRef = useRef(1);
  type SubRow = {
    id: string;
    customer_id: string;
    plan_id: string;
    router_id?: string | null;
    username: string;
    customer_name: string;
    plan_name: string;
    status: string;
    started_at?: string | null;
    next_bill_at?: string | null;
    odp_id?: string | null;
    odp_code?: string;
    odp_name?: string;
    port_number?: number | null;
  };
  type CustomerOpt = {
    id: string;
    full_name: string;
    customer_code: string;
    cluster_id?: string | null;
    cluster_name?: string;
    service_status?: string;
    dismantled_at?: string | null;
  };
  type OfferOpt = {
    plan_id: string;
    plan_name: string;
    plan_code: string;
    price: number;
    service_type: string;
    is_active: boolean;
  };
  type RouterOpt = { id: string; name: string; cluster_id?: string | null; is_active: boolean };
  type OdpOpt = {
    id: string;
    name: string;
    code: string;
    cluster_id?: string | null;
    port_count: number;
    used_ports: number;
    free_ports: number;
  };
  type OdpPortOpt = { port_number: number; status: string };

  const emptyForm = {
    customer_id: customerId,
    plan_id: "",
    router_id: "",
    username: "",
    password: "",
    odp_id: "",
    port_number: "",
  };
  const emptyBillForm = () => {
    const start = new Date();
    start.setHours(12, 0, 0, 0);
    const next = nextCycleAnchor(start, cycleStartDayRef.current);
    return {
      activate_now: true,
      started_at: toDateInput(start),
      next_bill_at: toDateInput(next),
      prorate: true,
    };
  };
  const [form, setForm] = useState(emptyForm);
  const [billForm, setBillForm] = useState(emptyBillForm);
  const [createOpen, setCreateOpen] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);
  const [editStatus, setEditStatus] = useState<string | null>(null);
  const [editStartedAt, setEditStartedAt] = useState<string | null>(null);
  const [editNextBillAt, setEditNextBillAt] = useState<string | null>(null);
  const [formErr, setFormErr] = useState("");
  const [activateTarget, setActivateTarget] = useState<SubRow | null>(null);
  const [changePlanTarget, setChangePlanTarget] = useState<SubRow | null>(null);
  const [changePlanId, setChangePlanId] = useState("");
  type PlanChangeQuote = {
    old_plan_id: string;
    old_plan_name: string;
    old_price: number;
    new_plan_id: string;
    new_plan_name: string;
    new_price: number;
    remaining_days: number;
    period_days: number;
    old_credit: number;
    new_charge: number;
    delta_subtotal: number;
    tax_amount: number;
    total_amount: number;
    direction: string;
    next_bill_at?: string;
    requires_charge: boolean;
  };
  const [changePlanQuote, setChangePlanQuote] = useState<PlanChangeQuote | null>(null);
  const [changePlanErr, setChangePlanErr] = useState("");

  const dialogOpen = createOpen || Boolean(editId);

  function toDateInput(d: Date) {
    const y = d.getFullYear();
    const m = String(d.getMonth() + 1).padStart(2, "0");
    const day = String(d.getDate()).padStart(2, "0");
    return `${y}-${m}-${day}`;
  }
  function parseDateInput(s: string) {
    const [y, m, d] = s.split("-").map(Number);
    if (!y || !m || !d) return null;
    return new Date(y, m - 1, d, 12, 0, 0, 0);
  }
  function addBillingCycle(from: Date, cycle: string) {
    const n = new Date(from);
    switch (cycle) {
      case "daily":
        n.setDate(n.getDate() + 1);
        break;
      case "weekly":
        n.setDate(n.getDate() + 7);
        break;
      case "yearly":
        n.setFullYear(n.getFullYear() + 1);
        break;
      default:
        n.setMonth(n.getMonth() + 1);
    }
    return n;
  }
  function cycleDaysOf(cycle: string) {
    switch (cycle) {
      case "daily":
        return 1;
      case "weekly":
        return 7;
      case "yearly":
        return 365;
      default:
        return 30;
    }
  }
  function defaultNextBill(start: Date, cycle: string, prorate: boolean) {
    if (!prorate) return addBillingCycle(start, cycle);
    return nextCycleAnchor(start, cycleStartDayRef.current);
  }
  function isoToDateInput(iso?: string | null) {
    if (!iso) return "";
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return "";
    return toDateInput(d);
  }
  function validateBillDates(f: { started_at: string; next_bill_at: string; prorate: boolean }) {
    if (!f.started_at) return "Tanggal mulai wajib diisi";
    if (f.prorate && !f.next_bill_at) return "Tanggal tagihan berikutnya wajib diisi";
    const start = parseDateInput(f.started_at);
    const next = parseDateInput(f.next_bill_at);
    if (f.prorate && start && next && !(next > start)) {
      return "Tanggal tagihan berikutnya harus setelah tanggal mulai";
    }
    return "";
  }
  const openActivate = (s: SubRow) => {
    const start = s.started_at ? new Date(s.started_at) : new Date();
    start.setHours(12, 0, 0, 0);
    const next = s.next_bill_at
      ? new Date(s.next_bill_at)
      : defaultNextBill(start, "monthly", true);
    next.setHours(12, 0, 0, 0);
    setActivateTarget(s);
    setBillForm({
      activate_now: true,
      started_at: toDateInput(start),
      next_bill_at: toDateInput(next),
      prorate: true,
    });
  };

  const customerQ = useQuery({
    queryKey: ["customer", customerId],
    queryFn: () => api<CustomerOpt>(`/api/customers/${customerId}`),
    enabled: Boolean(customerId),
  });
  const listQ = useQuery({
    queryKey: ["subs", customerId],
    queryFn: () =>
      api<{ data: SubRow[] }>(
        `/api/subscriptions?customer_id=${encodeURIComponent(customerId)}&limit=200`,
      ),
    enabled: Boolean(customerId),
  });
  const customersQ = useQuery({
    queryKey: ["customers"],
    queryFn: () => api<{ data: CustomerOpt[] }>("/api/customers?limit=500"),
    enabled: false,
  });
  const routersQ = useQuery({
    queryKey: ["routers"],
    queryFn: () => api<RouterOpt[]>("/api/routers"),
  });
  const odpsQ = useQuery({
    queryKey: ["odps"],
    queryFn: () => api<OdpOpt[]>("/api/odps"),
    enabled: dialogOpen,
  });

  const lockedCustomer = customerQ.data
    ? {
        id: customerQ.data.id,
        full_name: customerQ.data.full_name,
        customer_code: customerQ.data.customer_code,
        cluster_id: customerQ.data.cluster_id,
        cluster_name: customerQ.data.cluster_name,
        service_status: customerQ.data.service_status,
        dismantled_at: customerQ.data.dismantled_at,
      }
    : null;
  const customerCabut =
    lockedCustomer?.service_status === "dismantled" || Boolean(lockedCustomer?.dismantled_at);
  const customers = lockedCustomer ? [lockedCustomer] : [];
  const selectedCustomer = lockedCustomer || customers.find((c) => c.id === form.customer_id);
  const clusterId = selectedCustomer?.cluster_id || "";

  useEffect(() => {
    if (!createMode) return;
    setEditId(null);
    setEditStatus(null);
    setEditStartedAt(null);
    setEditNextBillAt(null);
    setForm({
      ...emptyForm,
      customer_id: customerId,
      username: lockedCustomer?.customer_code || "",
    });
    setBillForm(emptyBillForm());
    setFormErr("");
    setCreateOpen(true);
  }, [createMode, customerId, lockedCustomer?.customer_code]);

  useEffect(() => {
    if (createMode) return;
    if (!listQ.isSuccess || listQ.isFetching) return;
    if (customerCabut) return;
    if ((listQ.data?.data?.length ?? 0) === 0) onOpenCreate();
  }, [createMode, listQ.isSuccess, listQ.isFetching, listQ.data?.data?.length, onOpenCreate, customerCabut]);


  const offersQ = useQuery({
    queryKey: ["plan-offers", clusterId],
    queryFn: () => api<OfferOpt[]>(`/api/plan-offers?cluster_id=${clusterId}`),
    enabled: dialogOpen && Boolean(clusterId),
  });
  const plansQ = useQuery({
    queryKey: ["plans"],
    queryFn: () => api<{ id: string; name: string; code: string; price: number }[]>("/api/plans"),
    enabled: dialogOpen && Boolean(form.customer_id) && !clusterId,
  });
  const odpPortsQ = useQuery({
    queryKey: ["odp-ports", form.odp_id],
    queryFn: () => api<{ ports: OdpPortOpt[] }>(`/api/odps/${form.odp_id}/ports`),
    enabled: dialogOpen && Boolean(form.odp_id),
  });
  const activatePlansQ = useQuery({
    queryKey: ["plans", "activate"],
    queryFn: () =>
      api<
        {
          id: string;
          name: string;
          price: number;
          billing_cycle?: string;
        }[]
      >("/api/plans"),
    enabled: dialogOpen || Boolean(activateTarget),
  });
  const generalQ = useQuery({
    queryKey: ["settings-branding"],
    queryFn: () =>
      api<{ default_tax_percent?: number; billing_cycle_start_day?: number }>("/api/settings/branding"),
  });
  const cycleStartDay = clampCycleStartDay(generalQ.data?.billing_cycle_start_day);
  cycleStartDayRef.current = cycleStartDay;
  useEffect(() => {
    if (!generalQ.isSuccess) return;
    if (editId) return;
    if (activateTarget?.next_bill_at) return;
    setBillForm((f) => {
      if (!f.prorate) return f;
      const start = parseDateInput(f.started_at);
      if (!start) return f;
      const next = toDateInput(nextCycleAnchor(start, cycleStartDay));
      if (f.next_bill_at === next) return f;
      return { ...f, next_bill_at: next };
    });
  }, [cycleStartDay, generalQ.isSuccess, activateTarget, editId]);
  const billPlanId = activateTarget?.plan_id || form.plan_id;
  const activateCustomer = customers.find(
    (c) => c.id === (activateTarget?.customer_id || form.customer_id),
  );
  const activateClusterId = activateCustomer?.cluster_id || "";
  const activateOffersQ = useQuery({
    queryKey: ["plan-offers", activateClusterId, "activate"],
    queryFn: () => api<OfferOpt[]>(`/api/plan-offers?cluster_id=${activateClusterId}`),
    enabled: (dialogOpen || Boolean(activateTarget)) && Boolean(activateClusterId),
  });

  const offers = (Array.isArray(offersQ.data) ? offersQ.data : []).filter((o) => o.is_active);
  const basePlans = Array.isArray(plansQ.data) ? plansQ.data : [];
  const routers = (Array.isArray(routersQ.data) ? routersQ.data : []).filter(
    (r) => r.is_active && (!clusterId || r.cluster_id === clusterId),
  );
  const allOdps = Array.isArray(odpsQ.data) ? odpsQ.data : [];
  const odps = allOdps.filter(
    (o) =>
      (!clusterId || o.cluster_id === clusterId) &&
      (o.free_ports > 0 || (editId && form.odp_id === o.id)),
  );
  const freePorts = (odpPortsQ.data?.ports ?? []).filter(
    (p) =>
      p.status === "available" ||
      (editId && form.port_number && p.port_number === Number(form.port_number)),
  );
  const selectedOdp =
    odps.find((o) => o.id === form.odp_id) || allOdps.find((o) => o.id === form.odp_id);

  function closeForm() {
    const leavingCreatePage = createMode && !editId;
    setCreateOpen(false);
    setEditId(null);
    setEditStatus(null);
    setEditStartedAt(null);
    setEditNextBillAt(null);
    setForm({ ...emptyForm, customer_id: customerId });
    setBillForm(emptyBillForm());
    setFormErr("");
    if (leavingCreatePage) {
      const n = listQ.data?.data?.length ?? 0;
      if (n > 0) onOpenList();
      else onBack();
    }
  }

  function startEdit(s: SubRow) {
    setCreateOpen(false);
    setEditId(s.id);
    setEditStatus(s.status);
    setEditStartedAt(isoToDateInput(s.started_at));
    setEditNextBillAt(isoToDateInput(s.next_bill_at));
    setFormErr("");
    setForm({
      customer_id: s.customer_id,
      plan_id: s.plan_id,
      router_id: s.router_id || "",
      username: s.username,
      password: "",
      odp_id: s.odp_id || "",
      port_number: s.port_number != null ? String(s.port_number) : "",
    });
    const start = s.started_at ? new Date(s.started_at) : new Date();
    start.setHours(12, 0, 0, 0);
    const next = s.next_bill_at
      ? new Date(s.next_bill_at)
      : defaultNextBill(start, "monthly", true);
    next.setHours(12, 0, 0, 0);
    setBillForm({
      activate_now: s.status === "pending",
      started_at: toDateInput(start),
      next_bill_at: toDateInput(next),
      prorate: true,
    });
  }

  async function postActivate(id: string, f: typeof billForm) {
    return api<{
      status: string;
      prorate_days?: number;
      period_days?: number;
      invoice_total?: number;
    }>(`/api/subscriptions/${id}/activate`, {
      method: "POST",
      body: JSON.stringify({
        started_at: `${f.started_at}T12:00:00`,
        next_bill_at: f.prorate ? `${f.next_bill_at}T12:00:00` : undefined,
        prorate: f.prorate,
      }),
    });
  }

  const create = useMutation({
    mutationFn: async () => {
      if (billForm.activate_now) {
        const err = validateBillDates(billForm);
        if (err) throw new Error(err);
      }
      const body: Record<string, unknown> = {
        customer_id: customerId || form.customer_id,
        plan_id: form.plan_id,
        router_id: form.router_id || undefined,
        username: form.username.trim(),
        password: form.password,
      };
      if (form.odp_id) {
        body.odp_id = form.odp_id;
        if (form.port_number) body.port_number = Number(form.port_number);
      }
      const created = await api<{ id: string }>("/api/subscriptions", {
        method: "POST",
        body: JSON.stringify(body),
      });
      if (billForm.activate_now) {
        const act = await postActivate(created.id, billForm);
        return { created, act, activated: true as const };
      }
      return { created, act: null, activated: false as const };
    },
    onSuccess: (res) => {
      setCreateOpen(false);
      setEditId(null);
      setEditStatus(null);
      setEditStartedAt(null);
      setEditNextBillAt(null);
      setForm({ ...emptyForm, customer_id: customerId });
      setBillForm(emptyBillForm());
      setFormErr("");
      void qc.invalidateQueries({ queryKey: ["subs"] });
      void qc.invalidateQueries({ queryKey: ["odps"] });
      void qc.invalidateQueries({ queryKey: ["ftth-map"] });
      void qc.invalidateQueries({ queryKey: ["invoices"] });
      if (res.activated && res.act?.prorate_days && res.act.period_days) {
        void toastSuccess(
          `Langganan dibuat & diaktifkan (prorata ${res.act.prorate_days}/${res.act.period_days} hari)`,
        );
      } else if (res.activated) {
        void toastSuccess("Langganan dibuat & diaktifkan");
      } else {
        void toastSuccess("Langganan ditambahkan (status pending — belum ditagih)");
      }
      onOpenList();
    },
    onError: (e: Error) => {
      setFormErr(e.message);
      void toastError(e.message);
    },
  });

  const update = useMutation({
    mutationFn: async () => {
      const shouldActivate = editStatus === "pending" && billForm.activate_now;
      if (shouldActivate) {
        const err = validateBillDates(billForm);
        if (err) throw new Error(err);
      }
      const body: Record<string, unknown> = {
        plan_id: form.plan_id,
        router_id: form.router_id || null,
        username: form.username.trim(),
      };
      if (form.password.trim()) body.password = form.password.trim();
      if (!form.odp_id) {
        body.clear_odp = true;
      } else {
        body.odp_id = form.odp_id;
        if (form.port_number) body.port_number = Number(form.port_number);
      }
      await api(`/api/subscriptions/${editId}`, { method: "PUT", body: JSON.stringify(body) });
      if (shouldActivate && editId) {
        const act = await postActivate(editId, billForm);
        return { activated: true as const, act };
      }
      return { activated: false as const, act: null };
    },
    onSuccess: (res) => {
      closeForm();
      void qc.invalidateQueries({ queryKey: ["subs"] });
      void qc.invalidateQueries({ queryKey: ["odps"] });
      void qc.invalidateQueries({ queryKey: ["ftth-map"] });
      void qc.invalidateQueries({ queryKey: ["invoices"] });
      if (res.activated) {
        void toastSuccess("Langganan diperbarui & diaktifkan");
      } else {
        void toastSuccess("Langganan diperbarui");
      }
    },
    onError: (e: Error) => {
      setFormErr(e.message);
      void toastError(e.message);
    },
  });

  const activate = useMutation({
    mutationFn: (payload: {
      id: string;
      started_at: string;
      next_bill_at: string;
      prorate: boolean;
    }) => postActivate(payload.id, { ...payload, activate_now: true }),
    onSuccess: (res) => {
      setActivateTarget(null);
      void qc.invalidateQueries({ queryKey: ["subs"] });
      void qc.invalidateQueries({ queryKey: ["invoices"] });
      if (res?.prorate_days && res.period_days) {
        void toastSuccess(
          `Langganan diaktifkan (prorata ${res.prorate_days}/${res.period_days} hari${
            res.invoice_total != null ? ` · ${formatRp(res.invoice_total)}` : ""
          })`,
        );
      } else {
        void toastSuccess(
          res?.invoice_total != null
            ? `Langganan diaktifkan · invoice ${formatRp(res.invoice_total)}`
            : "Langganan diaktifkan",
        );
      }
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const changePlanCustomer = customers.find((c) => c.id === changePlanTarget?.customer_id);
  const changePlanClusterId = changePlanCustomer?.cluster_id || "";
  const changePlanOffersQ = useQuery({
    queryKey: ["plan-offers", changePlanClusterId, "change-plan"],
    queryFn: () => api<OfferOpt[]>(`/api/plan-offers?cluster_id=${changePlanClusterId}`),
    enabled: Boolean(changePlanTarget) && Boolean(changePlanClusterId),
  });
  const changePlanPlansQ = useQuery({
    queryKey: ["plans", "change-plan"],
    queryFn: () =>
      api<{ id: string; name: string; code: string; price: number; is_active?: boolean }[]>("/api/plans"),
    enabled: Boolean(changePlanTarget),
  });
  const changePlanOptions = changePlanClusterId
    ? (Array.isArray(changePlanOffersQ.data) ? changePlanOffersQ.data : []).filter(
        (o) => o.is_active && o.plan_id !== changePlanTarget?.plan_id,
      )
    : (Array.isArray(changePlanPlansQ.data) ? changePlanPlansQ.data : [])
        .filter((p) => p.is_active !== false && p.id !== changePlanTarget?.plan_id)
        .map((p) => ({
          plan_id: p.id,
          plan_name: p.name,
          plan_code: p.code,
          price: p.price,
          service_type: "",
          is_active: true,
        }));

  const previewChangePlan = useMutation({
    mutationFn: (planId: string) =>
      api<PlanChangeQuote>(`/api/subscriptions/${changePlanTarget!.id}/change-plan/preview`, {
        method: "POST",
        body: JSON.stringify({ plan_id: planId }),
      }),
    onSuccess: (q) => {
      setChangePlanQuote(q);
      setChangePlanErr("");
    },
    onError: (e: Error) => {
      setChangePlanQuote(null);
      setChangePlanErr(e.message);
    },
  });

  const applyChangePlan = useMutation({
    mutationFn: () =>
      api<{
        quote: PlanChangeQuote;
        invoice?: { invoice_number: string; total_amount: number };
        status: string;
      }>(`/api/subscriptions/${changePlanTarget!.id}/change-plan`, {
        method: "POST",
        body: JSON.stringify({ plan_id: changePlanId }),
      }),
    onSuccess: (res) => {
      setChangePlanTarget(null);
      setChangePlanId("");
      setChangePlanQuote(null);
      void qc.invalidateQueries({ queryKey: ["subs"] });
      void qc.invalidateQueries({ queryKey: ["invoices"] });
      const q = res.quote;
      if (q.requires_charge && res.invoice) {
        void toastSuccess(
          `Paket diganti (${q.direction}) · tagihan ${formatRp(res.invoice.total_amount)}`,
        );
      } else if (q.direction === "downgrade") {
        void toastSuccess(
          `Paket diganti (downgrade). Selisih ${formatRp(Math.abs(q.delta_subtotal))} tidak ditagih; harga baru berlaku di siklus berikutnya.`,
        );
      } else {
        void toastSuccess("Paket diganti");
      }
    },
    onError: (e: Error) => void toastError(e.message),
  });

  function openChangePlan(s: SubRow) {
    setChangePlanTarget(s);
    setChangePlanId("");
    setChangePlanQuote(null);
    setChangePlanErr("");
  }

  useEffect(() => {
    if (!changePlanTarget || !changePlanId || changePlanId === changePlanTarget.plan_id) {
      setChangePlanQuote(null);
      return;
    }
    const t = window.setTimeout(() => previewChangePlan.mutate(changePlanId), 200);
    return () => window.clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [changePlanTarget?.id, changePlanId]);

  const activatePlan = (Array.isArray(activatePlansQ.data) ? activatePlansQ.data : []).find(
    (p) => p.id === billPlanId,
  );
  const activateOffer = (Array.isArray(activateOffersQ.data) ? activateOffersQ.data : []).find(
    (o) => o.plan_id === billPlanId && o.is_active,
  );
  const activatePrice = activateOffer?.price ?? activatePlan?.price ?? 0;
  const activateCycle = activatePlan?.billing_cycle || "monthly";
  const activateCycleDays = cycleDaysOf(activateCycle);
  const activateTaxPct = Number(generalQ.data?.default_tax_percent) || 0;
  const activateStart = parseDateInput(billForm.started_at);
  const activateNext = parseDateInput(billForm.next_bill_at);
  let activateProrateDays = 0;
  if (billForm.prorate && activateStart && activateNext && activateNext > activateStart) {
    activateProrateDays = Math.max(
      1,
      Math.ceil((activateNext.getTime() - activateStart.getTime()) / (24 * 60 * 60 * 1000)),
    );
    if (activateProrateDays > activateCycleDays) activateProrateDays = activateCycleDays;
  }
  const activateIsProrated =
    billForm.prorate && activateProrateDays > 0 && activateProrateDays < activateCycleDays;
  const activateSubtotal = activateIsProrated
    ? Math.round((activatePrice * activateProrateDays) / activateCycleDays)
    : activatePrice;
  const activateTax = Math.round((activateSubtotal * activateTaxPct) / 100);
  const activateTotal = activateSubtotal + activateTax;
  useEffect(() => {
    if (!activatePlan) return;
    if (!activateTarget && !dialogOpen) return;
    setBillForm((f) => {
      const start = parseDateInput(f.started_at);
      if (!start) return f;
      return {
        ...f,
        next_bill_at: toDateInput(defaultNextBill(start, activateCycle, f.prorate)),
      };
    });
  }, [activateTarget?.id, editId, activatePlan?.id, activateCycle]);

  const remove = useMutation({
    mutationFn: (id: string) => api(`/api/subscriptions/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["subs"] });
      void qc.invalidateQueries({ queryKey: ["odps"] });
      void qc.invalidateQueries({ queryKey: ["ftth-map"] });
      void toastSuccess("Langganan dihapus");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const saving = create.isPending || update.isPending;

  const sectionTitle = lockedCustomer
    ? createMode
      ? `Buat secret · ${lockedCustomer.full_name}`
      : `Secrets · ${lockedCustomer.full_name}`
    : createMode
      ? "Buat secret"
      : "Secrets";
  const subRows = listQ.data?.data ?? [];

  return (
    <Section
      title={sectionTitle}
      actions={
        createMode || customerCabut ? null : (
          <button type="button" className="btn" onClick={onOpenCreate}>
            + Tambah secret
          </button>
        )
      }
    >
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <button
          type="button"
          className="btn-ghost"
          onClick={() => {
            if (createMode) {
              const n = listQ.data?.data?.length ?? 0;
              if (n > 0) onOpenList();
              else onBack();
              return;
            }
            onBack();
          }}
        >
          {createMode ? "← Kembali" : "← Kembali ke pelanggan"}
        </button>
        {lockedCustomer ? (
          <p className="text-sm text-[var(--muted)]">
            {lockedCustomer.customer_code} — {lockedCustomer.full_name}
            {lockedCustomer.cluster_name ? ` · ${lockedCustomer.cluster_name}` : ""}
            {customerCabut ? " · status cabut" : ""}
          </p>
        ) : null}
      </div>
      {formErr && !dialogOpen && <p className="mb-3 text-sm text-[var(--danger)]">{formErr}</p>}
      {!createMode ? (
        <>
          <p className="mb-3 text-sm text-[var(--muted)]">
            {customerCabut
              ? "Pelanggan ini sudah cabut. Secret lama tetap terlihat di sini; tidak bisa ditambah atau diaktifkan lagi."
              : "Secret khusus pelanggan ini. Aktifkan dengan tanggal/prorata; ikon kotak Ganti paket untuk hitung selisih harga sisa hari."}
          </p>
          <Table
            columns={["Username", "Paket", "ODP / Port", "Status", "Aksi"]}
            rows={subRows.map((s) => [
          s.username,
          s.plan_name,
          s.odp_code
            ? `${s.odp_code}${s.port_number != null ? ` · P${s.port_number}` : ""}`
            : "—",
          s.status === "cancelled" || s.status === "canceled"
            ? customerCabut
              ? "cabut"
              : "dibatalkan"
            : s.status,
          <span key={s.id} className="flex flex-wrap items-center gap-1.5">
            {s.status === "pending" && !customerCabut ? (
              <Button
                type="button"
                size="sm"
                title="Aktifkan langganan (prorata & tanggal)"
                aria-label="Aktifkan langganan"
                disabled={activate.isPending}
                onClick={() => openActivate(s)}
                className="gap-1.5"
              >
                <IconPlug />
                Aktifkan
              </Button>
            ) : null}
            {(s.status === "active" || s.status === "suspended" || s.status === "overdue") && !customerCabut ? (
              <IconButton label="Ganti paket" onClick={() => openChangePlan(s)}>
                <IconBox />
              </IconButton>
            ) : null}
            <IconButton label="Edit langganan" onClick={() => startEdit(s)}>
              <IconPencil />
            </IconButton>
            <IconButton
              label="Hapus langganan"
              danger
              disabled={remove.isPending}
              onClick={async () => {
                const ok = await confirm({
                  title: "Hapus langganan",
                  description: `Hapus langganan ${s.username}? Secret di router akan dicoba dihapus.`,
                  confirmLabel: "Hapus",
                });
                if (ok) remove.mutate(s.id);
              }}
            >
              <IconTrash />
            </IconButton>
          </span>,
        ])}
      />

        </>
      ) : (
        <p className="mb-4 text-sm text-[var(--muted)]">
          Form pembuatan secret hanya untuk pelanggan di atas — tidak bisa diganti ke pelanggan lain.
        </p>
      )}

      <FormDialog
        open={dialogOpen}
        wide
        asPage={createMode && !editId}
        title={editId ? "Edit langganan" : "Buat secret"}
        onClose={closeForm}
      >
        {customerCabut && !editId ? (
          <p className="text-sm text-[var(--muted)]">
            Pelanggan ini sudah cabut. Tidak bisa menambah secret baru.
          </p>
        ) : (
        <form
          className="grid gap-3 sm:grid-cols-2"
          onSubmit={(e) => {
            e.preventDefault();
            if (editId) update.mutate();
            else create.mutate();
          }}
        >
          <p className="text-sm sm:col-span-2 rounded-lg border border-[var(--border)] bg-[var(--panel)] px-3 py-2">
            Pelanggan:{" "}
            <strong>
              {lockedCustomer
                ? `${lockedCustomer.customer_code} — ${lockedCustomer.full_name}`
                : form.customer_id}
            </strong>
          </p>
          {selectedCustomer && !clusterId && (
            <p className="text-xs text-[var(--warn)] sm:col-span-2">
              Pelanggan tanpa cluster: set cluster di halaman Pelanggan agar harga offer dipakai.
            </p>
          )}
          <SearchableSelect
            required
            disabled={
              (Boolean(editId) && editStatus !== "pending") ||
              (Boolean(clusterId) && offers.length === 0)
            }
            placeholder={`— Paket ${clusterId ? "(offer cluster)" : "(harga dasar)"} —`}
            searchPlaceholder="Cari paket…"
            value={form.plan_id}
            onValueChange={(v) => setForm({ ...form, plan_id: v })}
            options={
              clusterId
                ? offers.map((o) => ({
                    value: o.plan_id,
                    label: `${o.plan_name} — ${formatRp(o.price)}`,
                    keywords: o.plan_name,
                  }))
                : basePlans.map((p) => ({
                    value: p.id,
                    label: `${p.name} — ${formatRp(p.price)}`,
                    keywords: `${p.name} ${p.code}`,
                  }))
            }
          />
          {editId && editStatus && editStatus !== "pending" ? (
            <p className="text-xs text-[var(--muted)] sm:col-span-2">
              Ganti paket untuk langganan aktif lewat tombol <strong>Ganti paket</strong> di tabel
              (agar hitung selisih harga).
            </p>
          ) : null}
          <SearchableSelect
            allowClear
            clearLabel="— Router (opsional) —"
            placeholder="— Router (opsional) —"
            searchPlaceholder="Cari router…"
            value={form.router_id}
            onValueChange={(v) => setForm({ ...form, router_id: v })}
            options={routers.map((r) => ({
              value: r.id,
              label: r.name,
              keywords: r.name,
            }))}
          />
          <SearchableSelect
            allowClear
            clearLabel="— ODP (opsional) —"
            placeholder="— ODP (opsional) —"
            searchPlaceholder="Cari kode / nama ODP…"
            value={form.odp_id}
            onValueChange={(v) => setForm({ ...form, odp_id: v, port_number: "" })}
            options={odps.map((o) => ({
              value: o.id,
              label: `${o.code} — ${o.name} · sisa ${o.free_ports}/${o.port_count}`,
              keywords: `${o.code} ${o.name}`,
            }))}
          />
          <select
            className="input"
            value={form.port_number}
            onChange={(e) => setForm({ ...form, port_number: e.target.value })}
            disabled={!form.odp_id}
          >
            <option value="">— Port otomatis (port kosong terendah) —</option>
            {freePorts.map((p) => (
              <option key={p.port_number} value={String(p.port_number)}>
                Port {p.port_number}
                {p.status !== "available" ? " (saat ini)" : ""}
              </option>
            ))}
          </select>
          {form.odp_id && selectedOdp && (
            <p className="text-xs text-[var(--muted)] sm:col-span-2">
              ODP {selectedOdp.code}: terpakai {selectedOdp.used_ports}, sisa {selectedOdp.free_ports}{" "}
              dari {selectedOdp.port_count} port.
            </p>
          )}
          <input
            className="input"
            placeholder="Username PPPoE"
            value={form.username}
            onChange={(e) => setForm({ ...form, username: e.target.value })}
            required
            autoComplete="off"
          />
          <SecretInput
            placeholder={editId ? "Password baru (kosongkan = tetap)" : "Password PPPoE"}
            value={form.password}
            onChange={(e) => setForm({ ...form, password: e.target.value })}
            required={!editId}
            autoComplete="new-password"
          />
          <p className="text-xs text-[var(--muted)] sm:col-span-2">
            {editId
              ? "Untuk langganan aktif/suspend: simpan akan sync ulang secret ke RouterOS (password, profil paket, username)."
              : "Password dipakai saat aktivasi ke RouterOS. Komentar secret: kode + nama pelanggan."}
          </p>

          {editId ? (
            <p className="rounded-lg border border-[var(--border)] bg-[var(--panel)] px-3 py-2 text-sm sm:col-span-2">
              Status: <strong>{editStatus || "—"}</strong>
              {editStatus && editStatus !== "pending" ? (
                <>
                  {" · "}Mulai: {editStartedAt || "—"}
                  {" · "}Tagihan berikutnya: {editNextBillAt || "—"}
                </>
              ) : null}
            </p>
          ) : null}

          {!editId || editStatus === "pending" ? (
            <div className="sm:col-span-2 grid gap-3 rounded-lg border border-[var(--border)] p-3">
              <label className="flex items-center gap-2 text-sm">
                <Checkbox
                  checked={billForm.activate_now}
                  onCheckedChange={(v) => {
                    const activate_now = v === true;
                    const start = parseDateInput(billForm.started_at) || new Date();
                    setBillForm((f) => ({
                      ...f,
                      activate_now,
                      next_bill_at: toDateInput(defaultNextBill(start, activateCycle, f.prorate)),
                    }));
                  }}
                />
                <span>{editId ? "Aktifkan saat simpan (set tanggal & buat invoice)" : "Aktifkan sekarang (set tanggal & buat invoice)"}</span>
              </label>
              {!billForm.activate_now ? (
                <p className="text-xs text-[var(--muted)]">
                  Tanpa aktivasi: status tetap <strong>pending</strong>. Belum sync billing/invoice;
                  secret bisa diaktifkan nanti.
                </p>
              ) : (
                <>
                  <label className="grid gap-1 text-sm">
                    <span>Tanggal mulai</span>
                    <Input
                      type="date"
                      required
                      value={billForm.started_at}
                      onChange={(e) => {
                        const started_at = e.target.value;
                        const start = parseDateInput(started_at);
                        setBillForm((f) => ({
                          ...f,
                          started_at,
                          next_bill_at: start
                            ? toDateInput(defaultNextBill(start, activateCycle, f.prorate))
                            : f.next_bill_at,
                        }));
                      }}
                    />
                  </label>
                  <label className="flex items-center gap-2 text-sm">
                    <Checkbox
                      checked={billForm.prorate}
                      onCheckedChange={(v) => {
                        const prorate = v === true;
                        const start = parseDateInput(billForm.started_at) || new Date();
                        setBillForm((f) => ({
                          ...f,
                          prorate,
                          next_bill_at: toDateInput(defaultNextBill(start, activateCycle, prorate)),
                        }));
                      }}
                    />
                    <span>Prorata tagihan pertama</span>
                  </label>
                  {billForm.prorate ? (
                    <label className="grid gap-1 text-sm">
                      <span>Tanggal tagihan berikutnya</span>
                      <Input
                        type="date"
                        required
                        value={billForm.next_bill_at}
                        onChange={(e) => setBillForm({ ...billForm, next_bill_at: e.target.value })}
                      />
                      <span className="text-xs text-[var(--muted)]">
                        Default: tanggal {cycleStartDay} (Pengaturan → Umum). Bisa diubah manual.
                      </span>
                    </label>
                  ) : (
                    <p className="text-xs text-[var(--muted)]">
                      Tanpa prorata: tagihan penuh 1 siklus ({activateCycle}).
                    </p>
                  )}
                  <div className="rounded-lg border border-[var(--border)] bg-[var(--panel)] p-3 text-sm">
                    <div className="flex justify-between gap-2">
                      <span className="text-[var(--muted)]">Harga paket</span>
                      <span>{formatRp(activatePrice)}</span>
                    </div>
                    {activateIsProrated ? (
                      <div className="mt-1 flex justify-between gap-2">
                        <span className="text-[var(--muted)]">
                          Prorata {activateProrateDays}/{activateCycleDays} hari
                        </span>
                        <span>{formatRp(activateSubtotal)}</span>
                      </div>
                    ) : (
                      <div className="mt-1 flex justify-between gap-2">
                        <span className="text-[var(--muted)]">Tagihan penuh</span>
                        <span>{formatRp(activateSubtotal)}</span>
                      </div>
                    )}
                    {activateTaxPct > 0 ? (
                      <div className="mt-1 flex justify-between gap-2">
                        <span className="text-[var(--muted)]">Pajak ({activateTaxPct}%)</span>
                        <span>{formatRp(activateTax)}</span>
                      </div>
                    ) : null}
                    <div className="mt-2 flex justify-between gap-2 border-t border-[var(--border)] pt-2 font-medium">
                      <span>Estimasi invoice</span>
                      <span>{formatRp(activateTotal)}</span>
                    </div>
                  </div>
                </>
              )}
            </div>
          ) : null}

          {clusterId && offers.length === 0 && (
            <p className="text-sm text-[var(--danger)] sm:col-span-2">
              Belum ada offer paket untuk cluster ini.
            </p>
          )}
          <div className="flex flex-wrap gap-2 sm:col-span-2">
            <button className="btn" disabled={saving || (Boolean(clusterId) && !form.plan_id)}>
              {saving
                ? "Menyimpan…"
                : editId
                  ? billForm.activate_now && editStatus === "pending"
                    ? "Simpan & aktifkan"
                    : "Simpan"
                  : billForm.activate_now
                    ? "Buat & aktifkan"
                    : "Buat (pending)"}
            </button>
            <button type="button" className="btn-ghost" onClick={closeForm}>
              Batal
            </button>
          </div>
          {formErr && <p className="text-sm text-[var(--danger)] sm:col-span-2">{formErr}</p>}
        </form>
        )}
      </FormDialog>

      <FormDialog
        open={Boolean(activateTarget)}
        title="Aktifkan langganan"
        onClose={() => setActivateTarget(null)}
      >
        <form
          className="grid gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (!activateTarget) return;
            if (!billForm.started_at) {
              void toastError("Tanggal mulai wajib diisi");
              return;
            }
            if (billForm.prorate && !billForm.next_bill_at) {
              void toastError("Tanggal tagihan berikutnya wajib diisi");
              return;
            }
            const start = parseDateInput(billForm.started_at);
            const next = parseDateInput(billForm.next_bill_at);
            if (billForm.prorate && start && next && !(next > start)) {
              void toastError("Tanggal tagihan berikutnya harus setelah tanggal mulai");
              return;
            }
            activate.mutate({
              id: activateTarget.id,
              started_at: billForm.started_at,
              next_bill_at: billForm.next_bill_at,
              prorate: billForm.prorate,
            });
          }}
        >
          <p className="text-sm text-[var(--muted)]">
            {activateTarget
              ? `${activateTarget.username} · ${activateTarget.customer_name} · ${activateTarget.plan_name}`
              : ""}
          </p>
          <label className="grid gap-1 text-sm">
            <span>Tanggal mulai</span>
            <Input
              type="date"
              required
              value={billForm.started_at}
              onChange={(e) => {
                const started_at = e.target.value;
                const start = parseDateInput(started_at);
                setBillForm((f) => ({
                  ...f,
                  started_at,
                  next_bill_at: start
                    ? toDateInput(defaultNextBill(start, activateCycle, f.prorate))
                    : f.next_bill_at,
                }));
              }}
            />
          </label>
          <label className="flex items-center gap-2 text-sm">
            <Checkbox
              checked={billForm.prorate}
              onCheckedChange={(v) => {
                const prorate = v === true;
                const start = parseDateInput(billForm.started_at) || new Date();
                setBillForm((f) => ({
                  ...f,
                  prorate,
                  next_bill_at: toDateInput(defaultNextBill(start, activateCycle, prorate)),
                }));
              }}
            />
            <span>Prorata tagihan pertama</span>
          </label>
          {billForm.prorate ? (
            <label className="grid gap-1 text-sm">
              <span>Tanggal tagihan berikutnya</span>
              <Input
                type="date"
                required
                value={billForm.next_bill_at}
                onChange={(e) => setBillForm({ ...billForm, next_bill_at: e.target.value })}
              />
              <span className="text-xs text-[var(--muted)]">
                Tagihan pertama dihitung dari tanggal mulai sampai tanggal ini. Default: tanggal{" "}
                {cycleStartDay} (Pengaturan → Umum), bisa diubah manual.
              </span>
            </label>
          ) : (
            <p className="text-xs text-[var(--muted)]">
              Tanpa prorata: tagihan penuh 1 siklus ({activateCycle}), next bill = tanggal mulai +
              siklus.
            </p>
          )}
          <div className="rounded-lg border border-[var(--border)] bg-[var(--panel)] p-3 text-sm">
            <div className="flex justify-between gap-2">
              <span className="text-[var(--muted)]">Harga paket</span>
              <span>{formatRp(activatePrice)}</span>
            </div>
            {activateIsProrated ? (
              <div className="mt-1 flex justify-between gap-2">
                <span className="text-[var(--muted)]">
                  Prorata {activateProrateDays}/{activateCycleDays} hari
                </span>
                <span>{formatRp(activateSubtotal)}</span>
              </div>
            ) : (
              <div className="mt-1 flex justify-between gap-2">
                <span className="text-[var(--muted)]">Tagihan penuh</span>
                <span>{formatRp(activateSubtotal)}</span>
              </div>
            )}
            {activateTaxPct > 0 ? (
              <div className="mt-1 flex justify-between gap-2">
                <span className="text-[var(--muted)]">Pajak ({activateTaxPct}%)</span>
                <span>{formatRp(activateTax)}</span>
              </div>
            ) : null}
            <div className="mt-2 flex justify-between gap-2 border-t border-[var(--border)] pt-2 font-medium">
              <span>Estimasi invoice</span>
              <span>{formatRp(activateTotal)}</span>
            </div>
          </div>
          <div className="flex flex-wrap gap-2">
            <button className="btn" disabled={activate.isPending}>
              {activate.isPending ? "Mengaktifkan…" : "Aktifkan"}
            </button>
            <button type="button" className="btn-ghost" onClick={() => setActivateTarget(null)}>
              Batal
            </button>
          </div>
        </form>
      </FormDialog>

      <FormDialog
        open={Boolean(changePlanTarget)}
        title="Ganti paket"
        onClose={() => {
          setChangePlanTarget(null);
          setChangePlanId("");
          setChangePlanQuote(null);
          setChangePlanErr("");
        }}
      >
        <form
          className="grid gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (!changePlanId) {
              void toastError("Pilih paket baru");
              return;
            }
            applyChangePlan.mutate();
          }}
        >
          <p className="text-sm text-[var(--muted)]">
            {changePlanTarget
              ? `${changePlanTarget.username} · ${changePlanTarget.customer_name} · saat ini: ${changePlanTarget.plan_name}`
              : ""}
          </p>
          <SearchableSelect
            required
            placeholder="— Paket baru —"
            searchPlaceholder="Cari paket…"
            value={changePlanId}
            onValueChange={(v) => setChangePlanId(v)}
            options={changePlanOptions.map((o) => ({
              value: o.plan_id,
              label: `${o.plan_name} — ${formatRp(o.price)}`,
              keywords: `${o.plan_name} ${o.plan_code || ""}`,
            }))}
          />
          {changePlanCustomer && !changePlanClusterId ? (
            <p className="text-xs text-[var(--warn)]">
              Pelanggan tanpa cluster: harga dasar paket dipakai.
            </p>
          ) : null}
          {previewChangePlan.isPending ? (
            <p className="text-sm text-[var(--muted)]">Menghitung selisih harga…</p>
          ) : null}
          {changePlanErr ? <p className="text-sm text-[var(--danger)]">{changePlanErr}</p> : null}
          {changePlanQuote ? (
            <div className="rounded-lg border border-[var(--border)] bg-[var(--panel)] p-3 text-sm">
              <div className="mb-2 text-xs uppercase tracking-wide text-[var(--muted)]">
                {changePlanQuote.direction === "upgrade"
                  ? "Upgrade"
                  : changePlanQuote.direction === "downgrade"
                    ? "Downgrade"
                    : "Sama harga"}{" "}
                · sisa {changePlanQuote.remaining_days}/{changePlanQuote.period_days} hari
              </div>
              <div className="flex justify-between gap-2">
                <span className="text-[var(--muted)]">Harga lama (penuh)</span>
                <span>{formatRp(changePlanQuote.old_price)}</span>
              </div>
              <div className="mt-1 flex justify-between gap-2">
                <span className="text-[var(--muted)]">Harga baru (penuh)</span>
                <span>{formatRp(changePlanQuote.new_price)}</span>
              </div>
              <div className="mt-1 flex justify-between gap-2">
                <span className="text-[var(--muted)]">Kredit sisa paket lama</span>
                <span>−{formatRp(changePlanQuote.old_credit)}</span>
              </div>
              <div className="mt-1 flex justify-between gap-2">
                <span className="text-[var(--muted)]">Prorata paket baru</span>
                <span>{formatRp(changePlanQuote.new_charge)}</span>
              </div>
              <div className="mt-1 flex justify-between gap-2">
                <span className="text-[var(--muted)]">Selisih</span>
                <span>
                  {changePlanQuote.delta_subtotal < 0 ? "−" : ""}
                  {formatRp(Math.abs(changePlanQuote.delta_subtotal))}
                </span>
              </div>
              {changePlanQuote.tax_amount > 0 ? (
                <div className="mt-1 flex justify-between gap-2">
                  <span className="text-[var(--muted)]">Pajak</span>
                  <span>{formatRp(changePlanQuote.tax_amount)}</span>
                </div>
              ) : null}
              <div className="mt-2 flex justify-between gap-2 border-t border-[var(--border)] pt-2 font-medium">
                <span>Tagihan sekarang</span>
                <span>
                  {changePlanQuote.requires_charge
                    ? formatRp(changePlanQuote.total_amount)
                    : "Rp 0 (tanpa tagihan)"}
                </span>
              </div>
              <p className="mt-2 text-xs text-[var(--muted)]">
                Tanggal tagihan berikutnya tidak berubah. Siklus berikutnya memakai harga paket baru
                penuh.
                {changePlanQuote.direction === "downgrade"
                  ? " Downgrade: selisih negatif tidak diganti uang; langsung pakai paket baru."
                  : ""}
              </p>
            </div>
          ) : null}
          <div className="flex flex-wrap gap-2">
            <button
              className="btn"
              disabled={
                applyChangePlan.isPending ||
                !changePlanId ||
                previewChangePlan.isPending ||
                Boolean(changePlanErr)
              }
            >
              {applyChangePlan.isPending ? "Memproses…" : "Konfirmasi ganti paket"}
            </button>
            <button
              type="button"
              className="btn-ghost"
              onClick={() => {
                setChangePlanTarget(null);
                setChangePlanId("");
                setChangePlanQuote(null);
                setChangePlanErr("");
              }}
            >
              Batal
            </button>
          </div>
        </form>
      </FormDialog>
    </Section>
  );
}

