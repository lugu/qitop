package main

import (
	"fmt"
	"time"

	"github.com/lugu/qiloop/bus"
	"github.com/lugu/qiloop/bus/net"
	"github.com/lugu/qiloop/type/value"
	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/widgets/linechart"
)

type callEvent struct {
	timestamp time.Time
	duration  time.Duration
	callSize  int
	replySize int
}

type collector struct {
	service string
	action  string
	slot    uint32

	callData    []float64
	replyData   []float64
	latencyData []float64

	cancel func()

	pending map[uint32]callEvent
	records []callEvent
}

func newCollector(sess bus.Session, w *widgets, service, action string) (*collector, error) {
	objectID := uint32(1)
	proxy, err := sess.Proxy(service, objectID)
	if err != nil {
		return nil, fmt.Errorf("trace %s: %s", service, err)
	}
	obj := bus.MakeObject(proxy)

	meta, err := obj.MetaObject(objectID)
	if err != nil {
		return nil, fmt.Errorf("%s: MetaObject: %s.", service, err)
	}

	slot, err := meta.MethodID(action)
	if err != nil {
		return nil, fmt.Errorf("method not found: %s.", action)
	}

	obj.EnableTrace(true)
	if err != nil {
		return nil, fmt.Errorf("Failed to start traces: %s", err)
	}

	cancel, events, err := obj.SubscribeTraceObject()
	if err != nil {
		return nil, fmt.Errorf("Failed to stop stats: %s.", err)
	}

	c := &collector{
		service: service,
		action:  action,
		slot:    slot,

		cancel: cancel,

		pending: map[uint32]callEvent{},
		records: []callEvent{},

		callData:    []float64{},
		replyData:   []float64{},
		latencyData: []float64{},
	}

	go func(events chan bus.EventTrace) {
		for {
			e, ok := <-events
			if !ok {
				obj.EnableTrace(false)
				return
			}
			c.refreshData(e)
			c.updateUI(w)
		}
	}(events)

	return c, nil
}

func (c *collector) refreshData(e bus.EventTrace) {
	if e.SlotId != c.slot {
		return
	}

	size := 24 + len(value.Bytes(e.Arguments))
	ts := time.Unix(e.Timestamp.Tv_sec, e.Timestamp.Tv_usec*1000)

	switch e.Kind {
	case int32(net.Call):
		c.callData = append(c.callData, float64(size))
		c.pending[e.Id] = callEvent{
			timestamp: ts,
			callSize:  size,
		}
	case int32(net.Reply):
		c.replyData = append(c.replyData, float64(size))
		call, ok := c.pending[e.Id]
		if !ok {
			break
		}
		call.replySize = size

		if ts.Before(call.timestamp) {
			delete(c.pending, e.Id)
			break
		}

		call.duration = ts.Sub(call.timestamp)
		delete(c.pending, e.Id)
		c.records = append(c.records, call)

		c.latencyData = append(c.latencyData,
			float64(call.duration.Microseconds()))
	case int32(net.Error): // TODO
	}
}

func (c *collector) updateUI(w *widgets) {
	dx := w.sizePlot.ValueCapacity()
	start := 0
	if dx != 0 && dx < len(c.callData) {
		start = len(c.callData) - dx
	}
	w.sizePlot.Series("call size (byte)",
		c.callData[start:],
		linechart.SeriesCellOpts(cell.FgColor(cell.ColorBlue)),
	)
	w.sizePlot.Series("reply size (byte)",
		c.replyData[start:],
		linechart.SeriesCellOpts(cell.FgColor(cell.ColorGreen)),
	)

	dx = w.latencyPlot.ValueCapacity()
	start = 0
	if dx != 0 && dx < len(c.latencyData) {
		start = len(c.latencyData) - dx
	}
	w.latencyPlot.Series("response time (us)",
		c.latencyData[start:],
		linechart.SeriesCellOpts(cell.FgColor(cell.ColorYellow)),
	)
}
