// SPDX-License-Identifier: MIT
pragma solidity 0.8.15;

// Testing utilities
import { CommonTest } from "test/setup/CommonTest.sol";
import { IGovernanceDelegation } from "src/governance/IGovernanceDelegation.sol";

contract GovernanceTokenInterop_Test is CommonTest {
    address owner;
    address rando;

    // Can't get events and errors from GovernanceDelegation as it's using 0.8.25
    event DelegationsCreated(address indexed account, IGovernanceDelegation.Delegation[] delegations);
    event DelegateVotesChanged(address indexed delegate, uint256 previousBalance, uint256 newBalance);

    /// @dev Sets up the test suite.
    function setUp() public virtual override {
        super.enableInterop();
        super.setUp();
        owner = governanceToken.owner();
        rando = makeAddr("rando");
    }

    /// @dev Tests that the constructor sets the correct initial state.
    function test_constructor_succeeds() external view {
        assertEq(governanceToken.owner(), owner);
        assertEq(governanceToken.name(), "Optimism");
        assertEq(governanceToken.symbol(), "OP");
        assertEq(governanceToken.decimals(), 18);
        assertEq(governanceToken.totalSupply(), 0);
    }

    /// @dev Tests that the owner can successfully call `mint`.
    function test_mint_fromOwner_succeeds() external {
        // Mint 100 tokens.
        vm.prank(owner);
        governanceToken.mint(owner, 100);

        // Balances have updated correctly.
        assertEq(governanceToken.balanceOf(owner), 100);
        assertEq(governanceToken.totalSupply(), 100);
    }

    /// @dev Tests that `mint` reverts when called by a non-owner.
    function test_mint_fromNotOwner_reverts() external {
        // Mint 100 tokens as rando.
        vm.prank(rando);
        vm.expectRevert("Ownable: caller is not the owner");
        governanceToken.mint(owner, 100);

        // Balance does not update.
        assertEq(governanceToken.balanceOf(owner), 0);
        assertEq(governanceToken.totalSupply(), 0);
    }

    /// @dev Tests that the owner can successfully call `burn`.
    function test_burn_succeeds() external {
        // Mint 100 tokens to rando.
        vm.prank(owner);
        governanceToken.mint(rando, 100);

        // Rando burns their tokens.
        vm.prank(rando);
        governanceToken.burn(50);

        // Balances have updated correctly.
        assertEq(governanceToken.balanceOf(rando), 50);
        assertEq(governanceToken.totalSupply(), 50);
    }

    /// @dev Tests that the owner can successfully call `burnFrom`.
    function test_burnFrom_succeeds() external {
        // Mint 100 tokens to rando.
        vm.prank(owner);
        governanceToken.mint(rando, 100);

        // Rando approves owner to burn 50 tokens.
        vm.prank(rando);
        governanceToken.approve(owner, 50);

        // Owner burns 50 tokens from rando.
        vm.prank(owner);
        governanceToken.burnFrom(rando, 50);

        // Balances have updated correctly.
        assertEq(governanceToken.balanceOf(rando), 50);
        assertEq(governanceToken.totalSupply(), 50);
    }

    /// @dev Tests that `transfer` correctly transfers tokens.
    function test_transfer_succeeds() external {
        // Mint 100 tokens to rando.
        vm.prank(owner);
        governanceToken.mint(rando, 100);

        // Rando transfers 50 tokens to owner.
        vm.prank(rando);
        governanceToken.transfer(owner, 50);

        // Balances have updated correctly.
        assertEq(governanceToken.balanceOf(owner), 50);
        assertEq(governanceToken.balanceOf(rando), 50);
        assertEq(governanceToken.totalSupply(), 100);
    }

    /// @dev Tests that `approve` correctly sets allowances.
    function test_approve_succeeds() external {
        // Mint 100 tokens to rando.
        vm.prank(owner);
        governanceToken.mint(rando, 100);

        // Rando approves owner to spend 50 tokens.
        vm.prank(rando);
        governanceToken.approve(owner, 50);

        // Allowances have updated.
        assertEq(governanceToken.allowance(rando, owner), 50);
    }

    /// @dev Tests that `transferFrom` correctly transfers tokens.
    function test_transferFrom_succeeds() external {
        // Mint 100 tokens to rando.
        vm.prank(owner);
        governanceToken.mint(rando, 100);

        // Rando approves owner to spend 50 tokens.
        vm.prank(rando);
        governanceToken.approve(owner, 50);

        // Owner transfers 50 tokens from rando to owner.
        vm.prank(owner);
        governanceToken.transferFrom(rando, owner, 50);

        // Balances have updated correctly.
        assertEq(governanceToken.balanceOf(owner), 50);
        assertEq(governanceToken.balanceOf(rando), 50);
        assertEq(governanceToken.totalSupply(), 100);
    }

    /// @dev Tests that `increaseAllowance` correctly increases allowances.
    function test_increaseAllowance_succeeds() external {
        // Mint 100 tokens to rando.
        vm.prank(owner);
        governanceToken.mint(rando, 100);

        // Rando approves owner to spend 50 tokens.
        vm.prank(rando);
        governanceToken.approve(owner, 50);

        // Rando increases allowance by 50 tokens.
        vm.prank(rando);
        governanceToken.increaseAllowance(owner, 50);

        // Allowances have updated.
        assertEq(governanceToken.allowance(rando, owner), 100);
    }

    /// @dev Tests that `decreaseAllowance` correctly decreases allowances.
    function test_decreaseAllowance_succeeds() external {
        // Mint 100 tokens to rando.
        vm.prank(owner);
        governanceToken.mint(rando, 100);

        // Rando approves owner to spend 100 tokens.
        vm.prank(rando);
        governanceToken.approve(owner, 100);

        // Rando decreases allowance by 50 tokens.
        vm.prank(rando);
        governanceToken.decreaseAllowance(owner, 50);

        // Allowances have updated.
        assertEq(governanceToken.allowance(rando, owner), 50);
    }

    /// @dev Tests that `delegate` correctly delegates and emits event.
    function test_delegate_succeeds() external {
        // Mint 100 tokens to rando.
        vm.prank(owner);
        governanceToken.mint(rando, 100);

        IGovernanceDelegation.Delegation[] memory delegations = new IGovernanceDelegation.Delegation[](1);
        delegations[0] = IGovernanceDelegation.Delegation(IGovernanceDelegation.AllowanceType.Relative, owner, 1e4);

        // Rando approves owner to spend 100 tokens.
        vm.prank(rando);
        vm.expectEmit(address(governanceDelegation));
        emit DelegateVotesChanged(owner, 0, 100);
        vm.expectEmit(address(governanceDelegation));
        emit DelegationsCreated(rando, delegations);
        governanceToken.delegate(owner);
    }

    /// @dev Tests that `delegateBySig` correctly delegates and emits event.
    function test_delegateBySig_succeeds() external {
        // TODO
    }
}
