package intermediate

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	logger "github.com/ElrondNetwork/elrond-go-logger"
	"github.com/ElrondNetwork/elrond-go/core/check"
	"github.com/ElrondNetwork/elrond-go/genesis"
	"github.com/ElrondNetwork/elrond-go/node/external"
	"github.com/ElrondNetwork/elrond-go/process"
	"github.com/ElrondNetwork/elrond-go/sharding"
)

// ArgStandardDelegationProcessor is the argument used to construct a standard delegation processor
type ArgStandardDelegationProcessor struct {
	Executor            genesis.TxExecutionProcessor
	ShardCoordinator    sharding.Coordinator
	AccountsParser      genesis.AccountsParser
	SmartContractParser genesis.InitialSmartContractParser
	NodesListSplitter   genesis.NodesListSplitter
	QueryService        external.SCQueryService
	NodePrice           *big.Int
}

const stakeFunction = "stakeGenesis"
const addNodesFunction = "addNodes"
const activateFunction = "activateGenesis"
const setStakePerNodeFunction = "setStakePerNode"

var log = logger.GetOrCreate("genesis/process/intermediate")
var zero = big.NewInt(0)
var genesisSignature = make([]byte, 32)

type standardDelegationProcessor struct {
	genesis.TxExecutionProcessor
	shardCoordinator     sharding.Coordinator
	accuntsParser        genesis.AccountsParser
	smartContractsParser genesis.InitialSmartContractParser
	nodesListSplitter    genesis.NodesListSplitter
	queryService         external.SCQueryService
	nodePrice            *big.Int
}

// NewStandardDelegationProcessor returns a new standard delegation processor instance
func NewStandardDelegationProcessor(arg ArgStandardDelegationProcessor) (*standardDelegationProcessor, error) {
	if check.IfNil(arg.Executor) {
		return nil, genesis.ErrNilTxExecutionProcessor
	}
	if check.IfNil(arg.ShardCoordinator) {
		return nil, genesis.ErrNilShardCoordinator
	}
	if check.IfNil(arg.AccountsParser) {
		return nil, genesis.ErrNilAccountsParser
	}
	if check.IfNil(arg.SmartContractParser) {
		return nil, genesis.ErrNilSmartContractParser
	}
	if check.IfNil(arg.NodesListSplitter) {
		return nil, genesis.ErrNilNodesListSplitter
	}
	if check.IfNil(arg.QueryService) {
		return nil, genesis.ErrNilQueryService
	}
	if arg.NodePrice == nil {
		return nil, genesis.ErrNilInitialNodePrice
	}
	if arg.NodePrice.Cmp(zero) <= 0 {
		return nil, genesis.ErrInvalidInitialNodePrice
	}

	return &standardDelegationProcessor{
		TxExecutionProcessor: arg.Executor,
		shardCoordinator:     arg.ShardCoordinator,
		accuntsParser:        arg.AccountsParser,
		smartContractsParser: arg.SmartContractParser,
		nodesListSplitter:    arg.NodesListSplitter,
		queryService:         arg.QueryService,
		nodePrice:            arg.NodePrice,
	}, nil
}

// ExecuteDelegation will execute stake, set bls keys and activate on all delegation contracts from this shard
func (sdp *standardDelegationProcessor) ExecuteDelegation() (genesis.DelegationResult, error) {
	smartContracts, err := sdp.getDelegationScOnCurrentShard()
	if err != nil {
		return genesis.DelegationResult{}, err
	}
	if len(smartContracts) == 0 {
		return genesis.DelegationResult{}, nil
	}

	err = sdp.setDelegationStartParameters(smartContracts)
	if err != nil {
		return genesis.DelegationResult{}, err
	}

	dr := genesis.DelegationResult{}
	dr.NumTotalDelegated, err = sdp.executeManageBlsKeys(smartContracts)
	if err != nil {
		return genesis.DelegationResult{}, err
	}

	dr.NumTotalStaked, err = sdp.executeStake(smartContracts)
	if err != nil {
		return genesis.DelegationResult{}, err
	}

	err = sdp.executeActivation(smartContracts)
	if err != nil {
		return genesis.DelegationResult{}, err
	}

	err = sdp.executeVerify(smartContracts)
	if err != nil {
		return genesis.DelegationResult{}, err
	}

	return dr, err
}

func (sdp *standardDelegationProcessor) getDelegationScOnCurrentShard() ([]genesis.InitialSmartContractHandler, error) {
	allSmartContracts, err := sdp.smartContractsParser.InitialSmartContractsSplitOnOwnersShards(sdp.shardCoordinator)
	if err != nil {
		return nil, err
	}

	smartContracts := make([]genesis.InitialSmartContractHandler, 0)
	smartContractsForCurrentShard := allSmartContracts[sdp.shardCoordinator.SelfId()]
	for _, sc := range smartContractsForCurrentShard {
		if sc.GetType() == genesis.DelegationType {
			smartContracts = append(smartContracts, sc)
		}
	}

	log.Trace("getDelegationScOnCurrentShard",
		"num delegation SC", len(smartContracts),
		"shard ID", sdp.shardCoordinator.SelfId(),
	)
	return smartContracts, nil
}

func (sdp *standardDelegationProcessor) setDelegationStartParameters(smartContracts []genesis.InitialSmartContractHandler) error {
	for _, sc := range smartContracts {
		delegatedNodes := sdp.nodesListSplitter.GetDelegatedNodes(sc.AddressBytes())
		numNodes := len(delegatedNodes)

		log.Trace("setDelegationStartParameters",
			"SC owner", sc.GetOwner(),
			"SC address", sc.Address(),
			"num delegated nodes", numNodes,
			"node price", sdp.nodePrice.String(),
			"shard ID", sdp.shardCoordinator.SelfId(),
		)

		err := sdp.executeSetNodePrice(sc)
		if err != nil {
			return err
		}
	}

	return nil
}

func (sdp *standardDelegationProcessor) executeSetNodePrice(sc genesis.InitialSmartContractHandler) error {
	setStakePerNodeTxData := fmt.Sprintf("%s@%x", setStakePerNodeFunction, sdp.nodePrice)

	nonce, err := sdp.GetNonce(sc.OwnerBytes())
	if err != nil {
		return err
	}

	return sdp.ExecuteTransaction(
		nonce,
		sc.OwnerBytes(),
		sc.AddressBytes(),
		zero,
		[]byte(setStakePerNodeTxData),
	)
}

func (sdp *standardDelegationProcessor) executeStake(smartContracts []genesis.InitialSmartContractHandler) (int, error) {
	stakedOnDelegation := 0

	for _, sc := range smartContracts {
		accounts := sdp.accuntsParser.GetInitialAccountsForDelegated(sc.AddressBytes())
		if len(accounts) == 0 {
			log.Debug("genesis delegation SC was not delegated by any account",
				"SC owner", sc.GetOwner(),
				"SC address", sc.Address(),
			)
			continue
		}

		totalDelegated := big.NewInt(0)
		for _, ac := range accounts {
			err := sdp.stake(ac, sc)
			if err != nil {
				return 0, fmt.Errorf("%w while calling stake function from account %s", err, ac.GetAddress())
			}

			totalDelegated.Add(totalDelegated, ac.GetDelegationHandler().GetValue())
		}

		log.Trace("executeStake",
			"SC owner", sc.GetOwner(),
			"SC address", sc.Address(),
			"num accounts", len(accounts),
			"total delegated", totalDelegated,
		)
		stakedOnDelegation += len(accounts)
	}

	return stakedOnDelegation, nil
}

func (sdp *standardDelegationProcessor) stake(ac genesis.InitialAccountHandler, sc genesis.InitialSmartContractHandler) error {
	isIntraShardCall := sdp.shardCoordinator.SameShard(ac.AddressBytes(), sc.AddressBytes())

	dh := ac.GetDelegationHandler()
	if check.IfNil(dh) {
		return genesis.ErrNilDelegationHandler
	}
	if dh.GetValue() == nil {
		return genesis.ErrInvalidDelegationValue
	}

	var err error
	var nonce = uint64(0)
	if isIntraShardCall {
		//intra shard transaction, get current nonce in order to make the tx processor work
		nonce, err = sdp.GetNonce(ac.AddressBytes())
		if err != nil {
			return err
		}
	}

	stakeData := fmt.Sprintf("%s@%s", stakeFunction, dh.GetValue().Text(16))
	err = sdp.ExecuteTransaction(
		nonce,
		ac.AddressBytes(),
		sc.AddressBytes(),
		zero,
		[]byte(stakeData),
	)
	if err != nil {
		return err
	}

	return nil
}

func (sdp *standardDelegationProcessor) executeManageBlsKeys(
	smartContracts []genesis.InitialSmartContractHandler,
) (int, error) {

	log.Trace("executeManageSetBlsKeys",
		"num delegation SC", len(smartContracts),
		"shard ID", sdp.shardCoordinator.SelfId(),
		"function", addNodesFunction,
	)

	totalDelegated := 0
	for _, sc := range smartContracts {
		delegatedNodes := sdp.nodesListSplitter.GetDelegatedNodes(sc.AddressBytes())

		lenDelegated := len(delegatedNodes)
		if lenDelegated == 0 {
			log.Debug("genesis delegation SC does not have staked nodes",
				"SC owner", sc.GetOwner(),
				"SC address", sc.Address(),
				"function", addNodesFunction,
			)
			continue
		}
		totalDelegated += lenDelegated

		log.Trace("executeAddNode",
			"SC owner", sc.GetOwner(),
			"SC address", sc.Address(),
			"num nodes", lenDelegated,
			"shard ID", sdp.shardCoordinator.SelfId(),
			"function", addNodesFunction,
		)

		arguments := make([]string, 0, len(delegatedNodes)+1)
		arguments = append(arguments, addNodesFunction)
		for _, node := range delegatedNodes {
			arguments = append(arguments, hex.EncodeToString(node.PubKeyBytes()))
			arguments = append(arguments, hex.EncodeToString(genesisSignature))
		}

		nonce, err := sdp.GetNonce(sc.OwnerBytes())
		if err != nil {
			return 0, err
		}

		err = sdp.ExecuteTransaction(
			nonce,
			sc.OwnerBytes(),
			sc.AddressBytes(),
			big.NewInt(0),
			[]byte(strings.Join(arguments, "@")),
		)
		if err != nil {
			return 0, err
		}
	}

	return totalDelegated, nil
}

func (sdp *standardDelegationProcessor) executeActivation(smartContracts []genesis.InitialSmartContractHandler) error {

	log.Trace("executeActivation",
		"num delegation SC", len(smartContracts),
		"shard ID", sdp.shardCoordinator.SelfId(),
		"function", activateFunction,
	)

	for _, sc := range smartContracts {
		log.Trace("executeActivation",
			"SC owner", sc.GetOwner(),
			"SC address", sc.Address(),
			"shard ID", sdp.shardCoordinator.SelfId(),
			"function", activateFunction,
		)

		nonce, err := sdp.GetNonce(sc.OwnerBytes())
		if err != nil {
			return err
		}

		err = sdp.ExecuteTransaction(
			nonce,
			sc.OwnerBytes(),
			sc.AddressBytes(),
			big.NewInt(0),
			[]byte(activateFunction),
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func (sdp *standardDelegationProcessor) executeVerify(smartContracts []genesis.InitialSmartContractHandler) error {
	for _, sc := range smartContracts {
		err := sdp.verify(sc)
		if err != nil {
			return fmt.Errorf("%w for contract %s, owner %s", err, sc.Address(), sc.GetOwner())
		}
	}

	return nil
}

func (sdp *standardDelegationProcessor) verify(sc genesis.InitialSmartContractHandler) error {
	err := sdp.verifyStakedValue(sc)
	if err != nil {
		return fmt.Errorf("%w for verifyStakedValue", err)
	}

	err = sdp.verifyRegisteredNodes(sc)
	if err != nil {
		return fmt.Errorf("%w for verifyRegisteredNodes", err)
	}

	return nil
}

func (sdp *standardDelegationProcessor) verifyStakedValue(sc genesis.InitialSmartContractHandler) error {
	providedStakedValue := big.NewInt(0)
	providedDelegators := sdp.accuntsParser.GetInitialAccountsForDelegated(sc.AddressBytes())

	for _, delegator := range providedDelegators {
		if check.IfNil(delegator) {
			continue
		}
		dh := delegator.GetDelegationHandler()
		if check.IfNil(dh) {
			continue
		}
		if dh.GetValue() == nil {
			continue
		}

		err := sdp.checkDelegator(delegator, sc)
		if err != nil {
			return err
		}

		providedStakedValue.Add(providedStakedValue, dh.GetValue())
	}

	return nil
}

func (sdp *standardDelegationProcessor) checkDelegator(
	delegator genesis.InitialAccountHandler,
	sc genesis.InitialSmartContractHandler,
) error {
	scQueryStakeValue := &process.SCQuery{
		ScAddress: sc.AddressBytes(),
		FuncName:  "getUserStake",
		Arguments: [][]byte{delegator.AddressBytes()},
	}
	vmOutputStakeValue, err := sdp.queryService.ExecuteQuery(scQueryStakeValue)
	if err != nil {
		return err
	}
	if len(vmOutputStakeValue.ReturnData) != 1 {
		return fmt.Errorf("%w return data should have contained one element", genesis.ErrWhileVerifyingDelegation)
	}

	scStakedValue := big.NewInt(0).SetBytes(vmOutputStakeValue.ReturnData[0])
	if scStakedValue.Cmp(delegator.GetDelegationHandler().GetValue()) != 0 {
		return fmt.Errorf("%w staked data mismatch: from SC: %s, provided: %s, account %s",
			genesis.ErrWhileVerifyingDelegation, scStakedValue.String(),
			delegator.GetDelegationHandler().GetValue().String(), delegator.GetAddress())
	}

	return nil
}

func (sdp *standardDelegationProcessor) verifyRegisteredNodes(sc genesis.InitialSmartContractHandler) error {
	delegatedNodes := sdp.nodesListSplitter.GetDelegatedNodes(sc.AddressBytes())
	if len(delegatedNodes) == 0 {
		log.Debug("genesis delegation SC does not have staked nodes",
			"SC owner", sc.GetOwner(),
			"SC address", sc.Address(),
			"function", addNodesFunction,
		)

		return nil
	}

	for _, node := range delegatedNodes {
		err := sdp.verifyOneNode(sc, node)
		if err != nil {
			return err
		}
	}

	return nil
}

func (sdp *standardDelegationProcessor) verifyOneNode(
	sc genesis.InitialSmartContractHandler,
	node sharding.GenesisNodeInfoHandler,
) error {

	function := "getNodeSignature"
	scQueryBlsKeys := &process.SCQuery{
		ScAddress: sc.AddressBytes(),
		FuncName:  function,
		Arguments: [][]byte{node.PubKeyBytes()},
	}

	vmOutput, err := sdp.queryService.ExecuteQuery(scQueryBlsKeys)
	if err != nil {
		return err
	}

	if len(vmOutput.ReturnData) == 0 {
		return fmt.Errorf("%w for SC %s, owner %s, function %s, node %s",
			genesis.ErrEmptyReturnData, sc.Address(), sc.GetOwner(), function,
			hex.EncodeToString(node.PubKeyBytes()),
		)
	}

	if !bytes.Equal(vmOutput.ReturnData[0], genesisSignature) {
		return fmt.Errorf("%w for SC %s, owner %s, function %s, node %s",
			genesis.ErrSignatureMismatch, sc.Address(), sc.GetOwner(), function,
			hex.EncodeToString(node.PubKeyBytes()),
		)
	}

	return nil
}

// IsInterfaceNil returns if underlying object is true
func (sdp *standardDelegationProcessor) IsInterfaceNil() bool {
	return sdp == nil || sdp.TxExecutionProcessor == nil
}
