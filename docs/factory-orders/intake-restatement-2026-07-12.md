# Channel A Intake Restatement — the exact reading Michael confirmed

Date: 2026-07-12
Operator: Michael Saucier (michael.saucier@transpara.com)
Channel: Claude Code chat session 8dab1975-a509-4008-8ba5-2405d331a763 (same
session as the intake and the confirmation)

Chain: this restatement was presented after FO v0.1.0 (git blob
`df8aa1c2ea64f0e5b95f82fb245446a69e8f2e94`) in response to Michael's request
"restate the requirements with no jargon. Black and white text with simple
answers". Michael's answers below produced FO v0.2.0 (git blob
`c76846e8b352001e7bf9cfea7036af0e49f9aeba`). The confirmation answers were
separately archived at `intake-confirmation-2026-07-12.md` (sha256
`b6b06615e4c7becdbfd012d3e378316bc38ea7979b293cb62e2ed2f7c340f310`); this file
adds the restatement text those answers confirmed.

## Verbatim restatement presented to Michael (the confirmed reading)

> Here are the seven requirements in plain language:
>
> 1. Change the name everywhere the software actually uses it. Every place in the hive code, tests, helper guides, and config files that says LOVYOU_API_KEY will say TRANSPARA_API_KEY instead. Proof it's done: a search for the old name in the working code finds nothing, and the software still builds.
>
> 2. Clean break, no safety net. The software reads only the new name. It never quietly falls back to the old name. If the key is missing, the program behaves exactly as it does today when the key is missing: it stops and says the key is required, or runs in local-only mode where that is allowed.
>
> 3. "Empty" and "missing" keep their current meanings. Today, an empty key means "run locally, don't talk to the website" and a filled key means "talk to the website." That behavior stays exactly the same — only the name changes.
>
> 4. The startup check keeps its safety features. There is a check that looks at the running service and reports whether the key is filled, empty, or missing. It must keep doing four things: match only the exact name, tell "empty" apart from "filled," never print the key itself, and say "unknown — stop" instead of guessing when it can't read the information.
>
> 5. Delete the real key from the repository. A real, working key is currently saved in a file in the public hive repository. That value gets removed. Proof it's done: a search of the whole repository finds no real key anywhere.
>
> 6. Give you written server-update steps. The change ships with simple instructions for you: rename the key in the settings file on the server, restart the services, run the check to confirm it worked. I write the steps; only you run them.
>
> 7. Leave old records alone. Past documents that mention the old name are history. They stay exactly as written.
>
> Not included unless you say otherwise:
>
> - The two other old-name variables (LOVYOU_BASE_URL and LOVYOU_SPACE) keep their names.
> - Old git history is not scrubbed.
> - Replacing the actual key on the website (so the leaked one stops working) is a separate, urgent step that only you can do. The rename does not fix the leak — replacing the key does.
>
> Three simple questions before any work starts:
>
> 1. Did I understand your order correctly? Yes or no.
> 2. Rename just this one variable, or all three old-name variables? One or all.
> 3. Leave git history alone and just replace the key, or also scrub history? Leave it or scrub it.

## Verbatim answers (same as the separately archived confirmation)

> 1. yes
> 2. all 3 old name variables
> 3. leave it

## What the confirmation therefore covers

Answer 1 ("yes") confirms the seven numbered requirements above as the correct
reading of the order — including credential removal from the repository (item
5), the operator migration runbook (item 6), historical-records protection
(item 7), and the hive-repository scope stated in item 1 ("the hive code").
Answer 2 expands scope to all three variables. Answer 3 settles history
handling as no-rewrite.
