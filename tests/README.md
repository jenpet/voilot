# tests/

Manual test scripts for verifying voilot's UI rendering and TTS voice output.

## Available tests

### agent-output-showcase.md

An 11-section test script that exercises every type of agent output voilot
handles. Each section asks the agent to produce a specific output pattern
(plain text, markdown, code blocks, tool use, errors, permissions, options)
and documents what to expect in the chat UI and over TTS.

**What it covers:**

| Section | Output type | Agent needed |
|---------|-------------|-------------|
| 1 | Plain text | any |
| 2 | Markdown formatting (headers, bold, lists, links) | any |
| 3 | Code blocks interleaved with text | any |
| 4 | Single tool use (bash) | build |
| 5 | Multiple same-tool uses (read x3) | build |
| 6 | Mixed tool types then text | build |
| 7 | Unexpected error (nonexistent file, agent unaware) | build |
| 8 | Long response with embedded code | build |
| 9 | Permission prompt (external directory) | build |
| 10 | Option request — single question (not yet implemented) | build |
| 11 | Option request — multiple questions (not yet implemented) | build |

**How to run:**

1. Start the voilot stack (`task dev` or individual services).
2. Open a session in the browser.
3. For sections 1-3 (text-only), either agent works. For sections 4-11,
   switch to **build** via the agent selector dropdown.
4. Say or type:

   ```
   Read tests/agent-output-showcase.md and execute section 1
   ```

5. Verify the chat UI and TTS output match the expectations in the file.
6. Repeat for each section.

**What to look for:**

- **UI:** Message bubbles render correctly (text, tool groups, system messages).
  Tool groups are collapsible. Streaming is visible during long responses.
  Permission prompts show interactive buttons. Option prompts are aspirational
  (sections 10-11 document target behavior for a feature not yet built).
- **TTS:** Text is spoken in sentence-sized chunks. Code blocks are summarized
  as "Wrote N lines of X" (not read aloud). Tool batches are announced as
  "Used bash." or "Used read 3 times." Markdown formatting is stripped from
  speech. Errors are announced.
- **Voice loop:** After TTS finishes, the mic should auto-activate for the
  next turn (if voice mode is on). The mic does NOT auto-activate while
  permission or option prompts are pending.
