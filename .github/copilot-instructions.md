# Cosmos EVM - AI Coding Agent Instructions

## Project Overview

**Cosmos EVM** is a plug-and-play Ethereum Virtual Machine (EVM) module for Cosmos SDK chains. It enables Ethereum compatibility (Solidity contracts, JSON-RPC APIs) while providing deep integration with Cosmos modules through precompiles and extensions.

**Key Traits:**
- Go 1.23.8, Cosmos SDK 0.53.4, Cometbft 0.38.18, Go-Ethereum 1.15.11
- Modular architecture: core modules in `x/vm`, `x/erc20`, `x/feemarket`, `x/precisebank`
- Dual transaction handling: EVM transactions + Cosmos SDK transactions
- JSON-RPC endpoint compatibility via `rpc/backend` and Ethereum client endpoints

## Architecture Fundamentals

### Core Module Structure
- **`x/vm`** - The main EVM module; implements StateDB interface for Geth execution; manages Ethereum accounts, contracts, storage, logs
- **`x/erc20`** - Bidirectional token mapping between IBC/Cosmos tokens and ERC-20 representations
- **`x/feemarket`** - EIP-1559 dynamic fee market mechanism for fee calculation
- **`x/precisebank`** - Handles decimal precision conversion (Cosmos decimals → EVM 18-decimal standard)
- **`x/ibc`** - IBC integration including callbacks, transfer extensions
- **`ante`** - Transaction validation handlers that route EVM vs. Cosmos SDK transactions differently

### Transaction Routing Pattern
Transactions are routed in `ante/ante.go` based on extension options:
```
ExtensionOptionsEthereumTx → EVM ante handler (EVMKeeper-based)
ExtensionOptionDynamicFeeTx → Cosmos ante handler (standard SDK)
```
This dual-path design is critical for understanding validation flows.

### Keeper Pattern & Storage
All modules follow Cosmos SDK keeper pattern:
- Keepers access state via `StoreKey` and `TransientKey` (reset per block)
- `x/vm/keeper.Keeper` implements EVM's `StateDB` interface for execution
- Multi-store architecture: `x/vm/store/snapshotkv` and `snapshotmulti` for atomic state snapshots

### RPC Backend & Eth Conversion
`rpc/backend/` provides JSON-RPC compatibility:
- `comet_to_eth.go` - Converts CometBFT blocks/transactions to Ethereum format
- `RPCBlockFromCometBlock()`, `EthMsgsFromCometBlock()` - Key conversion functions
- Full Ethereum client support (MetaMask, block explorers) through mapped interfaces

## Critical Developer Workflows

### Build & Installation
```bash
make build              # Builds evmd binary to ./build/
make build-linux        # Cross-compile for Linux
./local_node.sh        # Runs local 1-validator test node
```

### Testing
- **Unit Tests:** `make test-unit` (15-minute timeout)
- **Coverage:** `make test-unit-cover` → `filtered_coverage.txt`
- **Race Detection:** `make test-race`
- **Module-Specific:** `make test-evmd` for EVMD app tests
- **Solidity Tests:** `make test-solidity` (requires hardhat setup in `contracts/`)
- **Fuzz Testing:** `make test-fuzz`
- **Benchmarks:** `make benchmark`

### Debugging
- Use `evmd/test_helpers.go::Setup()` for test app initialization
- IBC testing framework in `testutil/ibc/` provides `TestChain` and `SignAndDeliver` helpers
- Test chains accept custom `AppOptions` and can be inspected via keeper accessors

## Key Patterns & Conventions

### 1. Module Registration in App
In `evmd/app.go`, modules are registered via `mm.RegisterModules()`. Each custom module follows:
```go
AppModuleBasic{} // registers codec/query CLI
AppModule{}      // implements BeginBlocker, EndBlocker, message handlers
```

### 2. Precompiles Pattern
`precompiles/` contains contract extensions bridging EVM ↔ Cosmos SDK modules:
- `bank/` - ERC-20 wrapper for native bank transfers
- `staking/` - Staking operations from Solidity
- `distribution/`, `gov/`, `slashing/` - Module integrations
- **Convention:** Each precompile is a custom EVM contract address; queries/calls validated by `x/vm/keeper`

### 3. Account & Balance Handling
Two-step conversion (Cosmos decimals ↔ EVM 18-decimals):
- `x/vm/wrappers` provides `BankWrapper` for conversions
- Token amounts must be converted at boundaries (e.g., `eth_getBalance` RPC)
- **Key:** Always check decimal precision in cross-module calls

### 4. Genesis & Initialization
- `evmd/genesis.go` initializes module genesis states and validator accounts
- Default genesis in `config/evmd_config.go` with module account permissions
- Module permissions define which accounts can Mint/Burn tokens

### 5. Configuration
- Server config in `config/evmd_config.go` and `config/server_app_options.go`
- EVM chain ID (not Cosmos chain ID) controls Ethereum tooling behavior
- Customize via `MsgUpdateParams` (governance-gated)

## Integration Points & Dependencies

### External Dependencies
- **Go-Ethereum (Geth):** Core EVM execution; `vm.EVM`, `core.BlockChain`, tracers
- **Cosmos SDK v0.53.4:** Keepers, Ante handlers, module system
- **CometBFT v0.38.18:** Consensus, block/transaction data
- **IBC-Go v10:** Cross-chain asset transfers, light clients

### Critical File Locations
- **EVM Execution:** `x/vm/keeper/keeper.go` (implements StateDB)
- **State Snapshots:** `x/vm/store/snapshotkv`, `snapshotmulti`
- **RPC Conversion:** `rpc/backend/comet_to_eth.go`
- **Ante Routing:** `ante/ante.go`
- **App Wiring:** `evmd/app.go` (1100+ lines; all modules registered here)

## Common Debugging Scenarios

| Issue | Investigation Path |
|-------|-------------------|
| Transaction rejected before EVM | Check `ante/ante.go` routing & signature verification |
| Wrong account balance in RPC | Verify `x/precisebank` decimal conversion in `x/vm/wrappers` |
| Precompile call fails | Ensure address is registered in `x/vm/keeper` and gas limits sufficient |
| Block hash mismatch | Inspect `comet_to_eth.go` Merkle proof construction |
| IBC token not in ERC-20 | Check `x/erc20` keeper for token pair registration |

## Code Quality Standards

- **Testing:** All public functions should have unit tests; use `require.` assertions
- **Errors:** Use `cosmossdk.io/errors` package with custom error codes
- **Documentation:** Package-level comments and exported function descriptions required
- **Linting:** Go standard practices; checked by `make lint`

## Quick Command Reference

```bash
# Run local node with 1 validator
./local_node.sh

# Build binary
make build

# Run all tests with coverage
make test-unit-cover

# Check for vulnerabilities
make vulncheck

# Format & lint
make fmt && make lint
```

## References
- Cosmos SDK: [cosmos.network](https://cosmos.network/)
- Geth Docs: [go-ethereum.org](https://geth.ethereum.org/)
- Cosmos EVM Docs: [evm.cosmos.network](https://evm.cosmos.network/)
- IBC-Go: [github.com/cosmos/ibc-go](https://github.com/cosmos/ibc-go)
