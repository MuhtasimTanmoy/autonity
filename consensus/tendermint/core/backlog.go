package core

import (
	"errors"
	"math/big"

	"github.com/autonity/autonity/common"
	"github.com/autonity/autonity/consensus/tendermint/core/constants"
	"github.com/autonity/autonity/consensus/tendermint/core/message"
)

const MaxSizeBacklogUnchecked = 1000

type backlogMessageEvent struct {
	msg message.Msg
}
type backlogUntrustedMessageEvent struct {
	msg message.Msg
}

func (c *Core) checkMessage(round int64, height uint64) error {
	h := new(big.Int).SetUint64(height)
	switch {
	case h.Cmp(c.Height()) > 0:
		return constants.ErrFutureHeightMessage
	case h.Cmp(c.Height()) < 0:
		return constants.ErrOldHeightMessage
	case round > c.Round():
		return constants.ErrFutureRoundMessage
	case round < c.Round():
		return constants.ErrOldRoundMessage
	}
	return nil
}

func (c *Core) storeBacklog(msg message.Msg, src common.Address) {
	logger := c.logger.New("from", src, "step", c.step)

	if src == c.address {
		logger.Warn("Rejected backloging message, coming from local", "msg", msg)
		return
	}

	logger.Debug("Storing future message")
	c.backlogs[src] = append(c.backlogs[src], msg)
}

// storeFutureMessage push to a special backlog future height consensus messages
// this is done in a way that prevents memory exhaustion in the case of a malicious peer.
func (c *Core) storeFutureMessage(msg message.Msg) {
	// future height messages of a gap wider than one block should not occur frequently as block sync should happen
	// Todo : implement a double ended priority queue (DEPQ)
	msgHeight := msg.H()
	c.backlogUntrusted[msgHeight] = append(c.backlogUntrusted[msgHeight], msg)
	c.backlogUntrustedSize++
	// We discard the furthest ahead messages in priority.
	if c.backlogUntrustedSize == MaxSizeBacklogUnchecked+1 {
		maxHeight := msgHeight
		for k := range c.backlogUntrusted {
			if k > maxHeight && len(c.backlogUntrusted[k]) > 0 {
				maxHeight = k
			}
		}

		// Forget in the local cache that we ever received this message.
		// It's needed for it to be able to be re-received and processed later, after a consensus sync, if needed.
		c.backend.RemoveMessageFromLocalCache(c.backlogUntrusted[maxHeight][len(c.backlogUntrusted[maxHeight])-1])

		// Remove it from the backlog buffer.
		c.backlogUntrusted[maxHeight] = c.backlogUntrusted[maxHeight][:len(c.backlogUntrusted[maxHeight])-1]
		c.backlogUntrustedSize--

		if len(c.backlogUntrusted[maxHeight]) == 0 {
			delete(c.backlogUntrusted, maxHeight)
		}
	}

}

func (c *Core) processBacklog() {
	var capToLenRatio = 5

	for src, backlog := range c.backlogs {
		logger := c.logger.New("from", src, "step", c.step)

		initialLen := len(backlog)
		if initialLen > 0 {
			// For loop will change the size for backlog therefore we need to keep track of the initial length and
			// adjust for index change. This is done by keeping track of how many elements have been removed and
			// subtracting it from the for-loop iterator, since each removed element will cause the index to change for
			// each element after the removed element.
			totalElemRemoved := 0
			for i := 0; i < initialLen; i++ {
				offset := i - totalElemRemoved
				curMsg := backlog[offset]

				r := curMsg.R()
				h := curMsg.H()
				err := c.checkMessage(r, h)
				if errors.Is(err, constants.ErrFutureHeightMessage) || errors.Is(err, constants.ErrFutureRoundMessage) {
					logger.Debug("Future message in backlog", "msg", curMsg, "err", err)
					continue

				}
				logger.Debug("Post backlog event", "msg", curMsg)

				go c.SendEvent(backlogMessageEvent{
					msg: curMsg,
				})

				backlog = append(backlog[:offset], backlog[offset+1:]...)
				totalElemRemoved++
			}
			// We need to ensure that there is no memory leak by reallocating new memory if the original underlying
			// array become very large and only a small part of it is being used by the slice.
			if cap(backlog)/capToLenRatio > len(backlog) {
				tmp := make([]message.Msg, len(backlog))
				copy(tmp, backlog)
				backlog = tmp
			}
		}
		c.backlogs[src] = backlog

	}
	for height := range c.backlogUntrusted {
		if height == c.height.Uint64() {
			for _, msg := range c.backlogUntrusted[height] {
				go c.SendEvent(backlogUntrustedMessageEvent{
					msg: msg,
				})
				c.logger.Debug("Post unchecked backlog event", "msg", msg)
			}
		}
		if height <= c.height.Uint64() {
			delete(c.backlogUntrusted, height)
		}
	}
}
