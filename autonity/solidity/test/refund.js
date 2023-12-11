const Oracle = artifacts.require("Oracle")
const Autonity = artifacts.require("Autonity")
const truffleAssert = require('truffle-assertions');
const assert = require('assert')
const utils = require('./utils.js');
const toBN = web3.utils.toBN;
const BN = require('bn.js');
const { Buffer } = require('node:buffer');

// this vote refund tests cannot be run on ganache, since it does not have the refund mechanism

contract("Oracle", accounts => {
  let oracle;

  describe('Oracle vote refund', function() {
    before(async() => {
      // for testing the refund we need to interact with the oracle contract deployed at genesis.
      // the refund logic checks if the vote is sent to this specific oracle contract.
      oracle = await Oracle.at("0x47e9Fbef8C83A1714F1951F142132E6e90F5fa5D")
      autonity = await Autonity.at("0xbd770416a3345f91e4b34576cb804a576fa48eb1")

      // register a new validator that we will use for voting in the refund test.
      // using the genesis validator makes fees computations harder because it receives the tip when a block is added to the chain
      let treasury = accounts[8];
      let oracleAddr = accounts[8]
      let nodeAddr = "0xDE03B7806f885Ae79d2aa56568b77caDB0de073E"
      let enode = "enode://a7ecd2c1b8c0c7d7ab9cc12e620605a762865d381eb1bc5417dcf07599571f84ce5725f404f66d3e254d590ae04e4e8f18fe9e23cd29087d095a0c37d0443252@3.209.45.79:30303"

      //let nodeKey = "e59be7e486afab41ec6ef6f23746d78e5dbf9e3f9b0ac699b5566e4f675e976b"
      //let oracleKey = "58951d75562e20501fdcbc8fa6d36b6a10e87aa429ea0e0d302cc0718973f9f2"
      //let treasuryAddr = "0xe12b43b69e57ed6acdd8721eb092bf7c8d41df41"
      let validatorKey = Buffer.from("b4c9a6216f9e39139b8ea2b36f277042bbf5e1198d8e01cff0cca816ce5cc820e219025d2fa399b133d3fc83920eeca5", "hex")
      let multisig = Buffer.from("d4b63f6b5535d7255dbb5ecc5092c7eb042de1d20dff80535321dc1f8fa3cf8844a2927ad86d4e74573b5af4bb69a2a788d0e98a0d2410aed51d355985836cb701c7ad08def133f8fb2778971ad4c225692e3c283b429d0927379904de1ebe31520f4f9f75fc2e7725d9ee09abd915ed2c9d9b6c2bba530c1131f1fcd56ff3eb5901b162451340875b034b45885eec8b0d9e0f56b8c3f89ba795276a011b337816ea6df213dcfb3bd9ee0eba3799638e6dc501166f0b81be73606582f4ddc401980f65888df2f4eaedfa9267703a3b3eee7e8c31ce4db28c01642f735a681e713238","hex")

      await autonity.registerValidator(enode, oracleAddr, validatorKey, multisig, {from: treasury});

      // bond to it
      await autonity.bond(nodeAddr, 10, {from: accounts[8]});

      // wait for epoch to end so that accounts[8] becomes a committee member
      let currentEpoch = (await autonity.epochID()).toNumber()
      for (;;){
        await utils.timeout(5000)
        let epoch = (await autonity.epochID()).toNumber()
        if(epoch > currentEpoch){
          break;
        }
      }
      
      // wait for an additional oracle round so that he is a "fully valid" oracle voter
      let currentRound = (await oracle.getRound()).toNumber()
      for (;;){
        await utils.timeout(5000)
        let round = (await oracle.getRound()).toNumber()
        if(round > currentRound){
          break;
        }
      }

    })
    afterEach(async() => {
      // after each test we wait for a round change, to have a clean contract state again
      let round =  await oracle.getRound();
      let curRound = +round;
      while (+curRound == +round) {
        await utils.timeout(1000);
        curRound = await oracle.getRound();
      }
    })
    it('fee is refunded for valid vote', async function () {
      const proposer = accounts[2];
      const origBalance = toBN(await web3.eth.getBalance(accounts[8]));
      const proposerInitBalance = toBN(await web3.eth.getBalance(proposer));
      const autonityInitBalance = toBN(await web3.eth.getBalance(autonity.address));

      await oracle.vote(0, [], 0, {from:accounts[8]});
     
      // check that voter balance did not change (refund was successfull)
      const updatedBalance = toBN(await web3.eth.getBalance(accounts[8]))
      assert.equal(updatedBalance.toString(), origBalance.toString());

      /*
       * normally the baseFee gets sent to the Autonity Contract for redistribution and the tip to the block proposer (see core/state_transition.go TransitionDb())
       * since for the oracle vote we are refunding both the baseFee and the tip, the balance of the AC and the block proposer should not change.
       */
      assert.equal(await web3.eth.getBalance(proposer), proposerInitBalance.toString(), "proposer balance changed");
      assert.equal(await web3.eth.getBalance(autonity.address), autonityInitBalance.toString(), "autonity balance changed");
    });
    it('double vote, only first is refunded', async function () {
      let currentEpoch = (await autonity.epochID()).toNumber()
      const proposer = accounts[2];
      let proposerInitBalance = toBN(await web3.eth.getBalance(proposer));
      let autonityInitBalance = toBN(await web3.eth.getBalance(autonity.address));
      
      // first vote gets refunded
      let origBalance = toBN(await web3.eth.getBalance(accounts[8]));
      let round = await oracle.getRound()
      await oracle.vote(0, [], 0, {from:accounts[8]});
      let updatedBalance = toBN(await web3.eth.getBalance(accounts[8]))
      assert.equal(updatedBalance.toString(), origBalance.toString());
      
      /*
       * normally the baseFee gets sent to the Autonity Contract for redistribution and the tip to the block proposer (see core/state_transition.go TransitionDb())
       * since for the oracle vote we are refunding both the baseFee and the tip, the balance of the AC and the block proposer should not change.
       */
      assert.equal(await web3.eth.getBalance(proposer), proposerInitBalance.toString(), "proposer balance changed");
      assert.equal(await web3.eth.getBalance(autonity.address), autonityInitBalance.toString(), "autonity balance changed");

      // make sure we are still in the same round
      round2 = await oracle.getRound()
      assert.equal(round.toString(),round2.toString())
      
      // second vote should fail, with !=0 gas expense
      await truffleAssert.fails(
        oracle.vote(0,[],0,{from:accounts[8]}),
        truffleAssert.ErrorType.REVERT,
        "already voted"
      );
      let failedTxHash = (await web3.eth.getBlock("latest")).transactions[0]
      const tx = await web3.eth.getTransaction(failedTxHash);
      const receipt = await web3.eth.getTransactionReceipt(failedTxHash);
      assert.equal(receipt.status,false)
      
      // compute total gasCost, baseFee and effectiveTip
      txBlock = await web3.eth.getBlock(tx.blockNumber)
      baseFee = toBN(txBlock.baseFeePerGas)
      const gasCost = toBN(tx.gasPrice).mul(toBN(receipt.gasUsed));
      const effectiveTip = BN.min(toBN(tx.maxPriorityFeePerGas),toBN(tx.maxFeePerGas).sub(baseFee))
      const tip = effectiveTip.mul(toBN(receipt.gasUsed));
      const baseCost = toBN(baseFee).mul(toBN(receipt.gasUsed));

      // gasCost = baseCost + tip
      assert.equal(gasCost.toString(),baseCost.add(tip).toString())
    
      // make sure that we are still in the same epoch --> no fee redistribution has happened
      let epoch = (await autonity.epochID()).toNumber()
      assert.equal(epoch,currentEpoch)
      
      // gasCost should have been spent
      updatedBalance2 = toBN(await web3.eth.getBalance(accounts[8]))
      assert.equal(updatedBalance2.toString(), updatedBalance.sub(gasCost).toString());

      /*
       * No refund in case of failed vote.
       * check that the basefee has been sent to the autonity contract and the tip to the proposer
       * the proposer is always accounts[2] since we are running on a 1-node autonity test network
       */
      assert.equal(await web3.eth.getBalance(proposer), proposerInitBalance.add(tip).toString(), "proposer did not receive tip");
      assert.equal(await web3.eth.getBalance(autonity.address), autonityInitBalance.add(baseCost).toString(), "autonity did not receive basefee");
    });
  });
});
