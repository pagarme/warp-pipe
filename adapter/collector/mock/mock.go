package mock

import (
	"context"

	"github.com/pagarme/warp-pipe/pipeline"
)

type collectFunc func(messageID uint64, publishCh chan<- pipeline.Message) (end bool)

type updateOffsetFunc func(offset uint64)

// Collector object
type Collector struct {
	pipeline.Collector
	numOfMessages  uint64
	collectCb      collectFunc
	updateOffsetCb updateOffsetFunc
}

// New return a Collector instance
func New(numberOfMessages uint64, collectCb collectFunc, updateOffsetCb updateOffsetFunc) *Collector {

	return &Collector{
		numOfMessages:  numberOfMessages,
		collectCb:      collectCb,
		updateOffsetCb: updateOffsetCb,
	}
}

// Init implements method from interface
func (c *Collector) Init(ctx context.Context) (err error) { return nil }

// Collect implements method from interface
func (c *Collector) Collect(publishCh chan<- pipeline.Message) {
	defer close(publishCh)

	for i := uint64(0); i < c.numOfMessages; i++ {
		var end bool
		if end = c.collectCb(i, publishCh); end {
			return
		}
	}
}

// UpdateOffset implements method from interface
func (c *Collector) UpdateOffset(offsetCh <-chan uint64) {
	for offset := range offsetCh {
		c.updateOffsetCb(offset)
	}
}
