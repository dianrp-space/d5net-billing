import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, apiUpload } from "./api";
import { compressImageForUpload } from "./imageCompress";
import { useAppDialog } from "./confirm";
import { Badge } from "./components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { IconCheck, IconEye, IconMapPin, IconUserCheck } from "./icons";
import { canDispatchOps, type MePermissions } from "./permissions";
import { toastError, toastSuccess } from "./swal";
import {
  Button,
  FormDialog,
  IconButton,
  Input,
  Label,
  SearchableSelect,
  Section,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Table,
} from "./ui";

type WorkOrder = {
  id: string;
  type: string;
  status: string;
  customer_id?: string | null;
  customer_name?: string;
  customer_code?: string;
  technician_id?: string | null;
  technician_name?: string;
  scheduled_at?: string | null;
  check_in_at?: string | null;
  check_in_lat?: number | null;
  check_in_lng?: number | null;
  completed_at?: string | null;
  notes?: string | null;
  photos?: string[];
  created_at?: string;
};

type Technician = {
  id: string;
  full_name: string;
  phone: string;
  email?: string;
  user_id?: string | null;
  is_active: boolean;
};

type CustomerOpt = { id: string; full_name: string; customer_code: string };

const statusLabels: Record<string, string> = {
  pending: "Menunggu",
  assigned: "Diassign",
  in_progress: "Proses",
  completed: "Selesai",
  cancelled: "Batal",
};

const typeLabels: Record<string, string> = {
  installation: "Instalasi",
  repair: "Perbaikan",
  survey: "Survey",
  maintenance: "Maintenance",
  other: "Lainnya",
};

const emptyWO = {
  type: "installation",
  notes: "",
  customer_id: "",
  technician_id: "",
  scheduled_at: "",
};

function formatWhen(iso?: string | null) {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleString("id-ID");
  } catch {
    return iso;
  }
}

function statusVariant(s: string): "default" | "danger" | "outline" | "success" {
  if (s === "completed") return "success";
  if (s === "cancelled") return "danger";
  if (s === "in_progress") return "default";
  return "outline";
}

function readGeo(): Promise<{ lat: number; lng: number }> {
  return new Promise((resolve, reject) => {
    if (!navigator.geolocation) {
      reject(new Error("Geolocation tidak tersedia di browser ini"));
      return;
    }
    navigator.geolocation.getCurrentPosition(
      (pos) => resolve({ lat: pos.coords.latitude, lng: pos.coords.longitude }),
      (err) => reject(new Error(err.message || "Gagal ambil lokasi GPS")),
      { enableHighAccuracy: true, timeout: 15000 },
    );
  });
}

export function TechPage() {
  const qc = useQueryClient();
  const { confirm } = useAppDialog();
  const [tab, setTab] = useState("orders");
  const [status, setStatus] = useState("");
  const [page, setPage] = useState(0);
  const limit = 20;

  const [openWO, setOpenWO] = useState(false);
  const [form, setForm] = useState(emptyWO);
  const [formErr, setFormErr] = useState("");

  const [detail, setDetail] = useState<WorkOrder | null>(null);
  const [assignOpen, setAssignOpen] = useState<WorkOrder | null>(null);
  const [assignTech, setAssignTech] = useState("");
  const [completeNotes, setCompleteNotes] = useState("");
  const photoRef = useRef<HTMLInputElement>(null);

  const meQ = useQuery({
    queryKey: ["me"],
    queryFn: () => api<MePermissions & { user_id: string }>("/api/me"),
  });
  const canDispatch = canDispatchOps(meQ.data?.permissions);

  const q = useQuery({
    queryKey: ["work-orders", status, page],
    queryFn: () =>
      api<{ data: WorkOrder[]; total: number }>(
        `/api/work-orders?limit=${limit}&offset=${page * limit}${status ? `&status=${encodeURIComponent(status)}` : ""}`,
      ),
  });
  const techQ = useQuery({
    queryKey: ["technicians"],
    queryFn: () => api<Technician[]>("/api/technicians"),
  });
  const customersQ = useQuery({
    queryKey: ["customers"],
    queryFn: () => api<{ data: CustomerOpt[] }>("/api/customers?limit=500"),
    enabled: canDispatch,
  });

  const list = q.data?.data ?? [];
  const total = q.data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / limit));
  const technicians = Array.isArray(techQ.data) ? techQ.data : [];
  const activeTechs = technicians.filter((t) => t.is_active);
  const customers = customersQ.data?.data ?? [];

  async function refreshDetail(id: string) {
    try {
      const wo = await api<WorkOrder>(`/api/work-orders/${id}`);
      setDetail(wo);
    } catch {
      /* keep stale */
    }
  }

  const createWO = useMutation({
    mutationFn: () =>
      api<WorkOrder>("/api/work-orders", {
        method: "POST",
        body: JSON.stringify({
          type: form.type,
          notes: form.notes.trim() || undefined,
          customer_id: form.customer_id || undefined,
          technician_id: form.technician_id || undefined,
          scheduled_at: form.scheduled_at ? new Date(form.scheduled_at).toISOString() : undefined,
        }),
      }),
    onSuccess: () => {
      setOpenWO(false);
      setForm(emptyWO);
      setFormErr("");
      qc.invalidateQueries({ queryKey: ["work-orders"] });
      void toastSuccess("Work order dibuat");
    },
    onError: (e: Error) => {
      setFormErr(e.message);
      void toastError(e.message);
    },
  });

  const assignMut = useMutation({
    mutationFn: ({ id, technician_id }: { id: string; technician_id?: string }) =>
      api(`/api/work-orders/${id}`, {
        method: "PATCH",
        body: JSON.stringify(
          technician_id
            ? { technician_id }
            : { clear_technician: true, status: "pending" },
        ),
      }),
    onSuccess: () => {
      setAssignOpen(null);
      setAssignTech("");
      qc.invalidateQueries({ queryKey: ["work-orders"] });
      void toastSuccess("Teknisi di-assign");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const checkinMut = useMutation({
    mutationFn: async (id: string) => {
      const geo = await readGeo();
      return api<WorkOrder>(`/api/work-orders/${id}/check-in`, {
        method: "POST",
        body: JSON.stringify(geo),
      });
    },
    onSuccess: (wo) => {
      qc.invalidateQueries({ queryKey: ["work-orders"] });
      setDetail(wo);
      void toastSuccess("Check-in GPS berhasil");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const completeMut = useMutation({
    mutationFn: (id: string) =>
      api<WorkOrder>(`/api/work-orders/${id}/complete`, {
        method: "POST",
        body: JSON.stringify({ notes: completeNotes.trim() || undefined }),
      }),
    onSuccess: (wo) => {
      setCompleteNotes("");
      qc.invalidateQueries({ queryKey: ["work-orders"] });
      setDetail(wo);
      void toastSuccess("Work order selesai");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const cancelMut = useMutation({
    mutationFn: (id: string) =>
      api(`/api/work-orders/${id}`, {
        method: "PATCH",
        body: JSON.stringify({ status: "cancelled" }),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["work-orders"] });
      if (detail) void refreshDetail(detail.id);
      void toastSuccess("Work order dibatalkan");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  const photoMut = useMutation({
    mutationFn: async ({ id, file }: { id: string; file: File }) => {
      const compressed = await compressImageForUpload(file);
      return apiUpload<WorkOrder>(`/api/work-orders/${id}/photos`, compressed);
    },
    onSuccess: (wo) => {
      qc.invalidateQueries({ queryKey: ["work-orders"] });
      setDetail(wo);
      void toastSuccess("Foto diunggah");
    },
    onError: (e: Error) => void toastError(e.message),
  });

  return (
    <>
      <Section title="Portal teknisi">
        <p className="mb-4 text-sm text-[var(--muted)]">
          {canDispatch
            ? "Work order lapangan: buat, assign teknisi, pantau check-in GPS dan penyelesaian."
            : "Work order yang di-assign ke Anda: check-in GPS, unggah foto, dan selesaikan. Pembuatan/assign hanya oleh admin."}
        </p>

        <Tabs value={canDispatch ? tab : "orders"} onValueChange={setTab}>
          <TabsList className="mb-4">
            <TabsTrigger value="orders">Work order</TabsTrigger>
            {canDispatch ? <TabsTrigger value="techs">Daftar teknisi</TabsTrigger> : null}
          </TabsList>

          <TabsContent value="orders" className="space-y-4">
            <div className="flex flex-wrap items-end justify-between gap-3">
              <div className="min-w-[180px]">
                <Label className="mb-1.5 block">Status</Label>
                <Select
                  value={status || "__all__"}
                  onValueChange={(v) => {
                    setStatus(v === "__all__" ? "" : v);
                    setPage(0);
                  }}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="__all__">Semua</SelectItem>
                    {Object.entries(statusLabels).map(([k, label]) => (
                      <SelectItem key={k} value={k}>
                        {label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              {canDispatch ? (
                <Button
                  type="button"
                  onClick={() => {
                    setForm(emptyWO);
                    setFormErr("");
                    setOpenWO(true);
                  }}
                >
                  + Work order
                </Button>
              ) : null}
            </div>

            <Table
              columns={["Tipe", "Pelanggan", "Teknisi", "Jadwal", "Status", "Aksi"]}
              rows={list.map((wo) => [
                <div key={`${wo.id}-t`}>
                  <p className="font-medium">{typeLabels[wo.type] || wo.type}</p>
                  <p className="text-xs text-[var(--muted)]">{formatWhen(wo.created_at)}</p>
                </div>,
                wo.customer_name
                  ? `${wo.customer_name}${wo.customer_code ? ` (${wo.customer_code})` : ""}`
                  : "—",
                wo.technician_name || "—",
                formatWhen(wo.scheduled_at),
                <Badge key={`${wo.id}-s`} variant={statusVariant(wo.status)}>
                  {statusLabels[wo.status] || wo.status}
                </Badge>,
                <span key={`${wo.id}-a`} className="flex flex-wrap items-center gap-1.5">
                  <IconButton
                    label="Detail"
                    onClick={() => {
                      setDetail(wo);
                      setCompleteNotes("");
                      void refreshDetail(wo.id);
                    }}
                  >
                    <IconEye />
                  </IconButton>
                  {wo.status !== "completed" && wo.status !== "cancelled" && (
                    <>
                      {canDispatch ? (
                        <IconButton
                          label="Assign teknisi"
                          onClick={() => {
                            setAssignOpen(wo);
                            setAssignTech(wo.technician_id || "");
                          }}
                        >
                          <IconUserCheck />
                        </IconButton>
                      ) : null}
                      <IconButton
                        label="Check-in GPS"
                        disabled={checkinMut.isPending}
                        onClick={() => {
                          void confirm({
                            title: "Check-in GPS?",
                            description: "Lokasi perangkat akan dikirim sebagai check-in.",
                            confirmLabel: "Check-in",
                          }).then((ok) => {
                            if (ok) checkinMut.mutate(wo.id);
                          });
                        }}
                      >
                        <IconMapPin />
                      </IconButton>
                      <IconButton
                        label="Selesaikan"
                        onClick={() => {
                          setDetail(wo);
                          setCompleteNotes("");
                        }}
                      >
                        <IconCheck />
                      </IconButton>
                    </>
                  )}
                </span>,
              ])}
            />

            {total > limit && (
              <div className="flex items-center justify-between gap-2 text-sm">
                <span className="text-[var(--muted)]">
                  {total} work order · halaman {page + 1}/{pages}
                </span>
                <div className="flex gap-2">
                  <Button type="button" variant="outline" disabled={page <= 0} onClick={() => setPage((p) => p - 1)}>
                    Sebelumnya
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    disabled={page + 1 >= pages}
                    onClick={() => setPage((p) => p + 1)}
                  >
                    Berikutnya
                  </Button>
                </div>
              </div>
            )}
          </TabsContent>

          {canDispatch ? (
            <TabsContent value="techs" className="space-y-4">
              <p className="text-sm text-[var(--muted)]">
                Daftar ini diisi otomatis dari user dengan role <code className="text-xs">teknisi</code> (atau izin
                ops/tech). Tambah/ubah lewat Settings → Users.
              </p>
              <Table
                columns={["Nama", "Kontak", "Status"]}
                rows={technicians.map((t) => [
                  <div key={`${t.id}-n`}>
                    <p className="font-medium">{t.full_name}</p>
                    {t.email ? <p className="text-xs text-[var(--muted)]">{t.email}</p> : null}
                  </div>,
                  t.phone && t.phone !== "-" ? t.phone : "—",
                  <Badge key={`${t.id}-s`} variant={t.is_active ? "success" : "outline"}>
                    {t.is_active ? "Aktif" : "Nonaktif"}
                  </Badge>,
                ])}
              />
              {technicians.length === 0 && !techQ.isLoading && (
                <p className="text-sm text-[var(--muted)]">
                  {techQ.isError
                    ? `Gagal memuat daftar: ${(techQ.error as Error)?.message || "error"}`
                    : "Belum ada teknisi. Buat user di Settings → Users dengan role teknisi."}
                </p>
              )}
            </TabsContent>
          ) : null}
        </Tabs>
      </Section>

      <FormDialog
        open={openWO}
        onClose={() => setOpenWO(false)}
        title="Buat work order"
      >
        <form
          className="grid gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            createWO.mutate();
          }}
        >
          <div>
            <Label className="mb-1.5 block">Tipe</Label>
            <Select value={form.type} onValueChange={(v) => setForm({ ...form, type: v })}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Object.entries(typeLabels).map(([k, label]) => (
                  <SelectItem key={k} value={k}>
                    {label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div>
            <Label className="mb-1.5 block">Pelanggan</Label>
            <SearchableSelect
              allowClear
              clearLabel="— Tanpa pelanggan —"
              placeholder="Opsional"
              searchPlaceholder="Cari nama / kode…"
              value={form.customer_id}
              onValueChange={(v) => setForm({ ...form, customer_id: v })}
              options={customers.map((c) => ({
                value: c.id,
                label: `${c.full_name} (${c.customer_code})`,
                keywords: `${c.full_name} ${c.customer_code}`,
              }))}
            />
          </div>
          <div>
            <Label className="mb-1.5 block">Teknisi</Label>
            <SearchableSelect
              allowClear
              clearLabel="— Belum diassign —"
              placeholder="Opsional"
              searchPlaceholder="Cari teknisi…"
              value={form.technician_id}
              onValueChange={(v) => setForm({ ...form, technician_id: v })}
              options={activeTechs.map((t) => ({
                value: t.id,
                label: t.full_name,
                keywords: t.full_name,
              }))}
            />
          </div>
          <div>
            <Label className="mb-1.5 block">Jadwal</Label>
            <Input
              type="datetime-local"
              value={form.scheduled_at}
              onChange={(e) => setForm({ ...form, scheduled_at: e.target.value })}
            />
          </div>
          <div>
            <Label className="mb-1.5 block">Catatan</Label>
            <Input
              value={form.notes}
              onChange={(e) => setForm({ ...form, notes: e.target.value })}
              placeholder="Alamat / keluhan / instruksi"
            />
          </div>
          {formErr && <p className="text-sm text-[var(--danger)]">{formErr}</p>}
          <div className="flex justify-end gap-2">
            <Button type="button" variant="outline" onClick={() => setOpenWO(false)}>
              Batal
            </Button>
            <Button type="submit" disabled={createWO.isPending}>
              {createWO.isPending ? "Menyimpan…" : "Buat"}
            </Button>
          </div>
        </form>
      </FormDialog>

      <FormDialog
        open={Boolean(assignOpen)}
        onClose={() => setAssignOpen(null)}
        title="Assign teknisi"
      >
        <div className="grid gap-3">
          <div>
            <Label className="mb-1.5 block">Teknisi</Label>
            <Select value={assignTech || "__none__"} onValueChange={(v) => setAssignTech(v === "__none__" ? "" : v)}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__none__">— Lepas assign —</SelectItem>
                {activeTechs.map((t) => (
                  <SelectItem key={t.id} value={t.id}>
                    {t.full_name} · {t.phone}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="flex justify-end gap-2">
            <Button type="button" variant="outline" onClick={() => setAssignOpen(null)}>
              Batal
            </Button>
            <Button
              type="button"
              disabled={assignMut.isPending || !assignOpen}
              onClick={() => {
                if (!assignOpen) return;
                assignMut.mutate({ id: assignOpen.id, technician_id: assignTech || undefined });
              }}
            >
              Simpan
            </Button>
          </div>
        </div>
      </FormDialog>

      <FormDialog
        open={Boolean(detail)}
        onClose={() => setDetail(null)}
        title="Detail work order"
        wide
      >
        {detail && (
          <div className="grid gap-3 text-sm">
            <div className="grid gap-1 rounded-md border border-[var(--border)] p-3">
              <p>
                <span className="text-[var(--muted)]">Status:</span>{" "}
                <Badge variant={statusVariant(detail.status)}>{statusLabels[detail.status] || detail.status}</Badge>
              </p>
              <p>
                <span className="text-[var(--muted)]">Pelanggan:</span> {detail.customer_name || "—"}
              </p>
              <p>
                <span className="text-[var(--muted)]">Teknisi:</span> {detail.technician_name || "—"}
              </p>
              <p>
                <span className="text-[var(--muted)]">Jadwal:</span> {formatWhen(detail.scheduled_at)}
              </p>
              <p>
                <span className="text-[var(--muted)]">Check-in:</span>{" "}
                {detail.check_in_at
                  ? `${formatWhen(detail.check_in_at)}${
                      detail.check_in_lat != null && detail.check_in_lng != null
                        ? ` (${detail.check_in_lat.toFixed(5)}, ${detail.check_in_lng.toFixed(5)})`
                        : ""
                    }`
                  : "—"}
              </p>
              <p>
                <span className="text-[var(--muted)]">Selesai:</span> {formatWhen(detail.completed_at)}
              </p>
              <p className="whitespace-pre-wrap">
                <span className="text-[var(--muted)]">Catatan:</span> {detail.notes || "—"}
              </p>
            </div>

            {(detail.photos?.length ?? 0) > 0 && (
              <div className="flex flex-wrap gap-2">
                {detail.photos!.map((url) => (
                  <a key={url} href={url} target="_blank" rel="noreferrer" className="block overflow-hidden rounded border border-[var(--border)]">
                    <img src={url} alt="" className="h-20 w-20 object-cover" />
                  </a>
                ))}
              </div>
            )}

            {detail.status !== "completed" && detail.status !== "cancelled" && (
              <>
                <div className="flex flex-wrap gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    disabled={checkinMut.isPending}
                    onClick={() => checkinMut.mutate(detail.id)}
                  >
                    Check-in GPS
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    disabled={photoMut.isPending}
                    onClick={() => photoRef.current?.click()}
                  >
                    Unggah foto
                  </Button>
                  <input
                    ref={photoRef}
                    type="file"
                    accept="image/*"
                    capture="environment"
                    className="hidden"
                    onChange={(e) => {
                      const file = e.target.files?.[0];
                      e.target.value = "";
                      if (file) photoMut.mutate({ id: detail.id, file });
                    }}
                  />
                  {canDispatch ? (
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => {
                        void confirm({
                          title: "Batalkan work order?",
                          description: "Status akan jadi dibatalkan.",
                          confirmLabel: "Batalkan",
                          danger: true,
                        }).then((ok) => {
                          if (ok) cancelMut.mutate(detail.id);
                        });
                      }}
                    >
                      Batalkan
                    </Button>
                  ) : null}
                </div>
                <div>
                  <Label className="mb-1.5 block">Catatan penyelesaian</Label>
                  <Input
                    value={completeNotes}
                    onChange={(e) => setCompleteNotes(e.target.value)}
                    placeholder="Ringkasan pekerjaan di lapangan"
                  />
                </div>
                <Button
                  type="button"
                  disabled={completeMut.isPending}
                  onClick={() => completeMut.mutate(detail.id)}
                >
                  {completeMut.isPending ? "Menyimpan…" : "Tandai selesai"}
                </Button>
              </>
            )}
          </div>
        )}
      </FormDialog>
    </>
  );
}
