# Kontrol Verification

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Getting Started](#getting-started)
  - [Overview](#overview)
  - [Directory structure](#directory-structure)
  - [Installation](#installation)
- [Usage](#usage)
  - [Build Deployment Summary](#build-deployment-summary)
  - [Execute Proofs](#execute-proofs)
  - [Add New Proofs](#add-new-proofs)
- [Implementation Details](#implementation-details)
  - [Assumptions](#assumptions)
  - [Deployment Summary Process](#deployment-summary-process)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Getting Started

### Overview

[Kontrol](https://github.com/runtimeverification/kontrol) is a tool by [Runtime Verification](https://runtimeverification.com/) (RV) that enables formal verification of Ethereum smart contracts. Quoting from the Kontrol [docs](https://docs.runtimeverification.com/kontrol/overview/readme):

> Kontrol combines [KEVM](https://github.com/runtimeverification/evm-semantics) and [Foundry](https://book.getfoundry.sh/) to grant developers the ability to perform [formal verification](https://en.wikipedia.org/wiki/Formal_verification) without learning a new language or tool. This is especially useful for those who are not verification engineers. Additionally, developers can leverage Foundry test suites they have already developed and use symbolic execution to increase the level of confidence.
>
> KEVM is a tool that enables formal verification of smart contracts on the Ethereum blockchain. It provides a mathematical foundation for specifying and implementing smart contracts. Developers can use KEVM to rigorously reason about the behavior of their smart contracts, ensuring correctness and reducing the likelihood of vulnerabilities in the contract code.

This document details the Kontrol setup used in this repo to run various proofs against the contracts in the [`contracts-bedrock`](../../) directory.

### Directory structure

The directory is structured as follows

<pre>
├── <a href="./pausability-lemmas.k">pausability-lemmas.k</a>: File containing the necessary lemmas for this project
├── <a href="./deployment">deployment</a>: Custom deploy sequence for Kontrol proofs and tests for its <a href="https://github.com/runtimeverification/kontrol/pull/271">fast summarization</a>
│   ├── <a href="./deployment/KontrolDeployment.sol">KontrolDeployment.sol</a>: Simplified deployment sequence for Kontrol proofs
│   └── <a href="./deployment/DeploymentSummary.t.sol">DeploymentSummary.t.sol</a>: Tests for the summarization of custom deployment
├── <a href="./proofs">proofs</a>: Where the proofs (tests) themselves live
│   ├── *.k.sol</a>: Symbolic property tests for contracts
│   ├── <a href="./proofs/interfaces">interfaces</a>: Interface files for src contracts, to avoid unnecessary compilation of contracts
│   └── <a href="./proofs/utils">utils</a>: Proof dependencies, including the autogenerated deployment summary contracts
└── <a href="./scripts">scripts</a>: Where the scripts of the projects live
    ├── <a href="./scripts/json">json</a>: Data cleaning scripts for the output of <a href="./deployment/KontrolDeployment.sol">KontrolDeployment.sol</a>
    ├── <a href="./scripts/make-summary-deployment.sh">make-summary-deployment.sh</a>: Executes <a href="./deployment/KontrolDeployment.sol">KontrolDeployment.sol</a>, curates the result and writes the summary deployment contract
    └── <a href="./scripts/run-kontrol.sh">run-kontrol.sh</a>: Wrapper around the kontrol CLI to run the proofs
</pre>

### Installation

1. `cd` to the root of this repo.
2. Install Foundry by running `pnpm install:foundry`. This installs `foundryup`, the foundry toolchain installer, then installs the required foundry version.
3. Install Kontrol by running `pnpm install:kontrol`. This installs `kup`, the package manager for RV tools, then installs the required kontrol version.
4. Install Docker.

## Usage

Verifying proofs has two steps: build, and execute.

### Build Deployment Summary

First, generate a deployment summary contract from the deploy script in [`KontrolDeployment.sol`](./deployment/KontrolDeployment.sol) by running the following command:

```bash
./test/kontrol/scripts/make-summary-deployment.sh
```

[`KontrolDeployment.sol`](./deployment/KontrolDeployment.sol) contains the minimal deployment sequence required by the proofs.
The [`make-summary-deployment.sh`](./scripts/make-summary-deployment.sh) script will generate a JSON state diff. This state diff is used in two ways by Kontrol.
First, Kontrol generates a summary contract recreating the state diffs recorded in the JSON. This contract is used to test that the information contained in the generated JSON is correct and aids in the specification of the symbolic property tests. The generation of this contract is also handled by the `make-summary-deployment.sh` script.
Second, the state diff JSON is used to load the post-deployment state directly into Kontrol when running the proofs.

This step is optional if an up-to-date summary contract already exists, which will be the case until the `KontrolDeployment` contract changes, or any of the source contracts under test change.
See the [Implementation Details](#implementation-details) section for more information, and to learn about the CI check that ensures the committed autogenerated files from this step are up-to-date.

The summary contract can be found in [`DeploymentSummary.sol`](./proofs/utils/DeploymentSummary.sol), which is summarization (state changes) of the [`KontrolDeployment.sol`](./deployment/KontrolDeployment.sol) contract.

### Execute Proofs

Use the [`run-kontrol.sh`](./scripts/run-kontrol.sh) script to runs the proofs in all `*.k.sol` files.

```
./test/kontrol/scripts/run-kontrol.sh [container|local|dev]
```

The `run-kontrol.sh` script supports three modes of proof execution:

- `container`: Runs the proofs using the same Docker image used in CI. This is the default execution mode—if no arguments are provided, the proofs will be executed in this mode.
- `local`: Runs the proofs with your local Kontrol install, and enforces that the Kontrol version matches the one used in CI, which is specified in [`versions.json`](../../../../versions.json).
- `dev`: Run the proofs with your local Kontrol install, without enforcing any version in particular. The intended use case is proof development and related matters.

For a similar description of the options run `run-kontrol.sh --help`.

### Add New Proofs

More details on best practices for writing and adding new proofs will be added here in the future.
The summary is:

1. Update the deployment summary and its tests as needed.
2. Write the proofs in an appropriate `*.k.sol` file in the `proofs` folder.
3. Add the proof name to the `test_list` array in the [`run-kontrol.sh`](./scripts/run-kontrol.sh) script.

## Implementation Details

### Assumptions

1. A critical invariant of the `KontrolDeployment.sol` contract is that it stays in sync with the original `Deploy.s.sol` contract.
   Currently, this is partly enforced by running some of the standard post-`setUp` deployment assertions in `DeploymentSummary.t.sol`.
   A more rigorous approach may be to leverage the `ChainAssertions` library, but more investigation is required to determine if this is feasible without large changes to the deploy script.

2. Until symbolic bytes are natively supported in Kontrol, we must make assumptions about the length of `bytes` parameters.
   All current assumptions can be found by searching for `// ASSUME:` comments in the files.
   Some of this assumptions can be lifted once [symbolic bytes](https://github.com/runtimeverification/kontrol/issues/272) are supported in Kontrol.

### Deployment Summary Process

As mentioned above, a deployment summary contract is first generated before executing the proofs.
This is because the proof execution leverages Kontrol's [fast summarization](https://github.com/runtimeverification/kontrol/pull/271) feature, which allows loading the post-`setUp` state directly into Kontrol.
This provides a significant reduction in proof execution time, as it avoids the need to execute the deployment script every time the proofs are run.

All code executed in Kontrol—even when execution is concrete and not symbolic—is significantly slower than in Foundry, due to the mathematical representation of the EVM in Kontrol.
Therefore we want to minimize the amount of code executed in Kontrol, and the fast summarization feature allows us to reduce `setUp` execution time.

This project uses two different [`foundry.toml` profiles](../../foundry.toml), `kdeploy` and `kprove`, to facilitate usage of this fast summarization feature.:

- `kdeploy`: Used by [`make-summary-deployment.sh`](./scripts/make-summary-deployment.sh) to generate the `DeploymentSummary.sol` contract based on execution of the `KontrolDeployment.sol` contract using Foundry's state diff recording cheatcodes.
  This is where all necessary [`src/L1`](../../src/L1) files are compiled with their bytecode saved into the `DeploymentSummaryCode.sol` file, which is inherited by `DeploymentSummary`.

- `kprove`: Used by the [`run-kontrol.sh`](./scrpts/run-kontrol.sh) script to execute proofs, which can be run once a `DeploymentSummary.sol` contract is present. This profile's `src` and `script` paths point to a test folder because we only want to compile what is in the `test/kontrol/proofs` folder, since that folder contains all bytecode and proofs.

The `make-summary-deployment.sh` scripts saves off the generated JSON state diff to `snapshots/state-diff/Kontrol-Deploy.json`, and is run as part of the `snapshots` script in `package.json`.
Therefore, the snapshots CI check will fail if the committed Kontrol state diff is out of sync.
Note that the CI check only compares the JSON state diff, not the generated `DeploymentSummary.sol` or `DeploymentSummaryCode` contracts.
This is for simplicity, as those three files will be in sync upon successful execution of the `make-summary-deployment.sh` script.
We commit the `DeploymentSummary.sol` and `DeploymentSummaryCode.sol` contracts, because forge fails to build if those contracts are not present—it is simpler to commit these autogenerated files than to workaround their absence in forge builds.

During `make-summary-deployment.sh`, the `mustGetAddress` usage in `Deploy.s.sol` is temporarily replaced by `getAddress`—the former reverts if the deployed contract does not exist, while the latter returns the zero address.
This is required because the deploy script in `KontrolDeployment.sol` is does not fully reproduce all deployments in `Deploy.s.sol`, so the `mustGetAddress` usage would cause the script to revert since some contracts are not deployed.
`KontrolDeployment.sol` is a simplified, minimal deployment sequence for Kontrol proofs, and is not intended to be a full deployment sequence for the contracts in `contracts-bedrock`.