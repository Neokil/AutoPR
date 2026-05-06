import { useEffect, useState } from "react";
import { MarkdownView } from "./MarkdownView";
import type { ExecutionLog } from "./types";

type Props = {
  logs: ExecutionLog[];
  loading?: boolean;
  onClose: () => void;
  githubBlobBase?: string;
  repoPath?: string;
  worktreePath?: string;
};

function logLabel(log: ExecutionLog): string {
  return log.state_display_name || log.state;
}

export function ExecutionLogsModal({ logs, loading, onClose, githubBlobBase, repoPath, worktreePath }: Props) {
  const [expandedRunIds, setExpandedRunIds] = useState<string[]>([]);

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        onClose();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  useEffect(() => {
    setExpandedRunIds((current) => current.filter((runId) => logs.some((log) => log.run_id === runId)));
  }, [logs]);

  function toggleRun(runId: string) {
    setExpandedRunIds((current) =>
      current.includes(runId) ? current.filter((id) => id !== runId) : [...current, runId]
    );
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal logs-modal" onClick={(event) => event.stopPropagation()}>
        <div className="logs-modal-header">
          <h3>Execution Logs</h3>
          <button type="button" className="secondary" onClick={onClose}>
            Close
          </button>
        </div>
        {loading ? <p className="meta">Loading logs...</p> : logs.length === 0 ? <p className="meta">No execution logs available yet.</p> : null}
        <div className="logs-list">
          {logs.map((log) => {
            const expanded = expandedRunIds.includes(log.run_id);
            return (
              <section key={log.run_id} className="log-entry">
                <button type="button" className="log-entry-toggle" onClick={() => toggleRun(log.run_id)}>
                  <span>{logLabel(log)}</span>
                  <span className="meta">{new Date(log.timestamp).toLocaleString()}</span>
                </button>
                {expanded ? (
                  <div className="log-entry-body">
                    <p className="meta">{log.path}</p>
                    <MarkdownView
                      content={log.content}
                      emptyText="No log content."
                      githubBlobBase={githubBlobBase}
                      repoPath={repoPath}
                      worktreePath={worktreePath}
                    />
                  </div>
                ) : null}
              </section>
            );
          })}
        </div>
      </div>
    </div>
  );
}
