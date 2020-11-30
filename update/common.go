package update

import (
	"github.com/ElrondNetwork/elrond-go-logger"
	"github.com/ElrondNetwork/elrond-go/core"
	"github.com/ElrondNetwork/elrond-go/core/check"
	"github.com/ElrondNetwork/elrond-go/data"
	"github.com/ElrondNetwork/elrond-go/data/block"
	"github.com/ElrondNetwork/elrond-go/hashing"
	"github.com/ElrondNetwork/elrond-go/marshal"
)

var log = logger.GetOrCreate("update")

// MbInfo defines the structure which hold the miniBlock info
type MbInfo struct {
	MbHash          []byte
	SenderShardID   uint32
	ReceiverShardID uint32
	Type            block.Type
	TxsInfo         []*TxInfo
}

// TxInfo defines the structure which hold the transaction info
type TxInfo struct {
	TxHash []byte
	Tx     data.TransactionHandler
}

// ArgsHardForkProcessor defines the arguments structure needed by hardfork processor methods
type ArgsHardForkProcessor struct {
	Hasher      hashing.Hasher
	Marshalizer marshal.Marshalizer
	ShardIDs    []uint32
}

// GetPendingMiniBlocks get all the pending miniBlocks from epoch start metaBlock and unFinished metaBlocks
func GetPendingMiniBlocks(
	epochStartMetaBlock *block.MetaBlock,
	unFinishedMetaBlocksMap map[string]*block.MetaBlock,
) ([]block.MiniBlockHeader, error) {

	if epochStartMetaBlock == nil {
		return nil, ErrNilEpochStartMetaBlock
	}
	if unFinishedMetaBlocksMap == nil {
		return nil, ErrNilUnFinishedMetaBlocksMap
	}

	pendingMiniBlocks := make([]block.MiniBlockHeader, 0)
	nonceToHashMap := createNonceToHashMap(unFinishedMetaBlocksMap)

	for _, shardData := range epochStartMetaBlock.EpochStart.LastFinalizedHeaders {
		computedPendingMiniBlocks, err := computePendingMiniBlocksFromUnFinishedMetaBlocks(
			shardData,
			unFinishedMetaBlocksMap,
			nonceToHashMap,
			epochStartMetaBlock.GetNonce(),
		)
		if err != nil {
			return nil, err
		}

		pendingMiniBlocks = append(pendingMiniBlocks, computedPendingMiniBlocks...)
	}

	return pendingMiniBlocks, nil
}

// createNonceToHashMap creates a map of nonce to hash from all the given metaBlocks
func createNonceToHashMap(unFinishedMetaBlocks map[string]*block.MetaBlock) map[uint64]string {
	nonceToHashMap := make(map[uint64]string, len(unFinishedMetaBlocks))
	for metaBlockHash, metaBlock := range unFinishedMetaBlocks {
		nonceToHashMap[metaBlock.GetNonce()] = metaBlockHash
	}

	return nonceToHashMap
}

// computePendingMiniBlocksFromUnFinishedMetaBlocks computes all the pending miniBlocks from unFinished metaBlocks
func computePendingMiniBlocksFromUnFinishedMetaBlocks(
	epochStartShardData block.EpochStartShardData,
	unFinishedMetaBlocks map[string]*block.MetaBlock,
	nonceToHashMap map[uint64]string,
	epochStartMetaBlockNonce uint64,
) ([]block.MiniBlockHeader, error) {
	pendingMiniBlocks := make([]block.MiniBlockHeader, 0)
	pendingMiniBlocks = append(pendingMiniBlocks, epochStartShardData.PendingMiniBlockHeaders...)

	firstPendingMetaBlock, ok := unFinishedMetaBlocks[string(epochStartShardData.FirstPendingMetaBlock)]
	if !ok {
		return nil, ErrWrongUnFinishedMetaHdrsMap
	}

	firstUnFinishedMetaBlockNonce := firstPendingMetaBlock.GetNonce()
	for nonce := firstUnFinishedMetaBlockNonce + 1; nonce <= epochStartMetaBlockNonce; nonce++ {
		metaBlockHash, exists := nonceToHashMap[nonce]
		if !exists {
			return nil, ErrWrongUnFinishedMetaHdrsMap
		}

		metaBlock, exists := unFinishedMetaBlocks[metaBlockHash]
		if !exists {
			return nil, ErrWrongUnFinishedMetaHdrsMap
		}

		pendingMiniBlocksFromMetaBlock := getAllMiniBlocksWithDst(metaBlock, epochStartShardData.ShardID)
		pendingMiniBlocks = append(pendingMiniBlocks, pendingMiniBlocksFromMetaBlock...)
	}

	return pendingMiniBlocks, nil
}

// getAllMiniBlocksWithDst returns all miniBlock headers with the given destination from the given metaBlock
func getAllMiniBlocksWithDst(metaBlock *block.MetaBlock, destShardID uint32) []block.MiniBlockHeader {
	mbHdrs := make([]block.MiniBlockHeader, 0)
	for i := 0; i < len(metaBlock.ShardInfo); i++ {
		if metaBlock.ShardInfo[i].ShardID == destShardID {
			continue
		}

		for _, mbHdr := range metaBlock.ShardInfo[i].ShardMiniBlockHeaders {
			if mbHdr.ReceiverShardID == destShardID && mbHdr.SenderShardID != destShardID {
				mbHdrs = append(mbHdrs, mbHdr)
			}
		}
	}

	for _, mbHdr := range metaBlock.MiniBlockHeaders {
		if mbHdr.ReceiverShardID == destShardID && mbHdr.SenderShardID != destShardID {
			mbHdrs = append(mbHdrs, mbHdr)
		}
	}

	return mbHdrs
}

// CreateBody will create a block body after hardfork import
func CreateBody(
	args ArgsHardForkProcessor,
	mapBodies map[uint32]*block.Body,
	mapHardForkBlockProcessor map[uint32]HardForkBlockProcessor,
) ([]*MbInfo, error) {
	allPostMbs := make([]*MbInfo, 0)
	for _, shardID := range args.ShardIDs {
		hardForkBlockProcessor, ok := mapHardForkBlockProcessor[shardID]
		if !ok {
			return nil, ErrNilHardForkBlockProcessor
		}

		body, postMbs, err := hardForkBlockProcessor.CreateBody()
		if err != nil {
			return nil, err
		}

		log.Debug("CreateBody",
			"miniBlocks in body", len(body.MiniBlocks),
			"postMbs", len(postMbs),
		)

		allPostMbs = append(allPostMbs, postMbs...)
		mapBodies[shardID] = body
	}

	return CleanDuplicates(args, mapBodies, allPostMbs)
}

// CreatePostMiniBlocks will create all the post miniBlocks after hardfork import
func CreatePostMiniBlocks(
	args ArgsHardForkProcessor,
	lastPostMbs []*MbInfo,
	mapBodies map[uint32]*block.Body,
	mapHardForkBlockProcessor map[uint32]HardForkBlockProcessor,
) error {
	var err error
	numPostMbs := len(lastPostMbs)
	for numPostMbs > 0 {
		log.Debug("CreatePostBodies", "numPostMbs", numPostMbs)
		currentPostMbs := make([]*MbInfo, 0)
		for _, shardID := range args.ShardIDs {
			hardForkBlockProcessor, ok := mapHardForkBlockProcessor[shardID]
			if !ok {
				return ErrNilHardForkBlockProcessor
			}

			postBody, postMbs, err := hardForkBlockProcessor.CreatePostMiniBlocks(lastPostMbs)
			if err != nil {
				return err
			}

			currentBody, ok := mapBodies[shardID]
			if !ok {
				return ErrNilBlockBody
			}

			log.Debug("CreatePostBodies",
				"shard", shardID,
				"miniBlocks in current body", len(currentBody.MiniBlocks),
				"miniBlocks in post body", len(postBody.MiniBlocks),
				"postMbs", len(postMbs),
			)

			currentBody.MiniBlocks = append(currentBody.MiniBlocks, postBody.MiniBlocks...)
			currentPostMbs = append(currentPostMbs, postMbs...)
			mapBodies[shardID] = currentBody
		}

		lastPostMbs, err = CleanDuplicates(args, mapBodies, currentPostMbs)
		if err != nil {
			return err
		}

		numPostMbs = len(lastPostMbs)
	}

	return nil
}

// CleanDuplicates cleans from the post miniBlocks map, the already existing miniBlocks in bodies map
func CleanDuplicates(
	args ArgsHardForkProcessor,
	mapBodies map[uint32]*block.Body,
	postMbs []*MbInfo,
) ([]*MbInfo, error) {
	if check.IfNil(args.Hasher) {
		return nil, ErrNilHasher
	}
	if check.IfNil(args.Marshalizer) {
		return nil, ErrNilMarshalizer
	}

	mapMiniBlocksHashes := make(map[string]struct{})
	for _, shardID := range args.ShardIDs {
		currentBody, ok := mapBodies[shardID]
		if !ok {
			return nil, ErrNilBlockBody
		}

		for _, miniBlock := range currentBody.MiniBlocks {
			miniBlockHash, err := core.CalculateHash(args.Marshalizer, args.Hasher, miniBlock)
			if err != nil {
				return nil, err
			}

			mapMiniBlocksHashes[string(miniBlockHash)] = struct{}{}
		}
	}

	cleanedPostMbs := make([]*MbInfo, 0)
	for _, postMb := range postMbs {
		_, ok := mapMiniBlocksHashes[string(postMb.MbHash)]
		if ok {
			log.Debug("CleanDuplicates: found duplicated miniBlock", "hash", postMb.MbHash)
			continue
		}

		cleanedPostMbs = append(cleanedPostMbs, postMb)
	}

	return cleanedPostMbs, nil
}
