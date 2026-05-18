export function extractOpenQuestions(markdown: string): string[] {
  const section = extractSection(markdown, "Open Questions");
  if (!section) {
    return [];
  }

  const lines = section.split("\n");
  const questions: string[] = [];
  let current = "";
  let sawListItem = false;

  for (const rawLine of lines) {
    const line = rawLine.replace(/\s+$/, "");
    const trimmed = line.trim();

    if (!trimmed) {
      continue;
    }

    const listMatch = trimmed.match(/^([-*+]|\d+\.)\s+(.+)$/);
    if (listMatch) {
      if (current) {
        questions.push(current.trim());
      }
      current = listMatch[2].trim();
      sawListItem = true;
      continue;
    }

    if (!sawListItem) {
      return [];
    }

    if (/^\s{2,}\S/.test(line)) {
      current = `${current} ${trimmed}`.trim();
      continue;
    }

    return [];
  }

  if (current) {
    questions.push(current.trim());
  }

  return questions;
}

export function formatFeedbackMessage(
  questions: string[],
  answers: Record<string, string>,
  generalFeedback: string
): string {
  const answeredQuestions = questions
    .map((question, index) => ({
      question,
      answer: (answers[String(index)] ?? "").trim()
    }))
    .filter((entry) => entry.answer);
  const trimmedGeneralFeedback = generalFeedback.trim();

  if (answeredQuestions.length === 0) {
    return trimmedGeneralFeedback;
  }

  const parts = ["## Answers to Open Questions", ""];
  answeredQuestions.forEach((entry, index) => {
    parts.push(`${index + 1}. **Question:** ${entry.question}`);
    parts.push("");
    parts.push(`   **Answer:** ${entry.answer}`);
    parts.push("");
  });

  if (trimmedGeneralFeedback) {
    parts.push("## Additional Feedback", "");
    parts.push(trimmedGeneralFeedback);
  }

  return parts.join("\n").trim();
}

function extractSection(markdown: string, sectionName: string): string {
  const lines = markdown.split("\n");
  const sectionHeader = `## ${sectionName}`.toLowerCase();
  let inSection = false;
  const sectionLines: string[] = [];

  for (const line of lines) {
    const trimmed = line.trim();
    if (!inSection) {
      if (trimmed.toLowerCase() === sectionHeader) {
        inSection = true;
      }
      continue;
    }

    if (/^##\s+/.test(trimmed)) {
      break;
    }

    sectionLines.push(line);
  }

  return sectionLines.join("\n").trim();
}
