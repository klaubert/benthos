// Copyright (c) 2018 Ashley Jeffs
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package processor

import (
	"fmt"

	"github.com/Jeffail/benthos/lib/log"
	"github.com/Jeffail/benthos/lib/message"
	"github.com/Jeffail/benthos/lib/metrics"
	"github.com/Jeffail/benthos/lib/processor/condition"
	"github.com/Jeffail/benthos/lib/response"
	"github.com/Jeffail/benthos/lib/types"
)

//------------------------------------------------------------------------------

func init() {
	Constructors[TypeFilterParts] = TypeSpec{
		constructor: NewFilterParts,
		description: `
Tests each individual part of a message batch against a condition, if the
condition fails then the part is dropped. If the resulting batch is empty it
will be dropped. You can find a [full list of conditions here](../conditions),
in this case each condition will be applied to a part as if it were a single
part message.

This processor is useful if you are combining messages into batches using the
` + "[`batch`](#batch)" + ` processor and wish to remove specific parts.`,
		sanitiseConfigFunc: func(conf Config) (interface{}, error) {
			return condition.SanitiseConfig(conf.FilterParts.Config)
		},
	}
}

//------------------------------------------------------------------------------

// FilterPartsConfig contains configuration fields for the FilterParts
// processor.
type FilterPartsConfig struct {
	condition.Config `json:",inline" yaml:",inline"`
}

// NewFilterPartsConfig returns a FilterPartsConfig with default values.
func NewFilterPartsConfig() FilterPartsConfig {
	return FilterPartsConfig{
		Config: condition.NewConfig(),
	}
}

//------------------------------------------------------------------------------

// FilterParts is a processor that checks each part from a message against a
// condition and removes the part if the condition returns false.
type FilterParts struct {
	log   log.Modular
	stats metrics.Type

	condition condition.Type

	mCount       metrics.StatCounter
	mPartDropped metrics.StatCounter
	mDropped     metrics.StatCounter
	mSent        metrics.StatCounter
	mSentParts   metrics.StatCounter
}

// NewFilterParts returns a FilterParts processor.
func NewFilterParts(
	conf Config, mgr types.Manager, log log.Modular, stats metrics.Type,
) (Type, error) {
	nsLog := log.NewModule(".processor.filter_parts")
	nsStats := metrics.Namespaced(stats, "processor.filter_parts")
	cond, err := condition.New(conf.FilterParts.Config, mgr, nsLog, nsStats)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to construct condition '%v': %v",
			conf.FilterParts.Config.Type, err,
		)
	}
	return &FilterParts{
		log:       nsLog,
		stats:     stats,
		condition: cond,

		mCount:       stats.GetCounter("processor.filter_parts.count"),
		mPartDropped: stats.GetCounter("processor.filter_parts.part.dropped"),
		mDropped:     stats.GetCounter("processor.filter_parts.dropped"),
		mSent:        stats.GetCounter("processor.filter_parts.sent"),
		mSentParts:   stats.GetCounter("processor.filter_parts.parts.sent"),
	}, nil
}

//------------------------------------------------------------------------------

// ProcessMessage applies the processor to a message, either creating >0
// resulting messages or a response to be sent back to the message source.
func (c *FilterParts) ProcessMessage(msg types.Message) ([]types.Message, types.Response) {
	c.mCount.Incr(1)

	newMsg := message.New(nil)

	for i := 0; i < msg.Len(); i++ {
		if c.condition.Check(message.Lock(msg, i)) {
			newMsg.Append(msg.Get(i).Copy())
		} else {
			c.mPartDropped.Incr(1)
		}
	}
	if newMsg.Len() > 0 {
		c.mSent.Incr(1)
		c.mSentParts.Incr(int64(newMsg.Len()))
		msgs := [1]types.Message{newMsg}
		return msgs[:], nil
	}

	c.mDropped.Incr(1)
	return nil, response.NewAck()
}

//------------------------------------------------------------------------------
