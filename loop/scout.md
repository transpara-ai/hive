# Scout Report — Iteration 127

## Gap: Activity feed shows op type but not what it's about

"Matt intend" is meaningless without clicking through. The Activity lens and dashboard agent activity show op type + actor but not the node title. Users can't understand what happened without navigating to each node.

**Scope:** Join nodes in ListOps and ListUserAgentActivity queries, add NodeTitle to Op struct, show titles in activity views.
