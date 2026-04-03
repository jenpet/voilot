# tests/

Manual test scripts for verifying voilot's UI rendering and TTS voice output.

## Available tests

### agent-output-showcase.md

A 10-section test script that exercises every type of agent output voilot
handles. Each section asks the agent to produce a specific output pattern
(plain text, markdown, code blocks, tool use, errors, etc.) and documents
what to expect in the chat UI and over TTS.

**What it covers:**

| Section | Output type | Agent needed |
|---------|-------------|-------------|
| 1 | Short plain text | any |
| 2 | Long plain text (multi-sentence) | any |
| 3 | Markdown formatting (headers, bold, lists, links) | any |
| 4 | Fenced code block | any |
| 5 | Multiple code blocks interleaved with text | any |
| 6 | Single tool use (bash) | build |
| 7 | Multiple same-tool uses (read x3) | build |
| 8 | Mixed tool types then text | build |
| 9 | Error scenario (nonexistent file) | build |
| 10 | Long response with embedded code | build |

**How to run:**

1. Start the voilot stack (`task dev` or individual services).
2. Open a session in the browser.
3. For sections 1-5 (text-only), either agent works. For sections 6-10,
   switch to **build** via the agent selector dropdown.
4. Say or type:

   ```
   Read tests/agent-output-showcase.md and execute section 1
   ```

5. Verify the chat UI and TTS output match the expectations in the file.
6. Repeat for sections 2 through 10.

**What to look for:**

- **UI:** Message bubbles render correctly (text, tool groups, system messages).
  Tool groups are collapsible. Streaming is visible during long responses.
- **TTS:** Text is spoken in sentence-sized chunks. Code blocks are summarized
  as "Wrote N lines of X" (not read aloud). Tool batches are announced as
  "Used bash." or "Used read 3 times." Markdown formatting is stripped from
  speech. Errors are announced.
- **Voice loop:** After TTS finishes, the mic should auto-activate for the
  next turn (if voice mode is on).
