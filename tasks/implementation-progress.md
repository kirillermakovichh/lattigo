# Session handoff

Protocol, ledger, consensus, P2P, RPC, wallet and operator workflows are implemented.
Only main exists; no extra worktrees or remote actions.

BL-023 is complete. make check, the strict fault/race/network gate and the ordinary
binary network scenario pass. Reproduce the extended gate with make check-faults.
Verification reports were removed at the user’s request; keep evidence in executable
tests and keep coverage output temporary. Generated binaries and caches were removed.
Public-release tasks BL-020/BL-021 remain open.
