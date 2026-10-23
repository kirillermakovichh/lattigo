# Decision outcome audit

Measured on 2026-11-15 with test revision `ab8ca77`, using isolated copies of the source, package tests and the real
four-representative/restored-observer workflow. This report records observed outcomes,
not a claim of exhaustive correctness or release readiness.

Reproduce with `python3 scripts/branch_coverage.py --network`.
The tool never instruments the working source tree. Its own tests check named boolean
conditions, early returns, labeled ranges, switch defaults, select and output ownership.

## Measurement

896 decision sites, 1837 expected outcomes, 1609 observed (87.6%).
Scope: true/false for if and conditional for; empty/nonempty for range;
case bodies and implicit defaults for switch/type-switch; selected select clauses.
This is not MC/DC, independent short-circuit operand coverage, exhaustive schedules,
or proof that every case expression on a combined switch clause ran.
Instrumentation changes timing; ordinary-source race checks remain required.
Range nonempty means at least one iteration, not every possible iteration count.

| Package | Observed | Expected |
| --- | ---: | ---: |
| `cmd/blctl` | 4 | 4 |
| `cmd/blnode` | 11 | 14 |
| `cmd/blwallet` | 4 | 4 |
| `internal/client` | 64 | 64 |
| `internal/consensus` | 340 | 383 |
| `internal/control` | 113 | 134 |
| `internal/ledger` | 240 | 272 |
| `internal/node` | 106 | 124 |
| `internal/p2p` | 145 | 161 |
| `internal/protocol` | 109 | 114 |
| `internal/rpc` | 86 | 95 |
| `internal/storage` | 117 | 132 |
| `internal/wallet` | 270 | 336 |

## Residual outcomes

Every unobserved outcome is listed below; none is silently counted as covered.
Some require OS/descriptor/entropy fault injection; some depend on resource ceilings,
consensus schedules, wallet pagination or remote failure sequences not yet exercised.
Others are excluded by earlier validation or fixed nonempty literals. These categories
are review guidance, not a blanket assertion that the remaining outcomes are unreachable.
Examples: nonce exhaustion requires the uint64 search boundary; an empty range over a
fixed two-element literal is impossible; successful fixed-parameter AES construction
cannot produce its defensive error through user input. Closed-enum and recovered-state
paths need their preceding validation considered before calling them unreachable.

Further work must distinguish reachable gaps from proven invariants per outcome.
In particular, remaining wallet/control I/O errors, consensus persistence boundaries,
peer queue saturation and node runtime shutdown failures remain qualification gaps.
No 100% branch-coverage claim is made.

| Source site | Missing outcomes | Condition / expression |
| --- | --- | --- |
| `internal/consensus/engine.go:87:2:if` | true | `st.Height == math.MaxUint64` |
| `internal/consensus/engine.go:107:3:if` | false | `saved.Height == st.Height+1` |
| `internal/consensus/engine.go:112:9:if` | true | `!errors.Is(err, ledger.ErrNotFound)` |
| `internal/consensus/engine.go:115:2:if` | true | `e.state.Signed == nil` |
| `internal/consensus/engine.go:130:2:if` | true | `err != nil` |
| `internal/consensus/engine.go:141:2:if` | false | `err == nil` |
| `internal/consensus/engine.go:312:5:if` | false | `err == nil` |
| `internal/consensus/engine.go:346:3:if` | true | `uint64(r)+2 < uint64(round)` |
| `internal/consensus/engine.go:351:3:if` | false | `v.Round < round` |
| `internal/consensus/engine.go:360:3:if` | true | `err != nil` |
| `internal/consensus/engine.go:364:3:if` | true | `err != nil` |
| `internal/consensus/engine.go:274:3:range` | empty | `phases` |
| `internal/consensus/engine.go:275:4:range` | empty | `vs` |
| `internal/consensus/recovery.go:22:2:if` | true | `(s.Locked == nil) != (s.LockedRound < 0) &#124;&#124; (s.Valid == nil) != (s.ValidRound < 0)` |
| `internal/consensus/recovery.go:25:2:range` | empty | `[]*protocol.Block{s.Locked, s.Valid}` |
| `internal/consensus/voting.go:25:3:if` | false | `old.Block != hash` |
| `internal/consensus/voting.go:32:2:if` | true | `err != nil` |
| `internal/consensus/voting.go:37:2:if` | true | `err != nil` |
| `internal/consensus/voting.go:85:4:if` | false | `other.Round == v.Round` |
| `internal/consensus/voting.go:90:4:if` | true | `err != nil` |
| `internal/consensus/voting.go:94:5:if` | false | `other.Round == v.Round` |
| `internal/consensus/voting.go:207:4:if` | true | `err != nil` |
| `internal/consensus/voting.go:220:2:if` | true | `err != nil` |
| `internal/consensus/voting.go:233:2:for` | false | `step < 64` |
| `internal/consensus/voting.go:247:6:if` | true | `err != nil` |
| `internal/consensus/voting.go:256:4:if` | false | `e.state.Locked == nil &#124;&#124; e.state.Locked.ID() == p.Block.ID() &#124;&#124; p.ValidRound >= e.state.LockedRound` |
| `internal/consensus/voting.go:259:4:if` | true | `err != nil` |
| `internal/consensus/voting.go:271:6:if` | true | `err != nil` |
| `internal/consensus/voting.go:279:6:if` | true | `err != nil` |
| `internal/consensus/voting.go:282:6:if` | true | `err != nil` |
| `internal/consensus/voting.go:289:4:if` | true | `err != nil` |
| `internal/consensus/voting.go:297:5:if` | true | `err != nil` |
| `internal/consensus/voting.go:307:4:if` | true | `err != nil` |
| `internal/consensus/voting.go:323:6:if` | false | `!seen[rep]` |
| `internal/consensus/voting.go:334:4:if` | true | `err != nil` |
| `internal/consensus/voting.go:361:2:if` | false | `broadcast` |
| `internal/consensus/voting.go:371:3:if` | false | `id == r.Block.ID() &#124;&#124; e.cfg.Store.Validate(b) != nil` |
| `internal/consensus/voting.go:84:3:range` | empty | `e.ahead` |
| `internal/consensus/voting.go:93:4:range` | empty | `e.ahead` |
| `internal/consensus/voting.go:95:6:range` | empty | `other.Votes` |
| `internal/consensus/voting.go:143:2:range` | empty | `p.Proof` |
| `internal/consensus/voting.go:321:4:range` | empty | `phases` |
| `internal/consensus/voting.go:322:5:range` | empty | `vs` |
| `internal/control/control.go:96:2:switch` | default | `cmd` |
| `internal/control/control.go:102:3:if` | true | `e != nil` |
| `internal/control/control.go:106:3:if` | true | `e != nil` |
| `internal/control/control.go:126:4:if` | true | `e != nil` |
| `internal/control/control.go:133:4:if` | true | `e != nil` |
| `internal/control/control.go:152:4:if` | false | `representative == ""` |
| `internal/control/control.go:168:3:if` | true | `e != nil` |
| `internal/control/control.go:176:4:if` | false | `e != nil` |
| `internal/control/control.go:182:3:if` | false | `token == ""` |
| `internal/control/control.go:184:4:if` | true | `e != nil` |
| `internal/control/control.go:249:4:if` | true | `e != nil` |
| `internal/control/control.go:255:4:if` | true | `e != nil` |
| `internal/control/control.go:284:2:if` | true | `e != nil` |
| `internal/control/control.go:289:2:if` | true | `e != nil` |
| `internal/control/control.go:297:2:if` | true | `e != nil` |
| `internal/control/control.go:301:2:if` | true | `e != nil` |
| `internal/control/control.go:308:2:if` | true | `e != nil` |
| `internal/control/control.go:321:2:if` | true | `e != nil` |
| `internal/control/control.go:328:2:if` | true | `e != nil` |
| `internal/control/control.go:331:2:if` | true | `len(b) > 16<<10` |
| `internal/control/control.go:158:3:range` | empty | `g.Allocations` |
| `internal/ledger/state.go:57:2:switch` | default | `b.Kind` |
| `internal/ledger/state.go:81:2:if` | true | `!s.representativeCapacity(b, a, exists)` |
| `internal/ledger/state.go:122:2:if` | true | `ok` |
| `internal/ledger/state.go:125:2:if` | true | `e != nil` |
| `internal/ledger/state.go:22:2:range` | empty | `g.Allocations` |
| `internal/ledger/state.go:29:2:range` | empty | `s.accounts` |
| `internal/ledger/state.go:111:2:range` | empty | `weights` |
| `internal/ledger/store.go:81:2:if` | true | `e != nil` |
| `internal/ledger/store.go:91:3:if` | true | `uint64(len(p)) > s.historyLimit-s.historyBytes` |
| `internal/ledger/store.go:95:3:if` | true | `e != nil` |
| `internal/ledger/store.go:98:3:if` | true | `e != nil` |
| `internal/ledger/store.go:303:2:if` | true | `e != nil` |
| `internal/ledger/store.go:310:2:if` | true | `e != nil` |
| `internal/ledger/store.go:321:2:if` | true | `s.observerOnly` |
| `internal/ledger/store.go:369:2:if` | true | `e != nil` |
| `internal/ledger/store.go:376:9:if` | true | `!errors.Is(err, os.ErrNotExist)` |
| `internal/ledger/store.go:379:2:if` | true | `e != nil` |
| `internal/ledger/store.go:382:2:if` | true | `e != nil` |
| `internal/ledger/store.go:387:9:if` | true | `e != nil && !errors.Is(e, os.ErrNotExist)` |
| `internal/ledger/store.go:391:2:if` | true | `e != nil` |
| `internal/ledger/store.go:396:2:if` | true | `e != nil` |
| `internal/ledger/store.go:418:2:if` | true | `e != nil` |
| `internal/ledger/store.go:422:2:if` | true | `e != nil && !errors.Is(e, os.ErrExist)` |
| `internal/ledger/store.go:426:2:if` | true | `e != nil` |
| `internal/ledger/store.go:430:2:if` | true | `e != nil` |
| `internal/ledger/store.go:435:2:if` | true | `e != nil` |
| `internal/ledger/store.go:439:3:if` | true | `entry.Name() != "LOCK"` |
| `internal/ledger/store.go:443:2:if` | true | `e != nil` |
| `internal/ledger/store.go:446:2:if` | true | `e != nil` |
| `internal/ledger/store.go:438:2:range` | empty | `entries` |
| `internal/ledger/types.go:98:2:if` | true | `e != nil` |
| `internal/ledger/types.go:167:2:range` | empty | `r.Votes` |
| `internal/node/config.go:74:2:if` | true | `err != nil` |
| `internal/node/config.go:81:2:if` | true | `err != nil` |
| `internal/node/config.go:97:2:if` | true | `err != nil` |
| `internal/node/config.go:141:4:if` | true | `digit < '0' &#124;&#124; digit > '9'` |
| `internal/node/config.go:187:2:if` | true | `err != nil` |
| `internal/node/config.go:101:2:range` | empty | `[]*string{&cfg.DataDir, &cfg.Genesis, &cfg.RepresentativeKey}` |
| `internal/node/config.go:131:2:range` | empty | `[]string{c.RPCListen, c.P2PListen}` |
| `internal/node/config.go:140:3:range` | empty | `port` |
| `internal/node/config.go:195:2:range` | empty | `seed` |
| `internal/node/run.go:55:2:if` | true | `err != nil` |
| `internal/node/run.go:59:2:if` | true | `err != nil` |
| `internal/node/run.go:83:4:if` | true | `err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, context.Canceled)` |
| `internal/node/run.go:84:5:select` | case0, default | `(case selection)` |
| `internal/node/run.go:97:2:select` | case1 | `(case selection)` |
| `internal/node/run.go:112:4:if` | true | `err != nil` |
| `internal/node/run.go:127:2:if` | true | `err != nil` |
| `internal/node/run.go:108:2:range` | empty | `[]*http.Server{rpcServer, peerServer}` |
| `internal/p2p/network.go:82:2:if` | true | `strings.ContainsAny(u.Host, "\r\n")` |
| `internal/p2p/network.go:131:2:if` | true | `err != nil &#124;&#124; len(raw) > maxMessage` |
| `internal/p2p/network.go:161:5:if` | true | `!reserve(&p.queued, int64(len(raw)), 24<<20)` |
| `internal/p2p/network.go:165:5:select` | default | `(case selection)` |
| `internal/p2p/network.go:287:3:if` | true | `p.pendingCursor >= len(pending)` |
| `internal/p2p/network.go:293:3:if` | true | `err != nil` |
| `internal/p2p/network.go:296:3:if` | true | `err != nil` |
| `internal/p2p/network.go:415:2:if` | true | `records == nil` |
| `internal/p2p/network.go:424:3:if` | true | `err != nil` |
| `internal/p2p/network.go:429:3:if` | true | `len(raw) > 1` |
| `internal/p2p/network.go:432:3:if` | true | `len(encoded) > maxResponse-len(raw)-overhead` |
| `internal/p2p/network.go:433:4:if` | false, true | `len(raw) == 1` |
| `internal/p2p/network.go:439:3:if` | true | `len(raw) > 1` |
| `internal/p2p/network.go:268:3:range` | empty | `records` |
| `internal/p2p/network.go:337:4:range` | empty | `n.rates` |
| `internal/protocol/block.go:163:3:if` | true | `nonce == math.MaxUint64` |
| `internal/protocol/genesis.go:98:2:if` | true | `e != nil` |
| `internal/protocol/genesis.go:47:2:range` | empty | `g.Allocations` |
| `internal/protocol/genesis.go:81:2:range` | empty | `a` |
| `internal/protocol/types.go:26:2:range` | empty | `s` |
| `internal/rpc/server.go:190:2:if` | true | `items == nil` |
| `internal/rpc/server.go:207:2:if` | true | `items == nil` |
| `internal/rpc/server.go:255:2:if` | true | `!s.authenticated(w, r)` |
| `internal/rpc/server.go:276:2:if` | true | `err != nil` |
| `internal/rpc/server.go:287:2:if` | false | `ok` |
| `internal/rpc/server.go:317:3:type_switch` | default | `value := health[name].(type)` |
| `internal/rpc/server.go:326:2:if` | false | `s.cfg.PeerStats != nil` |
| `internal/rpc/server.go:316:2:range` | empty | `[]string{"pending", "round", "observed_voting_weight", "quorum_weight"}` |
| `internal/rpc/server.go:329:4:range` | empty | `[]string{"peers", "connected", "dropped", "received", "queued_bytes"}` |
| `internal/storage/journal.go:44:2:if` | true | `e != nil &#124;&#124; !info.IsDir() &#124;&#124; info.Mode()&os.ModeSymlink != 0` |
| `internal/storage/journal.go:74:2:if` | true | `e != nil` |
| `internal/storage/journal.go:84:3:if` | true | `e != nil` |
| `internal/storage/journal.go:87:3:if` | true | `e != nil` |
| `internal/storage/journal.go:90:3:if` | true | `e != nil` |
| `internal/storage/journal.go:102:3:if` | true | `e != nil` |
| `internal/storage/journal.go:110:4:if` | true | `e != nil` |
| `internal/storage/journal.go:113:4:if` | true | `e != nil` |
| `internal/storage/journal.go:211:2:if` | true | `e != nil` |
| `internal/storage/journal.go:225:3:if` | true | `statErr != nil` |
| `internal/storage/journal.go:244:2:if` | true | `err != nil` |
| `internal/storage/journal.go:268:3:if` | true | `e != nil` |
| `internal/storage/journal.go:272:9:if` | true | `e != nil` |
| `internal/storage/journal.go:312:2:if` | false | `e == nil` |
| `internal/storage/journal.go:316:2:if` | true | `e != nil` |
| `internal/wallet/command.go:87:2:if` | true | `e != nil` |
| `internal/wallet/command.go:173:4:if` | true | `e != nil` |
| `internal/wallet/command.go:178:4:if` | true | `e != nil` |
| `internal/wallet/command.go:183:4:if` | true | `e != nil` |
| `internal/wallet/command.go:188:4:if` | true | `e != nil` |
| `internal/wallet/command.go:194:3:if` | true | `e != nil` |
| `internal/wallet/command.go:215:3:if` | true | `e != nil` |
| `internal/wallet/command.go:229:4:if` | true | `e != nil` |
| `internal/wallet/command.go:241:5:if` | true | `e != nil` |
| `internal/wallet/command.go:256:3:if` | true | `e != nil` |
| `internal/wallet/command.go:260:3:if` | true | `e != nil` |
| `internal/wallet/command.go:266:2:if` | true | `e != nil` |
| `internal/wallet/command.go:287:3:if` | true | `e != nil` |
| `internal/wallet/command.go:298:4:if` | true | `e != nil` |
| `internal/wallet/command.go:303:4:if` | true | `e != nil` |
| `internal/wallet/command.go:312:4:if` | true | `e != nil` |
| `internal/wallet/command.go:319:4:if` | true | `e != nil` |
| `internal/wallet/command.go:325:3:if` | true | `e != nil` |
| `internal/wallet/command.go:331:2:if` | true | `e != nil` |
| `internal/wallet/command.go:336:2:if` | true | `e != nil` |
| `internal/wallet/command.go:341:2:if` | true | `e != nil` |
| `internal/wallet/command.go:355:3:if` | true | `e != nil` |
| `internal/wallet/command.go:358:9:if` | true | `!errors.Is(e, os.ErrNotExist)` |
| `internal/wallet/command.go:362:2:if` | true | `e != nil` |
| `internal/wallet/command.go:366:2:if` | true | `e != nil` |
| `internal/wallet/command.go:376:2:if` | true | `e != nil && !(command == "receive" && client.IsNotFound(e))` |
| `internal/wallet/command.go:385:2:switch` | default | `command` |
| `internal/wallet/command.go:408:3:if` | true | `e != nil` |
| `internal/wallet/command.go:417:4:if` | true | `e != nil` |
| `internal/wallet/command.go:421:5:if` | false | `r.Source == protocol.Hash(source)` |
| `internal/wallet/command.go:425:4:if` | false | `amount != 0 &#124;&#124; len(rows) < 1000` |
| `internal/wallet/command.go:428:4:if` | false, true | `offset >= 1_000_000` |
| `internal/wallet/command.go:456:2:if` | true | `e != nil` |
| `internal/wallet/command.go:459:2:if` | true | `e != nil` |
| `internal/wallet/command.go:463:3:if` | false | `e != nil` |
| `internal/wallet/command.go:472:2:if` | true | `e != nil` |
| `internal/wallet/command.go:484:3:if` | true | `e != nil` |
| `internal/wallet/command.go:487:3:if` | true | `e != nil` |
| `internal/wallet/command.go:502:2:if` | true | `e != nil` |
| `internal/wallet/command.go:509:2:if` | true | `e != nil` |
| `internal/wallet/command.go:551:2:if` | true | `err != nil` |
| `internal/wallet/command.go:543:3:range` | empty | `g.Allocations` |
| `internal/wallet/locking.go:16:2:if` | true | `err != nil` |
| `internal/wallet/locking.go:20:2:if` | true | `err != nil` |
| `internal/wallet/locking.go:30:3:if` | false | `errors.Is(err, syscall.EWOULDBLOCK) &#124;&#124; errors.Is(err, syscall.EAGAIN)` |
| `internal/wallet/wallet.go:51:2:if` | true | `err != nil` |
| `internal/wallet/wallet.go:66:2:if` | true | `err != nil` |
| `internal/wallet/wallet.go:157:2:if` | true | `err != nil` |
| `internal/wallet/wallet.go:160:2:if` | true | `err != nil` |
| `internal/wallet/wallet.go:164:2:if` | true | `err != nil` |
| `internal/wallet/wallet.go:169:2:if` | true | `err != nil` |
| `internal/wallet/wallet.go:173:2:if` | true | `err != nil` |
| `internal/wallet/wallet.go:206:2:if` | true | `err != nil` |
| `internal/wallet/wallet.go:211:2:if` | true | `err != nil` |
| `internal/wallet/wallet.go:215:2:if` | true | `err != nil` |
| `internal/wallet/wallet.go:232:2:if` | true | `err != nil` |
| `internal/wallet/wallet.go:242:2:if` | true | `err != nil` |
| `internal/wallet/wallet.go:245:2:if` | true | `len(data) > maxFileSize` |
| `internal/wallet/wallet.go:257:2:if` | true | `err != nil` |
| `internal/wallet/wallet.go:262:2:if` | true | `err != nil` |
| `internal/wallet/wallet.go:266:2:if` | true | `err != nil` |
| `internal/wallet/wallet.go:270:2:if` | true | `err != nil` |
| `internal/wallet/wallet.go:274:2:if` | true | `err != nil` |
| `internal/wallet/wallet.go:282:2:if` | true | `err != nil` |
| `internal/wallet/wallet.go:286:2:if` | true | `err != nil` |
| `cmd/blnode/main.go:35:2:if` | true | `flags.NArg() != 0` |
| `cmd/blnode/main.go:42:2:if` | true | `*config == ""` |
| `cmd/blnode/main.go:46:2:if` | true | `err != nil` |
