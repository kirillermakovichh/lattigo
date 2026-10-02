# Block-lattice domain

A payment network in which account owners authorize account-chain changes and
representatives participate in confirmation.

## Accounts and transfers

**Account**: An identity that owns a balance and authorizes its account-chain blocks.

**Account chain**: The ordered history of blocks signed by one account owner.
_Avoid_: global chain.

**Block lattice**: The network of account chains connected by transfer dependencies.

**State block**: A signed statement of an account's successor state.

**Frontier**: The current head of an account chain in a specified ledger view.

**Send**: An account-chain change that reduces the sender's balance and creates a receivable transfer.

**Receivable**: A transfer created by a send that the destination has not consumed.
_Avoid_: unspent output, pending balance.

**Receive**: An account-chain change that consumes a receivable and increases the destination balance.

**Representative change**: An account-chain change that assigns the account's voting delegation.

## Confirmation

**Representative**: A voting identity to which accounts delegate weight without transferring funds.

**Voting weight**: The amount of voting influence attributed to a representative under the protocol's weight rules.

**Conflict**: Competing successor blocks that attempt to occupy the same account-chain position.

**Election**: The process of choosing a block for one conflicting account-chain position.

**Vote**: A representative's signed statement supporting an election candidate.

**Quorum**: The protocol-defined voting condition required for an election outcome.

**Confirmation**: The protocol's final acceptance of a block and its required dependencies.
_Avoid_: admission, storage success.

**Confirmed frontier**: The latest confirmed block of an account chain.
