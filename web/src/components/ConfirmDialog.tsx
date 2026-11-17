import { Button } from "./ui/Button";
import { Dialog } from "./ui/Dialog";

/** Confirm is a pending confirmation: what to ask and what to run on yes. */
export interface Confirm {
  title: string;
  body: string;
  action: string;
  danger?: boolean;
  run: () => void;
}

export function ConfirmDialog({ confirm, onClose }: { confirm: Confirm | null; onClose: () => void }) {
  return (
    <Dialog open={confirm !== null} onClose={onClose} title={confirm?.title ?? ""}>
      <p className="mb-5 text-sm">{confirm?.body}</p>
      <div className="flex justify-end gap-2">
        <Button onClick={onClose}>Cancel</Button>
        <Button
          variant={confirm?.danger ? "danger" : "primary"}
          onClick={() => {
            confirm?.run();
            onClose();
          }}
        >
          {confirm?.action}
        </Button>
      </div>
    </Dialog>
  );
}
