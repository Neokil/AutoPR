import { runDisplayLabel } from "./tickets";
import type { StateRun } from "./types";

type Props = {
  runs: StateRun[];
  selectedRunId: string;
  onSelectRun: (runId: string) => void;
};

export function StateTimeline({ runs, selectedRunId, onSelectRun }: Props) {
  if (runs.length === 0) {
    return null;
  }

  return (
    <div className="timeline" aria-label="Workflow timeline">
      {runs.map((run, index) => (
        <div key={run.id} className="timeline-item">
          <button
            type="button"
            className={selectedRunId === run.id ? "timeline-pill active" : "timeline-pill"}
            onClick={() => onSelectRun(run.id)}
          >
            {runDisplayLabel(run, runs)}
          </button>
          {index < runs.length - 1 ? <span className="timeline-separator">{">"}</span> : null}
        </div>
      ))}
    </div>
  );
}
