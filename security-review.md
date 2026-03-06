# Security Review

Review the Remotely codebase for security issues before deploying publicly.

Focus area: $ARGUMENTS

## What to check

- **server/server.js**: Rate limiting on session creation, input validation on all incoming WS messages, session key entropy, stale session cleanup, CORS policy
- **host-agent/agent.js**: Privilege level of robotjs input injection, exposure of hostname/platform info, reconnect behavior that could be exploited
- **web/public/index.html**: XSS vectors in any dynamic content, clipboard API usage, WebRTC ICE leak risks

## Output format

For each issue found:
- **Severity**: Critical / High / Medium / Low
- **Location**: File + line/function
- **Issue**: What the problem is
- **Fix**: Concrete code change to resolve it

End with a prioritized list of the top 3 things to fix before going to production.
