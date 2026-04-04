# Agent Output Showcase

This file is a manual test script for voilot. Each numbered section asks the
agent to produce a specific type of output so you can verify it renders
correctly in the chat UI and sounds right via TTS.

## How to use

1. Open a voilot session (any agent works, but **build** has full tool access).
2. Say or type: **"Read tests/agent-output-showcase.md and execute section 1"**
3. Verify the UI and TTS output match the expectations listed below.
4. Repeat for each section.

> Tip: switch to **planitect** for sections 1-3 (text-only) and **build** for
> sections 4-11 (tool use / permissions required).

---

## Section 1: Plain text

**Instruction to agent:**
Respond with exactly one short sentence (under 15 words) that describes what
voilot is. Do not use any formatting, code, or markdown. Just a single plain
sentence.

**Expected UI:**
- One assistant message bubble with a short sentence.
- No tool groups, no system messages.

**Expected TTS:**
- The sentence is spoken immediately as a single chunk.
- No "Wrote N lines" summaries, no tool announcements.

---

## Section 2: Markdown formatting

**Instruction to agent:**
Respond using the following markdown elements, all in one message. Use each
element at least once:
- A level-2 heading
- **Bold text**
- *Italic text*
- An inline code snippet like `useState`
- A bulleted list with 3 items
- A markdown link like [Nuxt docs](https://nuxt.com)

The content should be about Vue composables. Keep it to ~80 words total.

**Expected UI:**
- One assistant message bubble containing raw markdown text (voilot renders
  plain text, not rendered HTML).
- All markdown syntax characters visible (`##`, `**`, `*`, backticks, `-`, `[]()`).

**Expected TTS:**
- Headers spoken without `##` prefix (just the title text).
- Bold/italic markers stripped -- text spoken naturally.
- Inline code backticks removed -- `useState` spoken as "use state".
- Bullet markers (`-`) stripped -- items spoken as sentences.
- Links spoken as label only ("Nuxt docs"), URL dropped.

---

## Section 3: Code blocks

**Instruction to agent:**
Explain the difference between `ref` and `reactive` in Vue 3. Structure your
response as:
1. A 1-sentence introduction.
2. A short code example using `ref` (3-5 lines, fenced as `typescript`).
3. A 1-sentence transition.
4. A short code example using `reactive` (3-5 lines, fenced as `typescript`).
5. A 1-sentence conclusion.

**Expected UI:**
- One assistant message with text, code, text, code, text interleaved.

**Expected TTS:**
- Introduction spoken normally.
- First code block: "Wrote N lines of typescript."
- Transition sentence spoken.
- Second code block: "Wrote N lines of typescript."
- Conclusion spoken.
- Two separate code summaries, each announcing its own line count.
- The actual code is NOT read aloud.

---

## Section 4: Single tool use (bash)

**Instruction to agent:**
Run `ls -la tests/` in the project root. Then say one sentence describing what
you see.

**Expected UI:**
- A collapsible ToolGroup showing a bash invocation and its result.
- Then an assistant text message with a one-sentence description.
- ToolGroup header: "Used bash for this step" with a duration.

**Expected TTS:**
- "Used bash." spoken first (tool batch flushed when text starts).
- Then the description sentence spoken normally.

---

## Section 5: Multiple same-tool uses

**Instruction to agent:**
Read these three files using the read tool (not bash cat):
1. `backend/internal/agent/adapter.go` -- just the first 5 lines
2. `backend/internal/api/router.go` -- just the first 5 lines
3. `backend/internal/voice/router.go` -- just the first 5 lines

After reading all three, respond with one sentence: "Read all three files
successfully."

**Expected UI:**
- A collapsible ToolGroup containing 3 read invocations with results.
- ToolGroup header: "Used 3 tools for this step" (or similar) with total duration.
- Expanding the group shows individual read results.
- Then one assistant text message.

**Expected TTS:**
- "Used read 3 times." (batched tool summary).
- Then "Read all three files successfully." spoken normally.

---

## Section 6: Mixed tools then text

**Instruction to agent:**
First, run `wc -l backend/internal/agent/opencode.go` to count lines. Then
read the first 10 lines of that same file. Finally, write a 2-sentence summary
of what the file appears to do based on those first lines.

**Expected UI:**
- A ToolGroup containing the bash invocation and the read invocation with results.
- Then an assistant text message with the 2-sentence summary.
- Duration shown on the ToolGroup.

**Expected TTS:**
- Tool batch: "Used 2 times bash and read." or "Used bash and read." (depends
  on whether wc -l counts as bash and read is separate).
- Then the 2-sentence summary spoken normally.
- Key test: the tool batch flushes BEFORE the text starts speaking.

---

## Section 7: Unexpected error

**Instruction to agent:**
Read the project configuration from `backend/config/settings.yaml` and
summarize its contents.

> **Note for tester:** This file does not exist. The instruction is
> deliberately phrased as if it does. The agent should attempt the read,
> encounter the error naturally, and then report what happened.

**Expected UI:**
- A ToolGroup showing the failed read attempt. The tool result should show an
  error indicator (red X or error status).
- Then an assistant text message acknowledging the file was not found.
- OR: a system error message if the agent surfaces it as an error event.

**Expected TTS:**
- Either "Used read." (tool batch) or an error announcement, depending on how
  the agent reports it.
- Then an acknowledgment or explanation spoken normally.

---

## Section 8: Long response with embedded code

**Instruction to agent:**
Write a brief technical overview (150-200 words) of how the voilot WebSocket
chat handler works. Include:
- 2-3 paragraphs of explanation.
- One code snippet (5-8 lines) showing the chatInbound struct from ws.go.
- A final paragraph summarizing the flow.

Base this on the actual code in `backend/internal/api/ws.go`.

**Expected UI:**
- A ToolGroup if the agent reads ws.go first (likely).
- Then a long streaming assistant message with paragraphs and an embedded code
  block.
- Text streams in visibly over several seconds.

**Expected TTS:**
- Tool batch spoken first if the agent reads the file.
- Text streamed as sentence-sized chunks (first sentence audible within ~2s of
  text starting).
- Code block: "Wrote N lines of go." (not read aloud).
- Final paragraph spoken after code summary.
- Full response may take 10-20s to speak -- verify no awkward pauses or
  cut-offs between chunks.

---

## Section 9: Permission prompt (external directory)

**Instruction to agent:**
Read the file `/etc/hosts` and tell me what hostnames are defined in it.

> **Prerequisite:** The `external_directory` permission must NOT be set to
> "always allow" in your OpenCode configuration, otherwise the prompt will be
> skipped. Reset permissions if needed before running this test.

**Expected UI:**
- An amber/yellow permission bubble appears in the chat with:
  - A warning icon (triangle) and the permission title (e.g., "Read external
    directory" or similar).
  - The file pattern shown in monospace below the title.
  - Three action buttons: "Allow Once" (green), "Allow Always" (blue),
    "Deny" (red).
- The streaming indicator changes to "Waiting for approval..." with an amber
  dot instead of the usual blue dot.
- After clicking "Allow Once":
  - The bubble turns green with a checkmark and shows "Allowed once".
  - Buttons disappear.
  - The agent continues, reads `/etc/hosts`, and responds with the hostnames.
- After clicking "Deny" (alternative test):
  - The bubble turns red with an X and shows "Denied".
  - Buttons disappear.
  - The agent reports that permission was denied and it cannot read the file.

**Expected TTS:**
- "Permission needed: [title]" announced when the prompt appears.
- Brief "Permission approved" or "Permission denied" after resolution.
- Voice loop does NOT auto-start recording while the permission prompt is
  pending (silence detection is paused).
- After approval: the agent's summary of `/etc/hosts` is spoken normally.

**Testing external resolution:**
If you have the OpenCode TUI open simultaneously, resolve the permission
there instead of in voilot. The voilot chat should update the permission
bubble to show the resolved state (via the `permission.replied` SSE event).

---

## Section 10: Option request — single question

**Instruction to agent:**
I want to add a new API endpoint to the backend. Before writing any code, ask
me one clarifying question: what HTTP method should the endpoint use? Present
the options GET, POST, PUT, and DELETE for me to choose from.

**Expected UI:**
- An indigo-themed question bubble appears in the chat with:
  - The question header shown as a bold label.
  - The question text displayed clearly below.
  - Four selectable option buttons: "GET", "POST", "PUT", "DELETE".
  - Each option button shows the label and description (if any).
  - A "Dismiss" link at the bottom to reject the question.
- The streaming indicator changes to "Answer a question..." with an indigo
  dot instead of the usual blue dot.
- A hint appears above the chat input: "Answering question — type a custom
  answer or pick an option above."
- After clicking an option button:
  - The bubble turns from pending to resolved state with a checkmark.
  - The selected option label is shown (e.g., "POST").
  - Buttons disappear.
  - The agent continues its response incorporating the chosen option.
- Alternatively, typing a custom answer in the chat input sends it as the
  answer (intercepted by `tryAnswerPendingQuestion`).
- After clicking "Dismiss":
  - The bubble shows "Dismissed" in the resolved state.
  - The question is rejected via the backend.
- If no selection is made, the agent remains waiting (no auto-timeout).

**Expected TTS:**
- The question is spoken aloud: "What HTTP method should the endpoint use?"
- Options are announced: "Options: GET, POST, PUT, DELETE."
- Voice loop does NOT auto-start recording while the question prompt is
  pending (same behavior as permission prompts).
- After selection: the agent's follow-up response is spoken normally.

---

## Section 11: Option request — multiple questions

**Instruction to agent:**
I want you to scaffold a new composable for the frontend. Before writing any
code, ask me these questions in a single response:
1. What should the composable be named? Suggest three options: `useMetrics`,
   `useAnalytics`, or `useTracking`.
2. Should it include a cleanup function on unmount? Yes or No.
3. What data format should it export? Options: "raw object", "readonly ref",
   or "computed property".

**Expected UI:**
- Three separate indigo-themed question bubbles appear in the chat (the
  backend splits multi-question batches into individual events):
  - Q1: three name option buttons (`useMetrics`, `useAnalytics`, `useTracking`).
  - Q2: Yes / No option buttons.
  - Q3: three format option buttons.
- Each question bubble has its own header, question text, options, and
  "Dismiss" link.
- Questions are answered sequentially — the first unanswered question
  accepts input (button click or typed custom answer).
- After all three questions are answered:
  - All answers are assembled into a single response and sent to the backend.
  - Each bubble shows its resolved state with the selected option.
  - The agent continues with the scaffolding based on the chosen answers.
- If any question is dismissed, the entire question batch is rejected.

**Expected TTS:**
- Each question is spoken in sequence with its options enumerated.
- Voice loop does NOT auto-start recording while questions are pending.
- After all answers are submitted: the agent's follow-up response is spoken
  normally.
