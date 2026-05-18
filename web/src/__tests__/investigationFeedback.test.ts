import { describe, expect, it } from "vitest";
import { extractOpenQuestions, formatFeedbackMessage } from "../investigationFeedback";

describe("extractOpenQuestions", () => {
  it("extracts numbered questions from the Open Questions section", () => {
    const markdown = `## Problem Summary

Text

## Open Questions

1. First question?
2. Second question?

## Risks

None`;

    expect(extractOpenQuestions(markdown)).toEqual(["First question?", "Second question?"]);
  });

  it("returns an empty list when the section is not a strict markdown list", () => {
    const markdown = `## Open Questions

This is prose instead of a list.`;

    expect(extractOpenQuestions(markdown)).toEqual([]);
  });

  it("supports indented continuation lines for a question", () => {
    const markdown = `## Open Questions

- How should we handle long answers
  when the question wraps multiple lines?
`;

    expect(extractOpenQuestions(markdown)).toEqual([
      "How should we handle long answers when the question wraps multiple lines?"
    ]);
  });
});

describe("formatFeedbackMessage", () => {
  it("formats answered questions and appends additional feedback", () => {
    const message = formatFeedbackMessage(
      ["Question one?", "Question two?"],
      { "0": "Answer one", "1": "" },
      "Extra context"
    );

    expect(message).toContain("## Answers to Open Questions");
    expect(message).toContain("1. **Question:** Question one?");
    expect(message).toContain("**Answer:** Answer one");
    expect(message).not.toContain("Question two?");
    expect(message).toContain("## Additional Feedback");
    expect(message).toContain("Extra context");
  });

  it("returns general feedback unchanged when there are no answered questions", () => {
    expect(formatFeedbackMessage(["Question one?"], { "0": "   " }, "General note")).toBe("General note");
  });

  it("returns an empty string when everything is blank", () => {
    expect(formatFeedbackMessage(["Question one?"], { "0": "" }, "   ")).toBe("");
  });
});
