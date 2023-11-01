package byzantine

import (
	"context"
	"github.com/autonity/autonity/common"
	"github.com/autonity/autonity/consensus/tendermint/core"
	"github.com/autonity/autonity/consensus/tendermint/core/interfaces"
	"github.com/autonity/autonity/consensus/tendermint/core/message"
	"github.com/autonity/autonity/core/types"
	"github.com/autonity/autonity/e2e_test"
	"github.com/autonity/autonity/node"
	fuzz "github.com/google/gofuzz"
	"github.com/stretchr/testify/require"
	"regexp"
	"testing"
)

type randomBytesBroadcaster struct {
	*core.Core
}

func (s *randomBytesBroadcaster) Broadcast(ctx context.Context, _ message.Message) {
	logger := s.Logger().New("step", s.Step())
	logger.Info("Broadcasting random bytes")

	for i := 0; i < 1000; i++ {
		payload, err := e2e.GenerateRandomBytes(2048)
		if err != nil {
			logger.Error("Failed to generate random bytes ", "err", err)
			return
		}
		var hash common.Hash
		copy(hash[:], payload)
		msg := message.NewFake(1, payload, hash)
		s.BroadcastAll(ctx, msg)
	}
}

// TestRandomBytesBroadcaster broadcasts random bytes in the network,
// We expect other nodes to detect this misbehaviour and discard these messages
// Receiving nodes should also disconnect misbehaving nodes
func TestRandomBytesBroadcaster(t *testing.T) {
	users, err := e2e.Validators(t, 6, "10e18,v,100,0.0.0.0:%s,%s")
	require.NoError(t, err)

	//set Malicious users
	users[0].TendermintServices = &node.TendermintServices{Broadcaster: &randomBytesBroadcaster{}}
	// creates a network of 6 users and starts all the nodes in it
	network, err := e2e.NewNetworkFromValidators(t, users, true)
	require.NoError(t, err)
	defer network.Shutdown()

	// network should be up and continue to mine blocks
	err = network.WaitToMineNBlocks(10, 180)
	require.NoError(t, err, "Network should be mining new blocks now, but it's not")
}

type garbageMessageBroadcaster struct {
	*core.Core
}

func (s *garbageMessageBroadcaster) Broadcast(ctx context.Context, _ message.Message) {
	logger := s.Logger().New("step", s.Step())
	var fMsg message.Prevote
	f := fuzz.New().NilChance(0.5).Funcs(
		func(cm message.Message, c fuzz.Continue) {
			c.Fuzz(cm)
		})
	f.Fuzz(&fMsg)
	logger.Info("Broadcasting random bytes")
	s.BroadcastAll(ctx, &fMsg)
}

// TestGarbageMessageBroadcaster broadcasts a garbage Messages in the network,
// We expect other nodes to detect this misbehaviour and discard these messages
// Receiving nodes should also disconnect misbehaving nodes
func TestGarbageMessageBroadcaster(t *testing.T) {
	users, err := e2e.Validators(t, 6, "10e18,v,100,0.0.0.0:%s,%s")
	require.NoError(t, err)

	//set Malicious users
	users[0].TendermintServices = &node.TendermintServices{Broadcaster: &garbageMessageBroadcaster{}}
	// creates a network of 6 users and starts all the nodes in it
	network, err := e2e.NewNetworkFromValidators(t, users, true)
	require.NoError(t, err)
	defer network.Shutdown()

	// network should be up and continue to mine blocks
	err = network.WaitToMineNBlocks(10, 180)
	require.NoError(t, err, "Network should be mining new blocks now, but it's not")
}

type garbagePrecommitSender struct {
	*core.Core
	interfaces.Precommiter
}

func (c *garbagePrecommitSender) SendPrecommit(ctx context.Context, isNil bool) {
	var precommit *message.Precommit
	precommitFieldComb := e2e.GetAllFieldCombinations(precommit)
	proposedBlockHash := common.Hash{}
	if !isNil {
		if h := c.CurRoundMessages().ProposalHash(); h == (common.Hash{}) {
			c.Logger().Error("Core.sendPrecommit Proposal is empty! It should not be empty!")
			return
		}
		proposedBlockHash = c.CurRoundMessages().ProposalHash()
	}
	//Each iteration tries to fuzz a unique set of fields and skipping
	// a few as provided by fieldsArray
	for _, fieldsArray := range precommitFieldComb {
		// a valid proposal block
		f := fuzz.New().NilChance(0.5)
		f.AllowUnexportedFields(true)
		for _, fieldName := range fieldsArray {
			f.SkipFieldsWithPattern(regexp.MustCompile(fieldName))
		}
		precommit = message.NewVote[message.Precommit](c.Round(), c.Height().Uint64(), proposedBlockHash, c.Backend().Sign)
		// fuzzing existing precommit message, skip the fields in field array
		f.Fuzz(&precommit)
		c.SetSentPrecommit(true)
		c.Br().Broadcast(ctx, precommit)
	}
}

// TestGarbagePrecommitter broadcasts a garbage precommit message in the network,
// We expect other nodes to detect this misbehaviour and discard these messages
// Receiving nodes should also disconnect misbehaving nodes
func TestGarbagePrecommitter(t *testing.T) {
	users, err := e2e.Validators(t, 6, "10e18,v,100,0.0.0.0:%s,%s")
	require.NoError(t, err)

	//set Malicious users
	users[0].TendermintServices = &node.TendermintServices{Precommitter: &garbagePrecommitSender{}}
	// creates a network of 6 users and starts all the nodes in it
	network, err := e2e.NewNetworkFromValidators(t, users, true)
	require.NoError(t, err)
	defer network.Shutdown()

	// network should be up and continue to mine blocks
	err = network.WaitToMineNBlocks(10, 120)
	require.NoError(t, err, "Network should be mining new blocks now, but it's not")
}

type garbagePrevoter struct {
	*core.Core
	interfaces.Prevoter
}

func (c *garbagePrevoter) SendPrevote(ctx context.Context, isNil bool) {
	var prevote message.Prevote
	prevoteFieldComb := e2e.GetAllFieldCombinations(&prevote)
	proposedBlockHash := c.CurRoundMessages().ProposalHash()
	if !isNil {
		if h := c.CurRoundMessages().ProposalHash(); h == (common.Hash{}) {
			c.Logger().Error("sendPrecommit Proposal is empty! It should not be empty!")
			return
		}
		proposedBlockHash = c.CurRoundMessages().ProposalHash()
	}

	//Each iteration tries to fuzz a unique set of fields and skipping
	// a few as provided by fieldsArray
	for _, fieldsArray := range prevoteFieldComb {
		// a valid proposal block
		f := fuzz.New().NilChance(0.5)
		f.AllowUnexportedFields(true)
		for _, fieldName := range fieldsArray {
			f.SkipFieldsWithPattern(regexp.MustCompile(fieldName))
		}
		prevote := message.NewVote[message.Prevote](c.Round(), c.Height().Uint64(), proposedBlockHash, c.Backend().Sign)
		f.Fuzz(prevote)
		c.BroadcastAll(ctx, prevote)
	}
	c.SetSentPrevote(true)
}

// TestGarbagePrevoter broadcasts a garbage prevote message in the network,
// We expect other nodes to detect this misbehaviour and discard these messages
// Receiving nodes should also disconnect misbehaving nodes
func TestGarbagePrevoter(t *testing.T) {
	users, err := e2e.Validators(t, 6, "10e18,v,100,0.0.0.0:%s,%s")
	require.NoError(t, err)

	//set Malicious users
	users[0].TendermintServices = &node.TendermintServices{Prevoter: &garbagePrevoter{}}
	// creates a network of 6 users and starts all the nodes in it
	network, err := e2e.NewNetworkFromValidators(t, users, true)
	require.NoError(t, err)
	defer network.Shutdown()

	// network should be up and continue to mine blocks
	err = network.WaitToMineNBlocks(10, 120)
	require.NoError(t, err, "Network should be mining new blocks now, but it's not")
}

type garbageProposer struct {
	*core.Core
	interfaces.Proposer
}

/*
type structNode struct {
	fName string
	sMap  map[string]*structNode
	fList []string
}

func generateFieldMap(v interface{}) map[string]reflect.Value {
	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Ptr {
		panic("Need pointer!")
	}
	outMap := make(map[string]reflect.Value)
	e := reflect.ValueOf(v).Elem()
	for i := 0; i < e.NumField(); i++ {
		fmt.Println("handling field => ", e.Type().Field(i).Name)
		if e.Field(i).Type().Kind() == reflect.Ptr {
			fKind := e.Field(i).Type().Elem().Kind()
			if fKind == reflect.Struct {
				fmt.Println("TODO - handle recursively")
			}
		} else if e.Field(i).Type().Kind() == reflect.Struct {
			fmt.Println("TODO - handle recursively")
		}
		outMap[e.Type().Field(i).Name] = e.Field(i)
	}
	return outMap
}
*/

func (c *garbageProposer) SendProposal(ctx context.Context, p *types.Block) {
	var proposalMsg *message.Propose
	allComb := e2e.GetAllFieldCombinations(proposalMsg)
	//Each iteration tries to fuzz a unique set of fields and skipping
	// a few as provided by fieldsArray
	for _, fieldsArray := range allComb {
		// a valid proposal block
		proposalMsg = message.NewPropose(c.Round(), c.Height().Uint64(), c.ValidRound(), p, c.Backend().Sign)
		f := fuzz.New().NilChance(0)
		f.AllowUnexportedFields(true)
		for _, fieldName := range fieldsArray {
			f.SkipFieldsWithPattern(regexp.MustCompile(fieldName))
		}

		f.Funcs(
			func(r *any, fc fuzz.Continue) {},
			func(tr *types.TxData, fc fuzz.Continue) {
				var txData types.LegacyTx
				fc.Fuzz(&txData)
				*tr = &txData
			},
			func(tr **types.Transaction, fc fuzz.Continue) {
				var fakeTransaction types.Transaction
				fc.Fuzz(&fakeTransaction)
				*tr = &fakeTransaction
			},
		)
		for i := 0; i < 100; i++ {
			f.Fuzz(proposalMsg)
			c.SetSentProposal(true)
			c.Backend().SetProposedBlockHash(p.Hash())
			c.BroadcastAll(ctx, proposalMsg)
		}
	}
}

// TestGarbagePrevoter broadcasts a garbage proposal message in the network,
// We expect other nodes to detect this misbehaviour and discard these messages
// Receiving nodes should also disconnect misbehaving nodes
func TestGarbageProposer(t *testing.T) {
	users, err := e2e.Validators(t, 6, "10e18,v,100,0.0.0.0:%s,%s")
	require.NoError(t, err)

	//set Malicious users
	users[0].TendermintServices = &node.TendermintServices{Proposer: &garbageProposer{}}
	// creates a network of 6 users and starts all the nodes in it
	network, err := e2e.NewNetworkFromValidators(t, users, true)
	require.NoError(t, err)
	defer network.Shutdown()

	// network should be up and continue to mine blocks
	err = network.WaitToMineNBlocks(10, 120)
	require.NoError(t, err, "Network should be mining new blocks now, but it's not")
}
