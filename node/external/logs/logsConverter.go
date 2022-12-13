package logs

import (
	"github.com/ElrondNetwork/elrond-go-core/core"
	"github.com/ElrondNetwork/elrond-go-core/data/transaction"
)

type logsConverter struct {
	pubKeyConverter core.PubkeyConverter
}

func newLogsConverter(pubKeyConverter core.PubkeyConverter) *logsConverter {
	return &logsConverter{
		pubKeyConverter: pubKeyConverter,
	}
}

func (converter *logsConverter) txLogToApiResource(logKey []byte, log *transaction.Log) *transaction.ApiLogs {
	events := make([]*transaction.Events, len(log.Events))

	for i, event := range log.Events {
		eventAddress := converter.encodeAddress(event.Address)

		events[i] = &transaction.Events{
			Address:    eventAddress,
			Identifier: string(event.Identifier),
			Topics:     event.Topics,
			Data:       event.Data,
		}
	}

	logAddress := converter.encodeAddress(log.Address)

	return &transaction.ApiLogs{
		Address: logAddress,
		Events:  events,
	}
}

func (converter *logsConverter) encodeAddress(pubkey []byte) string {
	return converter.pubKeyConverter.SilentEncode(pubkey, log)
}
