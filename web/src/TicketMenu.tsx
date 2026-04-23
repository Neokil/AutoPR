import { useEffect, useRef, useState } from "react";
import type { WorkflowStateInfo } from "./types";

type Props = {
  onLogs: () => void;
  onRerun: () => void;
  onCleanup: () => void;
  onMoveToState: (target: string) => void;
  workflowStates: WorkflowStateInfo[];
  currentStateName?: string;
  rerunDisabled?: boolean;
  cleanupDisabled?: boolean;
  moveDisabled?: boolean;
};

export function TicketMenu({
  onLogs,
  onRerun,
  onCleanup,
  onMoveToState,
  workflowStates,
  currentStateName,
  rerunDisabled,
  cleanupDisabled,
  moveDisabled
}: Props) {
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
          <div className="menu-item menu-submenu-root" role="none">
            <button type="button" className="menu-item submenu-trigger" role="menuitem" disabled={moveDisabled}>
              Move To State
            </button>
            {workflowStates.length > 0 ? (
              <div className="menu-submenu" role="menu">
                {workflowStates.map((state) => (
                  <button
                    key={state.name}
                    type="button"
                    className="menu-item"
                    role="menuitem"
                    disabled={moveDisabled || state.name === currentStateName}
                    onClick={() => handleSelect(() => onMoveToState(state.name))}
                  >
                    {state.display_name || state.name}
                  </button>
                ))}
              </div>
            ) : null}
          </div>
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
