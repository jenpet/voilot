# Agent Output Showcase

This file is a manual test script for voilot. Each numbered section asks the
agent to produce a specific type of output so you can verify it renders
correctly in the chat UI and sounds right via TTS.

## How to use

1. Open a voilot session (any agent works, but **build** has full tool access).
2. Say or type: **"Read tests/agent-output-showcase.md and execute section 1"**
3. Verify the UI and TTS output match the expectations listed below.
4. Repeat for sections 2 through 11.

> Tip: switch to **planitect** for sections 1-5 (text-only) and **build** for
> sections 6-11 (tool use required).

---

## Section 1: Short plain text

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

## Section 2: Long plain text

**Instruction to agent:**
Respond with exactly 4 sentences explaining how voice-first planning works in
voilot. Use simple language, no markdown formatting, no code blocks, no bullet
points. Just four plain sentences in a single paragraph.

**Expected UI:**
- One assistant message bubble with a paragraph of 4 sentences.
- Text streams in progressively (visible during response).

**Expected TTS:**
- Each sentence is spoken as it completes (sentence-chunked).
- You should hear the first sentence before the full response finishes.
- No pauses longer than normal speech cadence between sentences.

---

## Section 3: Markdown formatting

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

## Section 4: Fenced code block

**Instruction to agent:**
Write a short TypeScript function (5-10 lines) that checks whether a string is
a palindrome. Put it in a single fenced code block with the `typescript`
language tag. Add one sentence of explanation before the code and one sentence
after. No other formatting.

**Expected UI:**
- One assistant message bubble with: a sentence, then a code block (visible as
  ``` markers and indented code), then a closing sentence.

**Expected TTS:**
- First sentence spoken normally.
- Code block replaced with something like "Wrote 7 lines of typescript."
- Closing sentence spoken normally.
- The actual code is NOT read aloud.

---

## Section 5: Text with multiple code blocks

**Instruction to agent:**
Explain the difference between `ref` and `reactive` in Vue 3. Structure your
response as:
1. A 2-sentence introduction.
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

---

## Section 6: Single tool use (bash)

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

## Section 7: Multiple same-tool uses

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

## Section 8: Mixed tools then text

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

## Section 9: Error scenario

**Instruction to agent:**
Try to read the file `/tmp/voilot-does-not-exist-xyz-12345`. This file does not
exist. After the error, say one sentence acknowledging the file was not found.

**Expected UI:**
- A ToolGroup showing the failed read attempt. The tool result should show an
  error indicator (red X or error status).
- Then an assistant text message acknowledging the error.
- OR: a system error message if the agent surfaces it as an error event.

**Expected TTS:**
- Either "Used read." (tool batch) or an error announcement, depending on how
  the agent reports it.
- Then the acknowledgment sentence spoken normally.

---

## Section 10: Long response with embedded code

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

## Section 11: Permission prompt

**Instruction to agent:**
Try to read a file outside the project directory, for example
`/etc/hosts`. This should trigger a permission prompt from OpenCode asking
whether to allow reading an external directory.

If the agent does NOT trigger a permission prompt (e.g., the agent has
"always allow" configured), instead try: `cat /etc/shadow` via bash, or ask
the agent to read a file path that you know requires approval.

**Expected UI:**
- An amber/yellow permission bubble appears in the chat with:
  - A warning icon (triangle) and the permission title (e.g., "Read external
    directory" or similar).
  - The file pattern shown in monospace below the title.
  - Three action buttons: "Allow Once" (green), "Allow Always" (blue),
    "Deny" (red).
- The streaming indicator changes to "Waiting for approval..." with an amber
  dot instead of the usual blue dot.
- After clicking one of the buttons:
  - **Allow Once/Always**: The bubble turns green with a checkmark and shows
    "Allowed once" or "Allowed always". Buttons disappear. The agent
    continues its work.
  - **Deny**: The bubble turns red with an X and shows "Denied". Buttons
    disappear. The agent reports the permission was denied.
- The streaming indicator returns to normal after resolution.

**Expected TTS:**
- "Permission needed: [title]" announced when the prompt appears.
- Brief "Permission approved" or "Permission denied" after resolution.
- Voice loop does NOT auto-start recording while the permission prompt is
  pending (silence detection is paused).

**Testing external resolution:**
If you have the OpenCode TUI open simultaneously, resolve the permission
there instead of in voilot. The voilot chat should update the permission
bubble to show the resolved state (via the `permission.replied` SSE event).
