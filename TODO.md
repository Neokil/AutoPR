# Features
- Remove Shortcut references from UI. AutoPR should be tool agnoistic and only the prompts should contain the tool references.
- Add a UI for the Open Questions in the investigation phase. This means for every question we get a text field that we can use to provide feedback for this specific question. The general feedback field should be moved to the bottom of the page. When we now click on "provide feedback" the answers to the open questions and the general feedback should both be transmitted to the llm.
- Can we show the upcoming state already? So when I for example click on "Approve" for an Investigation step the new "Implementation" step is already showing up, we switch to it and the body shows a "running" indicator.
- Add support for multiple workflows. For example one workflow that is just there to refine tickets. One workflow to actually then work on them.
- Allow to define the model/provider per prompt. Some models may be better in analyzing, some in coding and some might be cheap but sufficient for tasks like writing commit messages.
- Add in "after-scripts" that receive the prompt output. That way we could do things like "generate commit message" as prompt and pipe that into a fixed "commit + push" script.

# Bugs
- 
