import type { StateRun } from "./types";

type Props = {
  runs: StateRun[];
  selectedRunId: string;
  onSelectRun: (runId: string) => void;
};

function runLabel(run: StateRun, runs: StateRun[]): string {
  const base = run.state_display_name || run.state_name;
  let seen = 0;
  let total = 0;
  for (const item of runs) {
    if (item.state_name !== run.state_name) {
      continue;
    }
    total += 1;
    if (item.id === run.id) {
      seen = total;
    }
  }
  if (total <= 1) {
    return base;
  }
  return `${base} ${seen}`;
}

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
            {runLabel(run, runs)}
          </button>
          {index < runs.length - 1 ? <span className="timeline-separator">{">"}</span> : null}
        </div>
      ))}
    </div>
  );
}
