# Add Input Event

Add a new input event type end-to-end across all three components of the Remotely codebase.

## Event to add: $ARGUMENTS

## Steps

1. **server/server.js** — Add a new `case` in the WebSocket message handler to relay the event from viewer to host (follow the pattern of `input:mouse`, `input:keyboard`, `input:scroll`)

2. **web/public/index.html** — In the `setupInput()` function, add a browser event listener that captures the new input and calls `sendInput({ type: 'input:<name>', ... })` with the relevant payload

3. **host-agent/agent.js** — In the `handleInput()` function, add a new `case` that handles the incoming event and uses `robotjs` to inject the appropriate OS-level action

4. After making changes, summarize what was added in each file and note any robotjs API calls used.
