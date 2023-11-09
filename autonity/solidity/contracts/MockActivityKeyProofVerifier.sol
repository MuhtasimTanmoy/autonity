// SPDX-License-Identifier: LGPL-3.0-only
pragma solidity ^0.8.19;

contract MockActivityKeyProofVerifier {

    constructor(){}

    fallback(bytes calldata input) external payable returns (bytes memory ret) {
        ret = new bytes(32); // 0 --> err, 1 --> success
        if (input.length != 196) {
            return ret;
        }
        ret[0] = bytes1(uint8(1));
        return ret;
    }
}
