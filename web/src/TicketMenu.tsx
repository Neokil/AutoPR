import { useEffect, useRef, useState } from "react";

type Props = {
  onLogs: () => void;
  onRerun: () => void;
  onCleanup: () => void;
  rerunDisabled?: boolean;
  cleanupDisabled?: boolean;
};

export function TicketMenu({ onLogs, onRerun, onCleanup, rerunDisabled, cleanupDisabled }: Props) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!open) {
      return;
    }
    const handlePointerDown = (event: MouseEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) {
        setOpen(false);
      }
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setOpen(false);
      }
    };
    window.addEventListener("mousedown", handlePointerDown);
    window.addEventListener("keydown", handleKeyDown);
    return () => {
      window.removeEventListener("mousedown", handlePointerDown);
      window.removeEventListener("keydown", handleKeyDown);
    };
  }, [open]);

  function handleSelect(action: () => void) {
    setOpen(false);
    action();
  }

  return (
    <div className="menu-root" ref={rootRef}>
      <button
        type="button"
        className="secondary menu-trigger"
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((current) => !current)}
      >
        ☰
      </button>
      {open ? (
        <div className="menu-dropdown" role="menu">
          <button type="button" className="menu-item" role="menuitem" onClick={() => handleSelect(onLogs)}>
            Logs
          </button>
          <button
            type="button"
            className="menu-item"
            role="menuitem"
            disabled={rerunDisabled}
            onClick={() => handleSelect(onRerun)}
          >
            Rerun
          </button>
          <button
            type="button"
            className="menu-item"
            role="menuitem"
            disabled={cleanupDisabled}
            onClick={() => handleSelect(onCleanup)}
          >
            Cleanup
          </button>
        </div>
      ) : null}
    </div>
  );
}
