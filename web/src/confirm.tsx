import { createContext, useContext, useMemo, type ReactNode } from "react";
import { swalAlert, swalConfirm, type ConfirmOptions } from "./swal";

export type { ConfirmOptions };

type ConfirmFn = (opts: ConfirmOptions | string) => Promise<boolean>;
type AlertFn = (opts: { title?: string; description: string } | string) => Promise<void>;

type DialogApi = {
  confirm: ConfirmFn;
  alert: AlertFn;
};

const DialogApiContext = createContext<DialogApi | null>(null);

/** Provider retained for API compatibility; dialogs use SweetAlert2. */
export function AppDialogProvider({ children }: { children: ReactNode }) {
  const api = useMemo<DialogApi>(
    () => ({
      confirm: swalConfirm,
      alert: swalAlert,
    }),
    [],
  );

  return <DialogApiContext.Provider value={api}>{children}</DialogApiContext.Provider>;
}

export function useAppDialog(): DialogApi {
  const ctx = useContext(DialogApiContext);
  if (!ctx) {
    throw new Error("useAppDialog must be used within AppDialogProvider");
  }
  return ctx;
}

export function useConfirm() {
  return useAppDialog().confirm;
}
