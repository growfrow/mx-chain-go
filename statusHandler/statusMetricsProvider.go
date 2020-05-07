package statusHandler

import (
	"strings"
	"sync"

	"github.com/ElrondNetwork/elrond-go/core"
)

// statusMetrics will handle displaying at /node/details all metrics already collected for other status handlers
type statusMetrics struct {
	nodeMetrics *sync.Map
}

// NewStatusMetrics will return an instance of the struct
func NewStatusMetrics() *statusMetrics {
	return &statusMetrics{
		nodeMetrics: &sync.Map{},
	}
}

// IsInterfaceNil returns true if there is no value under the interface
func (sm *statusMetrics) IsInterfaceNil() bool {
	return sm == nil
}

// Increment method increment a metric
func (sm *statusMetrics) Increment(key string) {
	keyValueI, ok := sm.nodeMetrics.Load(key)
	if !ok {
		return
	}

	keyValue, ok := keyValueI.(uint64)
	if !ok {
		return
	}

	keyValue++
	sm.nodeMetrics.Store(key, keyValue)
}

// AddUint64 method increase a metric with a specific value
func (sm *statusMetrics) AddUint64(key string, val uint64) {
	keyValueI, ok := sm.nodeMetrics.Load(key)
	if !ok {
		return
	}

	keyValue, ok := keyValueI.(uint64)
	if !ok {
		return
	}

	keyValue += val
	sm.nodeMetrics.Store(key, keyValue)
}

// Decrement method - decrement a metric
func (sm *statusMetrics) Decrement(key string) {
	keyValueI, ok := sm.nodeMetrics.Load(key)
	if !ok {
		return
	}

	keyValue, ok := keyValueI.(uint64)
	if !ok {
		return
	}
	if keyValue == 0 {
		return
	}

	keyValue--
	sm.nodeMetrics.Store(key, keyValue)
}

// SetInt64Value method - sets an int64 value for a key
func (sm *statusMetrics) SetInt64Value(key string, value int64) {
	sm.nodeMetrics.Store(key, value)
}

// SetUInt64Value method - sets an uint64 value for a key
func (sm *statusMetrics) SetUInt64Value(key string, value uint64) {
	sm.nodeMetrics.Store(key, value)
}

// SetStringValue method - sets a string value for a key
func (sm *statusMetrics) SetStringValue(key string, value string) {
	sm.nodeMetrics.Store(key, value)
}

// Close method - won't do anything
func (sm *statusMetrics) Close() {
}

// StatusMetricsMapWithoutP2P will return the non-p2p metrics in a map
func (sm *statusMetrics) StatusMetricsMapWithoutP2P() map[string]interface{} {
	statusMetricsMap := make(map[string]interface{})
	sm.nodeMetrics.Range(func(key, value interface{}) bool {
		keyString := key.(string)
		if strings.Contains(keyString, "_p2p_") {
			return true
		}

		statusMetricsMap[key.(string)] = value
		return true
	})

	return statusMetricsMap
}

// StatusP2pMetricsMap will return the p2p metrics in a map
func (sm *statusMetrics) StatusP2pMetricsMap() map[string]interface{} {
	statusMetricsMap := make(map[string]interface{})
	sm.nodeMetrics.Range(func(key, value interface{}) bool {
		keyString := key.(string)
		if !strings.Contains(keyString, "_p2p_") {
			return true
		}

		statusMetricsMap[key.(string)] = value
		return true
	})

	return statusMetricsMap
}

// EpochMetrics will return metrics related to current epoch
func (sm *statusMetrics) EpochMetrics() map[string]interface{} {
	epochMetrics := make(map[string]interface{})

	currentRound := sm.loadUint64Metric(core.MetricCurrentRound)
	roundNumberAtEpochStart := sm.loadUint64Metric(core.MetricRoundAtEpochStart)
	epochMetrics[core.MetricEpochNumber] = sm.loadUint64Metric(core.MetricEpochNumber)
	epochMetrics[core.MetricRoundsPerEpoch] = sm.loadUint64Metric(core.MetricRoundsPerEpoch)
	epochMetrics[core.MetricCurrentRound] = currentRound
	epochMetrics[core.MetricRoundAtEpochStart] = roundNumberAtEpochStart
	epochMetrics[core.MetricRoundsPassedInCurrentEpoch] = currentRound - roundNumberAtEpochStart

	return epochMetrics
}

// ConfigMetrics will return metrics related to current configuration
func (sm *statusMetrics) ConfigMetrics() map[string]interface{} {
	configMetrics := make(map[string]interface{})

	configMetrics[core.MetricNumShardsWithoutMetacahin] = sm.loadUint64Metric(core.MetricNumShardsWithoutMetacahin)
	configMetrics[core.MetricNumNodesPerShard] = sm.loadUint64Metric(core.MetricNumNodesPerShard)
	configMetrics[core.MetricNumMetachainNodes] = sm.loadUint64Metric(core.MetricNumMetachainNodes)
	configMetrics[core.MetricShardConsensusGroupSize] = sm.loadUint64Metric(core.MetricShardConsensusGroupSize)
	configMetrics[core.MetricMetaConsensusGroupSize] = sm.loadUint64Metric(core.MetricMetaConsensusGroupSize)
	configMetrics[core.MetricMinGasPrice] = sm.loadUint64Metric(core.MetricMinGasPrice)
	configMetrics[core.MetricMinGasLimit] = sm.loadUint64Metric(core.MetricMinGasLimit)
	configMetrics[core.MetricGasPerDataByte] = sm.loadUint64Metric(core.MetricGasPerDataByte)
	configMetrics[core.MetricChainId] = sm.loadStringMetric(core.MetricChainId)
	configMetrics[core.MetricRoundDuration] = sm.loadUint64Metric(core.MetricRoundDuration)
	configMetrics[core.MetricStartTime] = sm.loadUint64Metric(core.MetricStartTime)

	return configMetrics
}

// NetworkMetrics will return metrics related to current configuration
func (sm *statusMetrics) NetworkMetrics() map[string]interface{} {
	networkMetrics := make(map[string]interface{})

	networkMetrics[core.MetricNonce] = sm.loadUint64Metric(core.MetricNonce)
	networkMetrics[core.MetricCurrentRound] = sm.loadUint64Metric(core.MetricCurrentRound)
	networkMetrics[core.MetricEpochNumber] = sm.loadUint64Metric(core.MetricEpochNumber)

	return networkMetrics
}

func (sm *statusMetrics) loadUint64Metric(metric string) uint64 {
	metricObj, ok := sm.nodeMetrics.Load(metric)
	if !ok {
		return 0
	}
	metricAsUint64, ok := metricObj.(uint64)
	if !ok {
		return 0
	}

	return metricAsUint64
}

func (sm *statusMetrics) loadStringMetric(metric string) string {
	metricObj, ok := sm.nodeMetrics.Load(metric)
	if !ok {
		return ""
	}
	metricAsString, ok := metricObj.(string)
	if !ok {
		return ""
	}

	return metricAsString
}
