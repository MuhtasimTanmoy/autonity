package interfaces

import (
	"context"
	"math/big"

	"github.com/autonity/autonity/common"
	"github.com/autonity/autonity/consensus/tendermint/core/message"
)

type Precommiter interface {
	SendPrecommit(ctx context.Context, isNil bool)
	HandlePrecommit(ctx context.Context, msg *message.Precommit) error
	VerifyCommittedSeal(addressMsg common.Address, committedSealMsg []byte, proposedBlockHash common.Hash, round int64, height *big.Int) error
	HandleCommit(ctx context.Context)
	LogPrecommitMessageEvent(message string, precommit *message.Precommit, from, to string)
}
