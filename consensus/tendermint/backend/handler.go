// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package backend

import (
	"errors"

	"github.com/clearmatics/autonity/common"
	"github.com/clearmatics/autonity/consensus"
	"github.com/clearmatics/autonity/consensus/tendermint/events"
	"github.com/clearmatics/autonity/core/types"
	"github.com/clearmatics/autonity/p2p"
	lru "github.com/hashicorp/golang-lru"
)

const (
	tendermintMsg     = 0x11
	tendermintSyncMsg = 0x12
)

type UnhandledMsg struct {
	addr common.Address
	msg  p2p.Msg
}

var (
	// errDecodeFailed is returned when decode message fails
	errDecodeFailed = errors.New("fail to decode tendermint message")
)

// Protocol implements consensus.Handler.Protocol
func (sb *Backend) Protocol() (protocolName string, extraMsgCodes uint64) {
	return "tendermint", 2 //nolint
}

// HandleMsg implements consensus.Handler.HandleMsg
func (sb *Backend) HandleMsg(addr common.Address, msg p2p.Msg) (bool, error) {
	if msg.Code != tendermintMsg && msg.Code != tendermintSyncMsg {
		return false, nil
	}

	sb.coreMu.Lock()
	defer sb.coreMu.Unlock()

	switch msg.Code {
	case tendermintMsg:
		if !sb.coreStarted {
			return true, nil
		}

		var data []byte
		if err := msg.Decode(&data); err != nil {
			return true, errDecodeFailed
		}

		hash := types.RLPHash(data)

		// Mark peer's message
		ms, ok := sb.recentMessages.Get(addr)
		var m *lru.ARCCache
		if ok {
			m, _ = ms.(*lru.ARCCache)
		} else {
			m, _ = lru.NewARC(inmemoryMessages)
			sb.recentMessages.Add(addr, m)
		}
		m.Add(hash, true)

		go sb.Post(events.MessageEvent{
			Payload: data,
		})
	case tendermintSyncMsg:
		if !sb.coreStarted {
			sb.logger.Info("Sync message received but core not running")
			return true, nil // we return nil as we don't want to shutdown the connection if core is stopped
		}
		sb.logger.Info("Received sync message", "from", addr)
		go sb.Post(events.SyncEvent{Addr: addr})
	default:
		return false, nil
	}

	return true, nil
}

// SetBroadcaster implements consensus.Handler.SetBroadcaster
func (sb *Backend) SetBroadcaster(broadcaster consensus.Broadcaster) {
	sb.broadcaster = broadcaster
}

func (sb *Backend) NewChainHead() error {
	sb.coreMu.RLock()
	defer sb.coreMu.RUnlock()
	if !sb.coreStarted {
		return ErrStoppedEngine
	}
	go sb.Post(events.CommitEvent{})
	return nil
}
