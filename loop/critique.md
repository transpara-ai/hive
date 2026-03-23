# Critique — Iteration 130

PASS. Remove button only shows on "Depends on" rows (canRemove=true), not "Blocking" rows (canRemove=false). This is correct — you remove your own dependencies, not other tasks' dependencies on you.
